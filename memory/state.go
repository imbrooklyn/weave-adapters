package memory

import "strconv"

// State describes whether an accessor found a value, an explicit null, or no
// field at all. Its zero value is invalid.
type State uint8

const (
	// StateValue indicates that the accessor returned a present, non-null value.
	StateValue State = iota + 1
	// StateNull indicates that the field is present with an explicit null value.
	StateNull
	// StateMissing indicates that the field is not present in the record.
	StateMissing
)

// Valid reports whether s is a recognized field state.
func (s State) Valid() bool {
	switch s {
	case StateValue, StateNull, StateMissing:
		return true
	default:
		return false
	}
}

// String returns a stable English diagnostic identifier for s.
func (s State) String() string {
	switch s {
	case StateValue:
		return "value"
	case StateNull:
		return "null"
	case StateMissing:
		return "missing"
	default:
		return "state(" + strconv.FormatUint(uint64(s), 10) + ")"
	}
}

// Accessor reads one typed field from a record. The value is meaningful only
// when the returned state is StateValue. An Accessor runs once for each
// evaluated standard leaf and is not cached across nodes. Accessors are
// borrowed by Field and must be deterministic and safe for the caller's
// intended concurrency.
type Accessor[R, V any] func(record R) (V, State)

// Ordering is the result of a field-specific comparison.
type Ordering int8

const (
	// OrderLess indicates that the left value sorts before the right value.
	OrderLess Ordering = -1
	// OrderEqual indicates that the values compare equal.
	OrderEqual Ordering = 0
	// OrderGreater indicates that the left value sorts after the right value.
	OrderGreater Ordering = 1
	// OrderUnordered indicates that the values have no defined ordering, as with
	// comparisons involving a floating-point NaN.
	OrderUnordered Ordering = 2
)

// Valid reports whether o is a recognized ordering result.
func (o Ordering) Valid() bool {
	switch o {
	case OrderLess, OrderEqual, OrderGreater, OrderUnordered:
		return true
	default:
		return false
	}
}

// String returns a stable English diagnostic identifier for o.
func (o Ordering) String() string {
	switch o {
	case OrderLess:
		return "less"
	case OrderEqual:
		return "equal"
	case OrderGreater:
		return "greater"
	case OrderUnordered:
		return "unordered"
	default:
		return "ordering(" + strconv.FormatInt(int64(o), 10) + ")"
	}
}
