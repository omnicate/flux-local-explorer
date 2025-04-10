package resource

import (
	"fmt"

	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"sigs.k8s.io/kustomize/api/resource"
)

const OCIRepositoryKind = sourcev1b2.OCIRepositoryKind

type OCIRepository struct {
	*sourcev1b2.OCIRepository

	Error error
}

func NewOCIRepository(res *resource.Resource) (*OCIRepository, error) {
	if res.GetKind() != OCIRepositoryKind {
		return nil, fmt.Errorf("kind != OCIRepository")
	}
	result := &OCIRepository{}
	switch res.GetApiVersion() {
	case sourcev1b2.GroupVersion.String():
		if err := unmarshalInto(res, &result.OCIRepository); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported OCIRepository version %s", res.GetApiVersion())
	}
	return result, nil
}
