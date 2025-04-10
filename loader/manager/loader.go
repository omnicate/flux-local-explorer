package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/kustomize"
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

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/loader"
	"github.com/omnicate/flx/loader/controller"
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

type Controller interface {
	Get(kind, namespace, name string) (any, error)
	Handle(rs *loader.ResultSet) (*loader.ResultSet, error)
}

type Loader struct {
	logger zerolog.Logger
	cs     client.Client
	root   *intres.Kustomization

	controllers []Controller

	helmTemplate bool
	helmRepos    []*intres.HelmRepository

	repoCachePath string
	repoReplace   []*controller.GitLocalReplace
	gitViaHTTPS   bool
}

func NewLoader(opts ...Option) *Loader {
	l := &Loader{
		logger:        zerolog.New(os.Stderr).With().Timestamp().Logger(),
		cs:            fake.NewClientBuilder().WithScheme(scheme).Build(),
		root:          nil,
		repoCachePath: "./cache",
		helmTemplate:  true,
	}

	for _, opt := range opts {
		opt(l)
	}

	l.controllers = []Controller{
		controller.NewExternalSecrets(
			l.logger.With().Str("controller", "external-secrets").Logger(),
		),
		controller.NewGit(
			l.logger.With().Str("controller", "git").Logger(),
			l.repoCachePath,
			l.repoReplace,
			l.gitViaHTTPS,
		),
		controller.NewOCI(
			l.logger.With().Str("controller", "oci").Logger(),
			l.repoCachePath,
		),
		controller.NewHelm(
			l.logger.With().Str("controller", "helm").Logger(),
			l.cs,
			l.repoCachePath,
		),
	}

	return l
}

func (l *Loader) Load(
	fs filesys.FileSystem,
	path string,
	defaultNamespace string,
) (*loader.ResultSet, error) {
	start := time.Now()

	direct, err := loader.LoadPath(fs, path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	resultSet, err := loader.NewResultSet(direct, defaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build root Kustomization: %w", err)
	}
	if err := l.handleResultSet(resultSet); err != nil {
		return nil, fmt.Errorf("could not build initial source set: %w", err)
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
				logger.Debug().Err(err).Msg("failed")
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

func (l *Loader) createClientResources(rs *loader.ResultSet) error {
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
	return nil
}

func (l *Loader) handleResultSet(rs *loader.ResultSet) error {
	if err := l.createClientResources(rs); err != nil {
		return err
	}

	for _, ctrl := range l.controllers {
		next, err := ctrl.Handle(rs)
		if err != nil {
			return err
		}
		if err := l.createClientResources(next); err != nil {
			return err
		}
		rs.Merge(next)
	}

	return nil
}

func (l *Loader) loadKustomization(
	ks *intres.Kustomization,
) (
	*loader.ResultSet,
	error,
) {

	// Load the root exactly once:
	if l.root == nil {
		l.root = ks
	} else if ks.Namespace == l.root.Namespace && ks.Name == l.root.Name {
		return nil, ErrSkip
	}

	repoName := fmt.Sprintf(
		"%s/%s/%s",
		ks.Spec.SourceRef.Kind,
		orDefault(ks.Spec.SourceRef.Namespace, ks.Namespace),
		ks.Spec.SourceRef.Name,
	)

	// Retrieve a filesystem from plugins

	var repoFS filesys.FileSystem
	for _, ctrl := range l.controllers {
		fileSys, err := ctrl.Get(
			ks.Spec.SourceRef.Kind,
			orDefault(ks.Spec.SourceRef.Namespace, ks.Namespace),
			ks.Spec.SourceRef.Name,
		)
		if err != nil {
			continue
		}
		repoFS = fileSys.(filesys.FileSystem)
		break
	}
	if repoFS == nil {
		return nil, fmt.Errorf("could not find source: %s", repoName)
	}

	resources, err := loader.LoadPath(repoFS, ks.Spec.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to load path: %v", err)
	}

	// Variable substitution:
	if pb := ks.Spec.PostBuild; pb != nil && (pb.SubstituteFrom != nil || pb.Substitute != nil) {
		ksYaml, err := sigyaml.Marshal(ks)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Kustomization: %v", err)
		}
		var ksObj unstructured.Unstructured
		if err := sigyaml.Unmarshal(ksYaml, &ksObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Kustomization: %v", err)
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
				return nil, err
			}
			resources[i] = newRes
		}
	}

	// Parse the resulting resources:
	targetNamespace := orDefault(ks.Spec.TargetNamespace, ks.Namespace)
	resultSet, err := loader.NewResultSet(resources, targetNamespace)
	if err != nil {
		return nil, err
	}

	// Handle them:
	if err := l.handleResultSet(resultSet); err != nil {
		return nil, err
	}

	ks.Resources = resultSet.Resources
	return resultSet, nil
}

func orDefault[T comparable](a, def T) T {
	var zero T
	if a == zero {
		return def
	}
	return a
}
