package loader

import (
	"fmt"
	"iter"
)

type ErrSeq[T any] iter.Seq2[T, error]

func MapIf[T, Q any](seq ErrSeq[T], f func(T) (Q, bool)) ErrSeq[Q] {
	return func(yield func(Q, error) bool) {
		for item, err := range seq {
			if err != nil {
				var zero Q
				yield(zero, err)
				return
			}
			mapped, ok := f(item)
			if !ok {
				continue
			}
			if !yield(mapped, nil) {
				return
			}
		}
	}
}

func (seq ErrSeq[T]) Collect() ([]T, error) {
	var result []T
	for item, err := range seq {
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (seq ErrSeq[T]) Filter(f func(T) bool) ErrSeq[T] {
	return func(yield func(T, error) bool) {
		for item, err := range seq {
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			if !f(item) {
				continue
			}
			if !yield(item, nil) {
				return
			}
		}
	}
}

func (seq ErrSeq[T]) Find(f func(T) bool) (T, error) {
	for item, err := range seq {
		if err != nil {
			var zero T
			return zero, err
		}
		if f(item) {
			return item, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("not found")
}

func typedIter[T NamedResource](seq ErrSeq[NamedResource]) ErrSeq[T] {
	return MapIf(seq, func(nr NamedResource) (T, bool) {
		ks, ok := nr.(T)
		return ks, ok
	})
}
