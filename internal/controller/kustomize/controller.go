package kustomize

import (
	"context"
	"fmt"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/kustomize"
	"github.com/rs/zerolog"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flx/internal/controller"
	"github.com/omnicate/flx/internal/loader"
)

func init() {
	_ = kustomizev1.AddToScheme(ctrl.Scheme)
}

type Controller struct {
	logger zerolog.Logger
}

func NewController(logger zerolog.Logger) *Controller {
	return &Controller{logger: logger}
}

func (r Controller) Kinds() []string {
	return []string{"Kustomization"}
}

func (r Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	var ks kustomizev1.Kustomization
	if err := req.Unmarshal(&ks); err != nil {
		return nil, err
	}

	logger := r.logger.With().
		Str("namespace", ks.Namespace).
		Str("name", ks.Name).
		Logger()

	startTime := time.Now()

	// Get source repository:
	sourceAttachment, ok := ctx.GetAttachment(
		ks.Spec.SourceRef.Kind,
		ctrl.Any(ks.Spec.SourceRef.Namespace, ks.Namespace),
		ks.Spec.SourceRef.Name,
	)
	if !ok {
		return nil, fmt.Errorf(
			"failed to find source repository %s/%s",
			ctrl.Any(ks.Spec.SourceRef.Namespace, ks.Namespace),
			ks.Spec.SourceRef.Name,
		)
	}
	repoFS, ok := sourceAttachment.(filesys.FileSystem)
	if !ok {
		return nil, fmt.Errorf(
			"source repo attachment of invalid type: %T",
			sourceAttachment,
		)
	}

	// Load resources:
	resources, err := loader.LoadPath(repoFS, ks.Spec.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to load path: %v", err)
	}

	// Variable substitution:
	if pb := ks.Spec.PostBuild; pb != nil && (pb.SubstituteFrom != nil || pb.Substitute != nil) {
		obj, err := req.Unstructured()
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal Kustomization: %v", err)
		}
		clientSet := ctx.ClientSet("Secret", "ConfigMap")
		for i, res := range resources {
			newRes, err := kustomize.SubstituteVariables(
				context.Background(),
				clientSet,
				*obj,
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
	targetNamespace := ctrl.Any(ks.Spec.TargetNamespace, ks.Namespace)
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(targetNamespace)
		}
	}

	logger.Debug().
		Str("elapsed", time.Since(startTime).String()).
		Msg("loaded")

	// TODO: add load functions to directly use Resource.
	//  Only the KS controller should need to load resources. Ever.
	return &ctrl.Result{Resources: ctrl.NewResources(resources)}, nil
}
