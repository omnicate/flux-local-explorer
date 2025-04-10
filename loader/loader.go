package loader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	intres "github.com/omnicate/flx/resource"
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
	root   *intres.Kustomization

	helmTemplate bool
	helmRepos    []*intres.HelmRepository

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
		helmTemplate:  true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}
func (l *Loader) Load(
	fs filesys.FileSystem,
	path string,
	defaultNamespace string,
) (*ResultSet, error) {
	start := time.Now()

	direct, err := l.loadPath(fs, path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	resultSet, err := l.buildResultSet(direct, defaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build root Kustomization: %w", err)
	}
	if err := l.handleResultSet(resultSet); err != nil {
		return nil, fmt.Errorf("could not build initial source set")
	}

	queue := new(Queue)

	for _, ks := range resultSet.Kustomizations {
		queue.Push(&QueueItem{
			Value:   ks,
			Attempt: 0,
		})
	}

	for {
		item, ok := queue.Pop()
		if !ok {
			break
		}
		logger := l.logger.
			With().
			Str("namespace", item.Value.Namespace).
			Str("name", item.Value.Name).
			Logger()

		logger.Debug().Msg("loading kustomization")

		ksResultSet, err := l.loadKustomization(item.Value)
		if errors.Is(err, ErrSkip) {
			logger.Debug().Err(err).Msg("skipping")
			continue
		} else if err != nil {
			if !queue.Retry(item, err) {
				logger.Debug().Msg("failed")
			}
			continue
		}

		resultSet.Merge(ksResultSet)

		for _, ks := range ksResultSet.Kustomizations {
			queue.Push(&QueueItem{
				Value:   ks,
				Attempt: 0,
			})
		}
	}

	l.logger.Debug().
		Str("elapsed", time.Since(start).String()).
		Msg("done")

	return resultSet, nil
}

var trackedResourceKinds = map[string]struct{}{
	"ConfigMap": {},
	"Secret":    {},
}

func (l *Loader) handleResultSet(rs *ResultSet) error {

	// Insert resources we're interested in to the fake k8s client:
	for _, res := range rs.Resources {
		// Only store objects that we are interested in:
		if _, ok := trackedResourceKinds[res.GetKind()]; !ok {
			continue
		}
		yamlBytes, err := res.AsYAML()
		if err != nil {
			return fmt.Errorf("failed to convert resource to YAML: %v", err)
		}
		var obj unstructured.Unstructured
		if err := sigyaml.Unmarshal(yamlBytes, &obj); err != nil {
			// Only one chance to unmarshal into valid k8s resource.
			// Do not treat files that are not valid resources.
			//return fmt.Errorf("failed to unmarshal YAML into Unstructured: %v", err)
			continue
		}
		if err = l.cs.Create(context.Background(), &obj); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create resource: %v", err)
		}
	}

	// Sort git repositories:
	sort.Slice(rs.GitRepositories, func(i, j int) bool {
		a, b := rs.GitRepositories[i], rs.GitRepositories[j]
		return len(a.Spec.Include) < len(b.Spec.Include)
	})

	for _, r := range rs.GitRepositories {
		if _, err := l.handleGitRepository(l.logger, r); err != nil {
			r.Error = err
			continue
		}
	}
	for _, r := range rs.OCIRepositories {
		if _, err := l.handleOCIRepository(r); err != nil {
			r.Error = err
			continue
		}
	}
	for _, r := range rs.HelmRepositories {
		if _, err := l.handleHelmRepository(r); err != nil {
			r.Error = err
			continue
		}
	}

	// Execute helm releases recursively:
	for _, r := range rs.HelmReleases {
		helmResult, err := l.renderHelmRelease(r)
		if err != nil {
			l.logger.Err(err).Msg("failed to render helm release")
			r.Error = err
			continue
		}
		if err := l.handleResultSet(helmResult); err != nil {
			l.logger.Err(err).Msg("failed to handle result set")
			r.Error = err
			continue
		}
		rs.Merge(helmResult)
	}

	return nil
}

func (l *Loader) buildResultSet(
	resources []*resource.Resource,
	defaultNamespace string,
) (
	*ResultSet,
	error,
) {
	result := EmptyResultSet()
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(defaultNamespace)
		}
		switch res.GetKind() {
		case intres.OCIRepositoryKind:
			hr, err := intres.NewOCIRepository(res)
			if err != nil {
				return nil, err
			}
			result.OCIRepositories = append(result.OCIRepositories, hr)
		case intres.GitRepositoryKind:
			hr, err := intres.NewGitRepository(res)
			if err != nil {
				return nil, err
			}
			result.GitRepositories = append(result.GitRepositories, hr)
		case intres.KustomizationKind:
			hr, err := intres.NewKustomization(res)
			if err != nil {
				return nil, err
			}
			result.Kustomizations = append(result.Kustomizations, hr)
		case intres.HelmRepositoryKind:
			hr, err := intres.NewHelmRepository(res)
			if err != nil {
				return nil, err
			}
			result.HelmRepositories = append(result.HelmRepositories, hr)
		case intres.HelmReleaseKind:
			hr, err := intres.NewHelmRelease(res)
			if err != nil {
				return nil, err
			}
			result.HelmReleases = append(result.HelmReleases, hr)
		default:
			result.Resources = append(result.Resources, res)
		}
	}
	return result, nil
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

type ResultSet struct {
	Kustomizations   []*intres.Kustomization
	GitRepositories  []*intres.GitRepository
	OCIRepositories  []*intres.OCIRepository
	HelmReleases     []*intres.HelmRelease
	HelmRepositories []*intres.HelmRepository
	Resources        []*resource.Resource
}

func EmptyResultSet() *ResultSet {
	return &ResultSet{}
}

func (rs *ResultSet) Merge(other *ResultSet) {
	for _, v := range other.Kustomizations {
		rs.Kustomizations = append(rs.Kustomizations, v)
	}
	for _, v := range other.GitRepositories {
		rs.GitRepositories = append(rs.GitRepositories, v)
	}
	for _, v := range other.OCIRepositories {
		rs.OCIRepositories = append(rs.OCIRepositories, v)
	}
	for _, v := range other.HelmReleases {
		rs.HelmReleases = append(rs.HelmReleases, v)
	}
	for _, v := range other.HelmRepositories {
		rs.HelmRepositories = append(rs.HelmRepositories, v)
	}
	for _, v := range other.Resources {
		rs.Resources = append(rs.Resources, v)
	}
}

func orDefault[T comparable](a, def T) T {
	var zero T
	if a == zero {
		return def
	}
	return a
}
