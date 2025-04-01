package loader

import (
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	"sigs.k8s.io/kustomize/api/resource"
)

// Kustomization and error.
// See https://fluxcd.io/flux/components/kustomize/kustomizations/.
type Kustomization struct {
	*kustomizev1.Kustomization

	// Error encountered while executing this kustomization.
	Error error
	// Resources emitted by this Kustomization. Does not include resources
	// emitted by child Kustomizations.
	Resources []*resource.Resource
}

// GitRepository and error.
// See https://fluxcd.io/flux/components/source/gitrepositories/.
type GitRepository struct {
	*sourcev1.GitRepository

	// Error encountered while instantiating this repository.
	Error error
}

// OCIRepository and error.
// See https://fluxcd.io/flux/components/source/ocirepositories/.
type OCIRepository struct {
	*sourcev1b2.OCIRepository

	// Error encountered while instantiating this repository.
	Error error
}
