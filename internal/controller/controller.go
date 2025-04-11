package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scheme to use for building k8s ClientSets.
var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
}

type Context interface {
	// ClientSet for a subset of resource kinds.
	ClientSet(kinds ...string) client.Client
	// GetAttachment for a specific resource.
	GetAttachment(kind, namespace, name string) (any, bool)
	// GetResource from the tree.
	GetResource(kind, namespace, name string) (*Resource, bool)
}

type Result struct {
	Resources  []*Resource
	Attachment any
}

type Controller interface {
	// Kinds this controller will accept.
	Kinds() []string
	// Reconcile is invoked for each resource. Returns a list of resources to create
	// and an error. If error is ErrSkip, the controller is skipped for this invocation.
	Reconcile(Context, *Resource) (*Result, error)
}

// Any returns the first non-zero value, or zero.
func Any[T comparable](items ...T) T {
	var zero T
	for _, item := range items {
		if item != zero {
			return item
		}
	}
	return zero
}
