package controller

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"
)

type Resource struct {
	*resource.Resource
}

func NewResource(r *resource.Resource) *Resource {
	return &Resource{Resource: r}
}

func NewResources(r []*resource.Resource) []*Resource {
	out := make([]*Resource, len(r))
	for i := range r {
		out[i] = NewResource(r[i])
	}
	return out
}

func (r Resource) Unstructured() (*unstructured.Unstructured, error) {
	var obj unstructured.Unstructured
	return &obj, r.Unmarshal(&obj)
}

func (r Resource) Unmarshal(dest any) error {
	data, err := r.Resource.AsYAML()
	if err != nil {
		return err
	}
	return sigyaml.Unmarshal(data, dest)
}
