package controller

import (
	"errors"
)

var ErrNotFound = errors.New("not found")

func orDefault[T comparable](a, def T) T {
	var zero T
	if a == zero {
		return def
	}
	return a
}

type namedResource interface {
	GetName() string
	GetNamespace() string
}

func namespacedName(r namedResource) string {
	return r.GetNamespace() + "/" + r.GetName()
}
