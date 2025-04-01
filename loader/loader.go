package loader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/kustomize"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
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
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)
	_ = sourcev1b2.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)
}

type Kustomization struct {
	*kustomizev1.Kustomization

	Error     error                `json:"-"`
	Resources []*resource.Resource `json:"-"`
}

type GitRepository struct {
	*sourcev1.GitRepository

	FS filesys.FileSystem
}

type OCIRepository struct {
	*sourcev1b2.OCIRepository

	FS filesys.FileSystem
}

type Loader struct {
	logger zerolog.Logger
	cs     client.Client
	root   *kustomizev1.Kustomization

	kustomizations  map[string]*Kustomization
	gitRepositories map[string]*GitRepository
	ociRepositories map[string]*OCIRepository

	repoCachePath string
	repoReplace   []*LocalGitRepository
}

func NewLoader(opts ...Option) *Loader {
	l := &Loader{
		logger:          zerolog.New(os.Stderr).With().Timestamp().Logger(),
		cs:              fake.NewClientBuilder().WithScheme(scheme).Build(),
		root:            nil,
		kustomizations:  make(map[string]*Kustomization),
		gitRepositories: make(map[string]*GitRepository),
		ociRepositories: make(map[string]*OCIRepository),
		repoCachePath:   "./cache",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

func (l *Loader) Kustomizations() []*Kustomization {
	var ks []*Kustomization
	for _, v := range l.kustomizations {
		ks = append(ks, v)
	}
	return ks
}

func (l *Loader) GitRepositories() []*GitRepository {
	var ks []*GitRepository
	for _, v := range l.gitRepositories {
		ks = append(ks, v)
	}
	return ks
}

func (l *Loader) OciRepositories() []*OCIRepository {
	var ks []*OCIRepository
	for _, v := range l.ociRepositories {
		ks = append(ks, v)
	}
	return ks
}

type StopCondition func(ks *Kustomization, gr *GitRepository) bool

func (l *Loader) Load(
	fs filesys.FileSystem,
	path string,
	namespace string,
	shouldStop StopCondition,
) error {
	start := time.Now()
	defer func() {
		l.logger.Debug().
			Dur("duration", time.Since(start)).
			Str("path", path).
			Msg("loading finished")
	}()

	direct, err := l.loadPath(fs, path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
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
			Int("attempt", item.Attempt).
			Logger()

		if item.Attempt > 5 {
			switch value := item.Value.(type) {
			case *kustomizev1.Kustomization:
				l.kustomizations[namespacedName(value)] = &Kustomization{
					Kustomization: value,
					Error:         item.Err,
				}
			}
			continue
		}

		switch value := item.Value.(type) {
		case *resource.Resource:
			err = l.handleResource(queue, value, namespace)
		case *sourcev1.GitRepository:
			err = l.handleGitRepository(value)
			if err == nil {
				logger.Debug().
					Str("url", value.Spec.URL).
					Str("ref", gitRepoReference(value)).
					Msg("loaded git repository")
			}
			lgr, ok := l.gitRepositories[namespacedName(value)]
			if ok {
				if shouldStop(nil, lgr) {
					return nil
				}
			}
		case *sourcev1b2.OCIRepository:
			err = l.handleOCIRepository(value)
			if err == nil {
				logger.Debug().
					Str("url", value.Spec.URL).
					Str("ref", ociRepoReference(value)).
					Msg("loaded oci repository")
			}
		case *kustomizev1.Kustomization:
			if l.root == nil {
				l.root = value
			} else if value.Namespace == l.root.Namespace && value.Name == l.root.Name {
				continue
			}
			if err := l.handleKustomization(queue, value); err != nil {
				queue.Retry(item, err)
				continue
			}
			l.logger.Debug().
				Err(err).
				Str("name", item.NamespacedName()).
				Msg("added kustomization")
			lks, ok := l.kustomizations[namespacedName(value)]
			if ok {
				if shouldStop(lks, nil) {
					return nil
				}
			}
		default:
			return fmt.Errorf("unknown queue item %T", item.Value)
		}
		if err != nil {
			queue.Retry(item, err)
			logger.Debug().Err(err).Msg("retrying")
		}
	}

	return nil
}

func (l *Loader) handleResource(queue *Queue, res *resource.Resource, namespace string) error {
	nn := namespacedName(res)
	if nn == "" {
		return nil
	}
	if res.GetNamespace() == "" {
		_ = res.SetNamespace(namespace)
	}

	yamlBytes, err := res.AsYAML()
	if err != nil {
		return fmt.Errorf("failed to convert resource to YAML: %v", err)
	}

	kind := res.GetKind()
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

func (l *Loader) handleKustomization(
	queue *Queue,
	ks *kustomizev1.Kustomization,
) error {
	repoName := fmt.Sprintf(
		"%s/%s",
		orDefault(ks.Spec.SourceRef.Namespace, ks.Namespace),
		ks.Spec.SourceRef.Name,
	)

	var repoFS filesys.FileSystem

	//fmt.Println(kustomizations.Namespace, kustomizations.Name, repoMeta.Namespace, repoMeta.Name)
	switch ks.Spec.SourceRef.Kind {
	case "GitRepository":
		repo, ok := l.gitRepositories[repoName]
		if !ok {
			return fmt.Errorf("could not find git source: %s", repoName)
		}
		repoFS = repo.FS
	case "OCIRepository":
		repo, ok := l.ociRepositories[repoName]
		if !ok {
			return fmt.Errorf("could not find OCI source: %s", repoName)
		}
		repoFS = repo.FS
	default:
		return fmt.Errorf(
			"unsupported Kustomization source: %s", ks.Spec.SourceRef.Kind,
		)
	}

	targetNamespace := orDefault(ks.Spec.TargetNamespace, ks.Namespace)

	resources, err := l.loadPath(repoFS, ks.Spec.Path)
	if err != nil {
		return fmt.Errorf("failed to load path: %v", err)
	}

	// Variable substitution:
	if pb := ks.Spec.PostBuild; pb != nil && (pb.SubstituteFrom != nil || pb.Substitute != nil) {
		ksYaml, err := sigyaml.Marshal(ks)
		if err != nil {
			return fmt.Errorf("failed to marshal Kustomization: %v", err)
		}
		var ksObj unstructured.Unstructured
		if err := sigyaml.Unmarshal(ksYaml, &ksObj); err != nil {
			return fmt.Errorf("failed to unmarshal Kustomization: %v", err)
		}

		for i, res := range resources {
			newRes, err := kustomize.SubstituteVariables(
				context.Background(),
				l.cs,
				ksObj,
				res,
				kustomize.SubstituteWithStrict(true),
			)
			if err != nil {
				return err
			}
			resources[i] = newRes
		}
	}

	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(targetNamespace)
		}
		queue.Push(&QueueItem{Value: res})
	}

	l.kustomizations[namespacedName(ks)] = &Kustomization{
		Kustomization: ks,
		Resources:     resources,
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
	var resources []*resource.Resource

	if fs.Exists(filepath.Join(path, konfig.DefaultKustomizationFileName())) {
		resMap, err := kustomizer.Run(fs, path)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resMap.Resources()...)
		return resources, nil
	}

	// Folder:
	if fs.IsDir(path) {
		entries, err := fs.ReadDir(path)
		if err != nil {
			return nil, err
		}

		// Regular Folder:
		for _, entry := range entries {
			res, err := l.loadPath(fs, filepath.Join(path, entry))
			if err != nil {
				return nil, err
			}
			resources = append(resources, res...)
		}
		return resources, nil
	}

	// File:
	// Skip non YAML files:
	if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		return nil, nil
	}
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return l.loadBytes(data)
}

func (l *Loader) loadBytes(data []byte) ([]*resource.Resource, error) {
	var resources []*resource.Resource
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc map[string]any
	for {
		if err := dec.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			return resources, nil
		}
		docYaml, _ := yaml.Marshal(doc)
		res, err := kyaml.Parse(string(docYaml))
		if err != nil {
			return nil, err
		}
		resources = append(resources, &resource.Resource{
			RNode: *res,
		})
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
