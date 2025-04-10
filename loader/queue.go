package loader

import (
	intres "github.com/omnicate/flx/resource"
)

type Queue struct {
	items []*QueueItem
}

type QueueItem struct {
	Value   *intres.Kustomization
	Attempt int
}

func (i *QueueItem) NamespacedName() string {
	return i.Value.GetNamespace() + "/" + i.Value.GetName()
}

func (q *Queue) Pop() (*QueueItem, bool) {
	if len(q.items) == 0 {
		return nil, false
	}
	item := q.items[0]
	item.Value.Error = nil
	q.items = q.items[1:]
	return item, true
}

func (q *Queue) Push(item *QueueItem) {
	q.items = append(q.items, item)
}

func (q *Queue) Retry(item *QueueItem, err error) bool {
	item.Attempt += 1
	item.Value.Error = err

	if item.Attempt >= maxAttempts {
		return false
	}
	q.Push(item)
	return true
}

type NamedResource interface {
	GetName() string
	GetNamespace() string
}

func namespacedName(r NamedResource) string {
	return r.GetNamespace() + "/" + r.GetName()
}
