package controller

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"
)

// Resource that contains a single arbitrary k8s manifest.
type Resource struct {
	*resource.Resource
}

// NewResource from a kustomize resource.
func NewResource(r *resource.Resource) *Resource {
	return &Resource{Resource: r}
}

// NewResources from kustomize resources.
func NewResources(r []*resource.Resource) []*Resource {
	out := make([]*Resource, len(r))
	for i := range r {
		out[i] = NewResource(r[i])
	}
	return out
}

// Unstructured representation of this resource.
func (r Resource) Unstructured() (*unstructured.Unstructured, error) {
	var obj unstructured.Unstructured
	return &obj, r.Unmarshal(&obj)
}

// Unmarshal resource into dest.
func (r Resource) Unmarshal(dest any) error {
	data, err := r.Resource.AsYAML()
	if err != nil {
		return err
	}
	return sigyaml.Unmarshal(data, dest)
}
