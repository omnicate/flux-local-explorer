package resource

import (
	"fmt"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"sigs.k8s.io/kustomize/api/resource"
)

const KustomizationKind = kustomizev1.KustomizationKind

type Kustomization struct {
	*kustomizev1.Kustomization

	Resources []*resource.Resource
	Error     error
}

func NewKustomization(res *resource.Resource) (*Kustomization, error) {
	if res.GetKind() != KustomizationKind {
		return nil, fmt.Errorf("kind != Kustomization")
	}
	result := &Kustomization{}
	switch res.GetApiVersion() {
	case kustomizev1.GroupVersion.String():
		if err := unmarshalInto(res, &result.Kustomization); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported kustomization version %s", res.GetApiVersion())
	}
	return result, nil
}
