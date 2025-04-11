package helm

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	helmv2b1 "github.com/fluxcd/helm-controller/api/v2beta1"
	helmv2b2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/chartutil"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigyaml "sigs.k8s.io/yaml"

	ctrl "github.com/omnicate/flx/internal/controller"
	"github.com/omnicate/flx/internal/loader"
)

func init() {
	_ = helmv2.AddToScheme(ctrl.Scheme)
	_ = helmv2b1.AddToScheme(ctrl.Scheme)
	_ = helmv2b2.AddToScheme(ctrl.Scheme)
}

var _ ctrl.Controller = new(Controller)

type Controller struct {
	logger zerolog.Logger
	opts   Options
	mu     sync.Mutex
}

func NewController(logger zerolog.Logger, opts Options) *Controller {
	return &Controller{logger: logger, opts: opts}
}

type helmRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec struct {
		Chart struct {
			Spec struct {
				Chart     string `json:"chart"`
				Version   string `json:"version,omitempty"`
				SourceRef struct {
					APIVersion string `json:"apiVersion,omitempty"`
					Kind       string `json:"kind,omitempty"`
					Name       string `json:"name,omitempty"`
					Namespace  string `json:"namespace,omitempty"`
				} `json:"sourceRef"`
			} `json:"spec"`
		} `json:"chart"`
		Values     map[string]any         `json:"values,omitempty"`
		ValuesFrom []meta.ValuesReference `json:"valuesFrom,omitempty"`
	} `json:"spec"`
}

type helmRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec struct {
		Type string `yaml:"type,omitempty"`
		URL  string `yaml:"url,omitempty"`
	} `json:"spec"`
}

func (r *Controller) Kinds() []string {
	return []string{"HelmRelease"}
}

func (r *Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var hr helmRelease
	if err := req.Unmarshal(&hr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal helm release: %w", err)
	}

	if hr.Spec.Chart.Spec.Chart == "" {
		return nil, fmt.Errorf("missing: spec.chart.spec.chart")
	}
	if hr.Spec.Chart.Spec.Version == "" {
		return nil, fmt.Errorf("missing: spec.chart.spec.version")
	}
	if hr.Spec.Chart.Spec.SourceRef.Name == "" {
		return nil, fmt.Errorf("missing: spec.chart.spec.sourceRef.name")
	}
	if hr.Spec.Chart.Spec.SourceRef.Kind == "" {
		return nil, fmt.Errorf("missing: spec.chart.spec.sourceRef.kind")
	}

	repoResource, ok := ctx.GetResource(
		ctrl.Any(hr.Spec.Chart.Spec.SourceRef.Kind, "HelmRepository"),
		ctrl.Any(hr.Spec.Chart.Spec.SourceRef.Namespace, hr.Namespace),
		hr.Spec.Chart.Spec.SourceRef.Name,
	)
	if !ok {
		return nil, fmt.Errorf("missing helm repository")
	}
	var repo helmRepository
	if err := repoResource.Unmarshal(&repo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal helm repository: %w", err)
	}

	// Download helm chart:

	md5h := md5.Sum([]byte(repo.Spec.URL + "@" + hr.Spec.Chart.Spec.Version))
	hash := hex.EncodeToString(md5h[:])
	cachePath := filepath.Join(
		r.opts.CachePath,
		hr.Spec.Chart.Spec.Chart+"-"+hr.Spec.Chart.Spec.Version+"-"+hash,
	)

	// Run `helm pull` command.
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		var cmd *exec.Cmd
		if repo.Spec.Type == "oci" {
			cmd = exec.Command(
				"helm",
				"pull",
				"--version", hr.Spec.Chart.Spec.Version,
				"--untar",
				"--untardir", cachePath,
				repo.Spec.URL+"/"+hr.Spec.Chart.Spec.Chart,
			)
		} else {
			cmd = exec.Command(
				"helm",
				"pull",
				"--repo", repo.Spec.URL,
				"--version", hr.Spec.Chart.Spec.Version,
				"--untar",
				"--untardir", cachePath,
				hr.Spec.Chart.Spec.Chart,
			)
		}
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr

		r.logger.Debug().
			Str("path", cachePath).
			Str("name", hr.Name).
			Str("cmd", cmd.String()).
			Str("namespace", hr.Namespace).
			Msg("pulling")

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("helm pull download failed: %w", err)
		}
	}

	values, err := chartutil.ChartValuesFromReferences(
		context.Background(),
		zerologr.New(&r.logger),
		ctx.ClientSet("Secret", "ConfigMap"),
		hr.Namespace,
		hr.Spec.Values,
		hr.Spec.ValuesFrom...,
	)
	if err != nil {
		return nil, fmt.Errorf("values from references: %w", err)
	}

	// Write values to some file.
	data, err := sigyaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshalling values.yaml: %w", err)
	}
	valuesMd5h := md5.Sum(data)
	valuesHash := hex.EncodeToString(valuesMd5h[:])
	valuesFile := filepath.Join(cachePath, "values-"+valuesHash+".yaml")
	if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
		if err := os.WriteFile(valuesFile, data, 0600); err != nil {
			return nil, fmt.Errorf("writing values.yaml: %w", err)
		}
	}

	// Run `helm template` command.
	var out bytes.Buffer
	cmd := exec.Command(
		"helm",
		"template",
		"-n", hr.Namespace,
		"--name-template", hr.Name,
		"-f", valuesFile,
		filepath.Join(cachePath, filepath.Base(hr.Spec.Chart.Spec.Chart)),
	)
	r.logger.Debug().
		Str("path", cachePath).
		Str("name", hr.Name).
		Str("cmd", cmd.String()).
		Str("namespace", hr.Namespace).
		Msg("templating helm chart")

	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	cmd.Env = []string{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template: %w", err)
	}

	resources, err := loader.LoadBytes(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("loading resources from helm: %w", err)
	}

	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(hr.Namespace)
		}
	}

	return &ctrl.Result{
		Resources:  ctrl.NewResources(resources),
		Attachment: nil,
	}, nil
}
