package resource

import (
	"fmt"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b1 "github.com/fluxcd/source-controller/api/v1beta1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"sigs.k8s.io/kustomize/api/resource"
)

const HelmRepositoryKind = sourcev1.HelmRepositoryKind

type HelmRepository struct {
	ObjectMeta

	Type string
	URL  string

	Error error
}

func NewHelmRepository(res *resource.Resource) (*HelmRepository, error) {
	if res.GetKind() != HelmRepositoryKind {
		return nil, fmt.Errorf("kind != HelmRepository")
	}
	result := &HelmRepository{
		ObjectMeta: ObjectMeta{
			Name:      res.GetName(),
			Namespace: res.GetNamespace(),
		},
	}
	switch res.GetApiVersion() {
	case sourcev1.GroupVersion.String():
		var hr sourcev1.HelmRepository
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.URL = hr.Spec.URL
	case sourcev1b1.GroupVersion.String():
		var hr sourcev1b1.HelmRepository
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.Type = "default"
		result.URL = hr.Spec.URL
	case sourcev1b2.GroupVersion.String():
		var hr sourcev1b2.HelmRepository
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.Type = hr.Spec.Type
		result.URL = hr.Spec.URL
	default:
		return nil, fmt.Errorf("unsupported api version: %s", res.GetApiVersion())
	}
	return result, nil
}
