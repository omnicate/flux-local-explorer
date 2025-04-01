package loader

import (
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	"sigs.k8s.io/kustomize/api/resource"
)

type NamedResource interface {
	GetName() string
	GetNamespace() string
}

type Queue struct {
	items []*QueueItem
}

type QueueItem struct {
	Value   NamedResource
	Attempt int
	Err     error
}

func (i *QueueItem) NamespacedName() string {
	return i.Value.GetNamespace() + "/" + i.Value.GetName()
}

func (i *QueueItem) Kind() string {
	switch i.Value.(type) {
	case *kustomizev1.Kustomization:
		return "Kustomization"
	case *sourcev1.GitRepository:
		return "GitRepository"
	case *sourcev1b2.OCIRepository:
		return "OCIRepository"
	case *resource.Resource:
		return "Resource"
	}
	panic("unknown queue item")
}

func (q *Queue) Pop() (*QueueItem, bool) {
	if len(q.items) == 0 {
		return nil, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *Queue) Push(item *QueueItem) {
	q.items = append(q.items, item)
}

func (q *Queue) Retry(item *QueueItem, err error) {
	item.Attempt += 1
	item.Err = err
	q.Push(item)
}

func namespacedName(r NamedResource) string {
	return r.GetNamespace() + "/" + r.GetName()
}
