package memory

import "errors"

var (
	// ErrInvalidState indicates that an accessor returned an unrecognized State.
	ErrInvalidState = errors.New("memory: invalid field state")
	// ErrInvalidOrdering indicates that a CompareFunc returned an unrecognized
	// Ordering value.
	ErrInvalidOrdering = errors.New("memory: invalid ordering")
	// ErrNilCondition indicates an attempt to execute a nil Condition.
	ErrNilCondition = errors.New("memory: nil condition")
	// ErrNilExpression indicates an attempt to execute a nil Expression.
	ErrNilExpression = errors.New("memory: nil expression")
)

var errInvalidOperator = errors.New("memory: invalid operator")

// Condition evaluates one record and returns its match result. An error means
// that evaluation did not produce a match decision and propagates through
// enclosing groups unchanged.
type Condition[R any] func(record R) (bool, error)

// Match evaluates record. A nil Condition returns ErrNilCondition instead of
// panicking.
func (c Condition[R]) Match(record R) (bool, error) {
	if c == nil {
		return false, ErrNilCondition
	}
	return c(record)
}

// Expression is a caller-supplied, record-level Boolean expression. Compiler
// accepts a non-nil Expression directly without wrapping or interpreting its
// Boolean meaning. Its validity, deterministic behavior, and captured-state
// concurrency are caller responsibilities.
type Expression[R any] func(record R) (bool, error)

// Evaluate evaluates record. A nil Expression returns ErrNilExpression instead
// of panicking.
func (e Expression[R]) Evaluate(record R) (bool, error) {
	if e == nil {
		return false, ErrNilExpression
	}
	return e(record)
}
