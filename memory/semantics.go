package memory

import (
	"bytes"
	"cmp"
	"time"
)

// EqualFunc reports whether two field values are equal.
type EqualFunc[V any] func(left, right V) bool

// CompareFunc compares two field values. It returns OrderUnordered when the
// domain does not define an ordering for the pair.
type CompareFunc[V any] func(left, right V) Ordering

// TextFunc projects a field value to the string used by literal text
// operators.
type TextFunc[V any] func(value V) string

// Semantics is an immutable set of borrowed value operations for a typed
// field. A nil operation means that its corresponding standard operators are
// not applicable to the field.
type Semantics[V any] struct {
	equal   EqualFunc[V]
	compare CompareFunc[V]
	text    TextFunc[V]
}

// NewSemantics returns an immutable configuration value using the supplied
// borrowed functions. Nil functions deliberately disable their corresponding
// operator families. Function panics are not recovered during Condition
// execution.
func NewSemantics[V any](
	equal EqualFunc[V],
	compare CompareFunc[V],
	text TextFunc[V],
) Semantics[V] {
	return Semantics[V]{
		equal:   equal,
		compare: compare,
		text:    text,
	}
}

// ComparableSemantics returns equality semantics for a comparable type.
func ComparableSemantics[V comparable]() Semantics[V] {
	return NewSemantics(
		func(left, right V) bool { return left == right },
		nil,
		nil,
	)
}

// OrderedSemantics returns equality and native Go ordering semantics. Values
// for which <, >, and == are all false compare as OrderUnordered.
func OrderedSemantics[V cmp.Ordered]() Semantics[V] {
	return NewSemantics(
		func(left, right V) bool { return left == right },
		func(left, right V) Ordering {
			switch {
			case left < right:
				return OrderLess
			case left > right:
				return OrderGreater
			case left == right:
				return OrderEqual
			default:
				return OrderUnordered
			}
		},
		nil,
	)
}

// StringSemantics returns equality, ordering, and literal text semantics for
// strings.
func StringSemantics() Semantics[string] {
	return NewSemantics(
		func(left, right string) bool { return left == right },
		func(left, right string) Ordering {
			switch {
			case left < right:
				return OrderLess
			case left > right:
				return OrderGreater
			default:
				return OrderEqual
			}
		},
		func(value string) string { return value },
	)
}

// TimeSemantics returns equality and chronological ordering semantics for
// time.Time values.
func TimeSemantics() Semantics[time.Time] {
	return NewSemantics(
		func(left, right time.Time) bool { return left.Equal(right) },
		func(left, right time.Time) Ordering {
			switch {
			case left.Before(right):
				return OrderLess
			case left.After(right):
				return OrderGreater
			default:
				return OrderEqual
			}
		},
		nil,
	)
}

// BytesSemantics returns byte-wise equality and lexicographic ordering
// semantics. It does not provide a text projection.
func BytesSemantics() Semantics[[]byte] {
	return NewSemantics(
		bytes.Equal,
		func(left, right []byte) Ordering {
			switch bytes.Compare(left, right) {
			case -1:
				return OrderLess
			case 1:
				return OrderGreater
			default:
				return OrderEqual
			}
		},
		nil,
	)
}
