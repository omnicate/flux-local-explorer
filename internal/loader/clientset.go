package loader

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ client.Client = new(ClientSet)

// ClientSet implements client.Client interface, handled Get and GroupVersionKindFor.
type ClientSet struct {
	client.Client

	scheme *runtime.Scheme
	tree   *ResourceNode
}

// NewClientSet form scheme and resource tree.
func NewClientSet(scheme *runtime.Scheme, root *ResourceNode) *ClientSet {
	return &ClientSet{
		scheme: scheme,
		tree:   root,
	}
}

// Get a particular resource from the tree.
func (c ClientSet) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}
	node, ok := c.tree.Find(gvk.Kind, key.Namespace, key.Name)
	if !ok {
		return fmt.Errorf("object not found")
	}
	return node.Resource.Unmarshal(obj)
}

// GroupVersionKindFor an object.
func (c ClientSet) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	kinds, _, err := c.scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return kinds[0], nil
}
