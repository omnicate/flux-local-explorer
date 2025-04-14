package loader

import (
	"errors"
	"fmt"
	"strings"

	"github.com/omnicate/flx/internal/controller"
)

type ResourceStatus int

const (
	StatusUnknown ResourceStatus = iota
	StatusCompleted
	StatusError
)

type ResourceNode struct {
	Status   ResourceStatus
	Attempts int
	Error    error

	// Resource this node contains.
	Resource *controller.Resource
	// Attachment set on this node.
	Attachment any

	// Children of this resource.
	Children []*ResourceNode
}

var (
	ErrStop    = errors.New("stop")
	ErrStopAll = errors.New("stop all")
)

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

func (n *ResourceNode) ListNodes(kind, namespace string, allNamespaces bool) []*ResourceNode {
	flat := make([]*ResourceNode, 0)
	n.Walk(func(node *ResourceNode) error {
		if node.Resource == nil {
			return nil
		}
		if node.Resource.GetKind() != kind {
			return nil
		}
		if !allNamespaces && node.Resource.GetNamespace() != namespace {
			return nil
		}
		flat = append(flat, node)
		return nil
	})
	return flat
}

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

func (n *ResourceNode) AddResources(res []*controller.Resource) {
	for _, r := range res {
		n.Children = append(n.Children, &ResourceNode{
			Resource: r,
			Status:   StatusUnknown,
		})
	}
}

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
