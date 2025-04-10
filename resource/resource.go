package resource

import (
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"
)

func unmarshalInto[T any](res *resource.Resource, dest *T) error {
	data, err := res.AsYAML()
	if err != nil {
		return err
	}
	return sigyaml.Unmarshal(data, dest)
}
