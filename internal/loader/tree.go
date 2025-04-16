package loader

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/omnicate/flx/internal/controller"
)

type ResourceStatus int

const (
	StatusUnknown ResourceStatus = iota
	StatusCompleted
	StatusError
)

type ResourceNode struct {
	// Status of this particular node/resource.
	Status ResourceStatus
	// Attempts taken to reconcile.
	Attempts int
	// Error that occurred during reconciliation.
	Error error
	// Time it took to reconcile this resource.
	Duration time.Duration
	// Resource this node contains, or nil for the root.
	Resource *controller.Resource
	// Attachment set on this node.
	Attachment any
	// Children of this resource.
	Children []*ResourceNode
}

var (
	// ErrStop is used during Walk to stop recursing.
	ErrStop = errors.New("stop")
)

// Walk the resource node recursively.
func (n *ResourceNode) Walk(f func(node *ResourceNode) error) error {
	err := f(n)
	if errors.Is(err, ErrStop) {
		return err
	} else if err != nil {
		return err
	}
	for _, child := range n.Children {
		err = child.Walk(f)
		if errors.Is(err, ErrStop) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

// ResourceNodes is a slice of ResourceNodes.
type ResourceNodes []*ResourceNode

// FlatByStatus returns a flat list of all resource nodes, that have a specific status.
func (n *ResourceNode) FlatByStatus(status ResourceStatus) ResourceNodes {
	flat := make(ResourceNodes, 0)
	_ = n.Walk(func(node *ResourceNode) error {
		if node.Resource == nil {
			return nil
		}
		if node.Status != status {
			return nil
		}
		flat = append(flat, node)
		return nil
	})
	return flat
}

// Flat returns a flat list of all resource nodes, that have a resource attached.
func (n *ResourceNode) Flat() ResourceNodes {
	flat := make(ResourceNodes, 0)
	_ = n.Walk(func(node *ResourceNode) error {
		if node.Resource == nil {
			return nil
		}
		flat = append(flat, node)
		return nil
	})
	return flat
}

// FilterByKind returns only nodes that are of a particular kind.
func (n ResourceNodes) FilterByKind(kind string) ResourceNodes {
	out := make([]*ResourceNode, 0)
	for i := range n {
		res := n[i].Resource
		if res == nil {
			continue
		}
		if res.GetKind() == kind {
			out = append(out, n[i])
		}
	}
	return out
}

// FilterByNamespace returns only nodes in a particular namespace.
func (n ResourceNodes) FilterByNamespace(ns string) ResourceNodes {
	out := make([]*ResourceNode, 0)
	for i := range n {
		res := n[i].Resource
		if res == nil {
			continue
		}
		if res.GetNamespace() == ns {
			out = append(out, n[i])
		}
	}
	return out
}

// GetResources retrieves all resources that are children of this node, recursively.
// Recursion is stopped when a `Kustomization` is encountered.
func (n *ResourceNode) GetResources() []*ResourceNode {
	// Always include the root.
	flat := []*ResourceNode{n}

	// Collect all child nodes that are not "Kustomizations":
	for _, child := range n.Children {
		if child.Resource.GetKind() == "Kustomization" {
			continue
		}
		flat = append(flat, child.GetResources()...)
	}

	return flat
}

// Find a specific node in the ResourceNode.
func (n *ResourceNode) Find(kind, namespace, name string) (*ResourceNode, bool) {
	var found *ResourceNode
	_ = n.Walk(func(node *ResourceNode) error {
		r := node.Resource
		if r == nil {
			return nil
		}
		if r.GetKind() != kind {
			return nil
		}
		if r.GetNamespace() != namespace {
			return nil
		}
		if r.GetName() != name {
			return nil
		}
		found = node
		return fmt.Errorf("stop")
	})
	if found == nil {
		return nil, false
	}
	return found, true
}

// AddResources to a node. Created resources will have StatusUnknown.
func (n *ResourceNode) AddResources(res []*controller.Resource) {
	for _, r := range res {
		n.Children = append(n.Children, &ResourceNode{
			Resource: r,
			Status:   StatusUnknown,
		})
	}
}

// String representation of a resource node.
func (n *ResourceNode) String() string {
	return n.string(0)
}

func (n *ResourceNode) string(indent int) string {
	var displayName string = "root"
	if n.Resource != nil {
		displayName = fmt.Sprintf(
			"%s %s/%s %v (%d)",
			n.Resource.GetKind(),
			n.Resource.GetNamespace(),
			n.Resource.GetName(),
			n.Error,
			n.Attempts,
		)
	}
	s := strings.Repeat(" ", indent) + displayName
	for _, child := range n.Children {
		s += "\n" + child.string(indent+2)
	}
	return s
}
