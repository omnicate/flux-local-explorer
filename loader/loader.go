package loader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/rs/zerolog"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	sigyaml "sigs.k8s.io/yaml"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resource"
	kustypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)
	_ = sourcev1b2.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)
}

const (
	maxAttempts = 5
)

var (
	ErrSkip = errors.New("skip")
)

type Loader struct {
	logger zerolog.Logger
	cs     client.Client
	root   *kustomizev1.Kustomization

	repos         map[string]filesys.FileSystem
	repoCachePath string
	repoReplace   []*LocalGitRepository
	gitViaHTTPS   bool
}

func NewLoader(opts ...Option) *Loader {
	l := &Loader{
		logger:        zerolog.New(os.Stderr).With().Timestamp().Logger(),
		cs:            fake.NewClientBuilder().WithScheme(scheme).Build(),
		root:          nil,
		repos:         make(map[string]filesys.FileSystem),
		repoCachePath: "./cache",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

func (l *Loader) Kustomizations(
	fs filesys.FileSystem,
	path string,
	namespace string,
) ErrSeq[*Kustomization] {
	seq := l.Iter(fs, path, namespace)
	return typedIter[*Kustomization](seq)
}

func (l *Loader) GitRepositories(
	fs filesys.FileSystem,
	path string,
	namespace string,
) ErrSeq[*GitRepository] {
	seq := l.Iter(fs, path, namespace)
	return typedIter[*GitRepository](seq)
}

func (l *Loader) OCIRepositories(
	fs filesys.FileSystem,
	path string,
	namespace string,
) ErrSeq[*OCIRepository] {
	seq := l.Iter(fs, path, namespace)
	return typedIter[*OCIRepository](seq)
}

func (l *Loader) Iter(
	fs filesys.FileSystem,
	path string,
	namespace string,
) ErrSeq[NamedResource] {
	start := time.Now()
	return func(yield func(NamedResource, error) bool) {
		l.logger.Debug().
			Str("path", path).
			Str("cache", l.repoCachePath).
			Msg("loading resources")

		direct, err := l.loadPath(fs, path)
		if err != nil {
			yield(nil, fmt.Errorf("loading %s: %w", path, err))
			return
		}
		queue := new(Queue)
		for _, res := range direct {
			queue.Push(&QueueItem{
				Value: res,
			})
		}

		for {
			item, ok := queue.Pop()
			if !ok {
				break
			}

			logger := l.logger.With().
				Str("name", item.NamespacedName()).
				Str("kind", item.Kind()).
				Logger()

			// If maximum attempts exceeded, yield the incomplete object:
			if item.Attempt >= maxAttempts {
				switch value := item.Value.(type) {
				case *kustomizev1.Kustomization:
					if !yield(&Kustomization{
						Kustomization: value,
						Error:         item.Err,
					}, nil) {
						return
					}
				case *sourcev1.GitRepository:
					if !yield(&GitRepository{
						GitRepository: value,
						Error:         item.Err,
					}, nil) {
						return
					}
				case *sourcev1b2.OCIRepository:
					if !yield(&OCIRepository{
						OCIRepository: value,
						Error:         item.Err,
					}, nil) {
						return
					}
				}
				continue
			}

			var result NamedResource
			switch value := item.Value.(type) {
			case *resource.Resource:
				err = l.handleResource(queue, value, namespace)
			case *sourcev1.GitRepository:
				result, err = l.handleGitRepository(logger, value)
			case *sourcev1b2.OCIRepository:
				result, err = l.handleOCIRepository(value)
			case *kustomizev1.Kustomization:
				result, err = l.handleKustomization(queue, value)
			default:
				yield(nil, fmt.Errorf("unknown queue item %T", item.Value))
				return
			}
			if errors.Is(err, ErrSkip) {
				logger.Debug().Msg("item skipped")
			} else if err != nil {
				queue.Retry(item, err)
				logger.Debug().
					Err(err).
					Int("attempt", item.Attempt).
					Msg("retrying")
			} else {
				if !yield(result, nil) {
					logger.Debug().
						Str("duration", time.Since(start).String()).
						Msg("stop yielding")
					return
				}
			}
		}

		l.logger.Debug().
			Str("duration", time.Since(start).String()).
			Msg("stop yielding")
	}
}

var traverseResourceKinds = map[string]struct{}{
	"Kustomization": {},
	"GitRepository": {},
	"OCIRepository": {},
	"ConfigMap":     {},
	"Secret":        {},
}

func (l *Loader) handleResource(queue *Queue, res *resource.Resource, namespace string) error {
	kind := res.GetKind()
	if _, ok := traverseResourceKinds[kind]; !ok {
		return nil
	}
	if res.GetNamespace() == "" {
		_ = res.SetNamespace(namespace)
	}

	yamlBytes, err := res.AsYAML()
	if err != nil {
		return fmt.Errorf("failed to convert resource to YAML: %v", err)
	}

	apiVersion := res.GetApiVersion()
	if apiVersion == "kustomize.toolkit.fluxcd.io/v1" && kind == "Kustomization" {
		ks := new(kustomizev1.Kustomization)
		if err := sigyaml.Unmarshal(yamlBytes, ks); err != nil {
			return err
		}
		queue.Push(&QueueItem{Value: ks})
		return nil
	}

	if apiVersion == "source.toolkit.fluxcd.io/v1" && kind == "GitRepository" {
		gr := new(sourcev1.GitRepository)
		if err := sigyaml.Unmarshal(yamlBytes, gr); err != nil {
			return err
		}
		queue.Push(&QueueItem{Value: gr})
		return nil
	}

	if apiVersion == "source.toolkit.fluxcd.io/v1beta2" && kind == "OCIRepository" {
		or := new(sourcev1b2.OCIRepository)
		if err := sigyaml.Unmarshal(yamlBytes, or); err != nil {
			return err
		}
		queue.Push(&QueueItem{Value: or})
		return nil
	}

	var obj unstructured.Unstructured
	if err := sigyaml.Unmarshal(yamlBytes, &obj); err != nil {
		// Only one chance to unmarshal into valid k8s resource.
		// Do not treat files that are not valid resources.
		//return fmt.Errorf("failed to unmarshal YAML into Unstructured: %v", err)
		return nil
	}
	if err = l.cs.Create(context.Background(), &obj); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create resource: %v", err)
	}

	return nil
}

var kustomizer = krusty.MakeKustomizer(&krusty.Options{
	LoadRestrictions: kustypes.LoadRestrictionsNone,
	PluginConfig:     kustypes.DisabledPluginConfig(),
})

func (l *Loader) loadPath(
	fs filesys.FileSystem,
	path string,
) ([]*resource.Resource, error) {

	// Kustomization:
	if fs.Exists(filepath.Join(path, konfig.DefaultKustomizationFileName())) {
		resMap, err := kustomizer.Run(fs, path)
		if err != nil {
			return nil, err
		}
		return resMap.Resources(), nil
	}

	// Folder:
	if fs.IsDir(path) {
		entries, err := fs.ReadDir(path)
		if err != nil {
			return nil, err
		}

		// Regular Folder:
		resources := make([]*resource.Resource, 0, len(entries))
		for i := range entries {
			res, err := l.loadPath(fs, filepath.Join(path, entries[i]))
			if err != nil {
				return nil, err
			}
			resources = append(resources, res...)
		}
		return resources, nil
	}

	// Skip non YAML files:
	if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		return nil, nil
	}

	// YAML file:
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return l.loadBytes(data)
}

func (l *Loader) loadBytes(data []byte) ([]*resource.Resource, error) {
	docs, err := kio.FromBytes(data)
	if err != nil {
		return nil, err
	}
	resources := make([]*resource.Resource, len(docs))
	for i, doc := range docs {
		resources[i] = &resource.Resource{
			RNode: *doc,
		}
	}
	return resources, nil
}

func orDefault[T comparable](a, def T) T {
	var zero T
	if a == zero {
		return def
	}
	return a
}
