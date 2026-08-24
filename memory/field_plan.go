package memory

import (
	"strings"

	"github.com/imbrooklyn/weave"
)

type conditionPlan[R any] func() Condition[R]

type runtimeField[R any] interface {
	fieldDescriptor
	memoryComparisonPlan(weave.Operator, any) (conditionPlan[R], bool)
	memoryMembershipPlan(weave.Operator, []any) (conditionPlan[R], bool)
	memoryRangePlan(any, any) (conditionPlan[R], bool)
	memoryNullPlan(weave.Operator) conditionPlan[R]
	memoryTextPlan(weave.Operator, string) conditionPlan[R]
}

func (f Field[R, V]) memoryComparisonPlan(
	operator weave.Operator,
	value any,
) (conditionPlan[R], bool) {
	operand, ok := value.(V)
	if !ok {
		return nil, false
	}

	return func() Condition[R] {
		return func(record R) (bool, error) {
			return f.evaluateValue(record, func(fieldValue V) (bool, error) {
				switch operator {
				case weave.OperatorEQ:
					return f.semantics.equal(fieldValue, operand), nil
				case weave.OperatorNEQ:
					return !f.semantics.equal(fieldValue, operand), nil
				case weave.OperatorLT,
					weave.OperatorLTE,
					weave.OperatorGT,
					weave.OperatorGTE:
					return orderingMatches(
						f.semantics.compare(fieldValue, operand),
						operator,
					)
				default:
					return false, errInvalidOperator
				}
			})
		}
	}, true
}

func (f Field[R, V]) memoryMembershipPlan(
	operator weave.Operator,
	values []any,
) (conditionPlan[R], bool) {
	operands := make([]V, len(values))
	for index, value := range values {
		typed, ok := value.(V)
		if !ok {
			return nil, false
		}
		operands[index] = typed
	}

	return func() Condition[R] {
		return func(record R) (bool, error) {
			return f.evaluateValue(record, func(fieldValue V) (bool, error) {
				contained := false
				for _, operand := range operands {
					if f.semantics.equal(fieldValue, operand) {
						contained = true
						break
					}
				}
				switch operator {
				case weave.OperatorIn:
					return contained, nil
				case weave.OperatorNotIn:
					return !contained, nil
				default:
					return false, errInvalidOperator
				}
			})
		}
	}, true
}

func (f Field[R, V]) memoryRangePlan(
	lower any,
	upper any,
) (conditionPlan[R], bool) {
	typedLower, lowerOK := lower.(V)
	typedUpper, upperOK := upper.(V)
	if !lowerOK || !upperOK {
		return nil, false
	}

	return func() Condition[R] {
		return func(record R) (bool, error) {
			return f.evaluateValue(record, func(fieldValue V) (bool, error) {
				lowerMatch, err := orderingMatches(
					f.semantics.compare(fieldValue, typedLower),
					weave.OperatorGTE,
				)
				if err != nil || !lowerMatch {
					return false, err
				}
				return orderingMatches(
					f.semantics.compare(fieldValue, typedUpper),
					weave.OperatorLTE,
				)
			})
		}
	}, true
}

func (f Field[R, V]) memoryNullPlan(
	operator weave.Operator,
) conditionPlan[R] {
	return func() Condition[R] {
		return func(record R) (bool, error) {
			_, state := f.accessor(record)
			switch state {
			case StateValue:
				return operator == weave.OperatorNotNull, nil
			case StateNull:
				return operator == weave.OperatorIsNull, nil
			case StateMissing:
				return false, nil
			default:
				return false, ErrInvalidState
			}
		}
	}
}

func (f Field[R, V]) memoryTextPlan(
	operator weave.Operator,
	operand string,
) conditionPlan[R] {
	return func() Condition[R] {
		return func(record R) (bool, error) {
			return f.evaluateValue(record, func(fieldValue V) (bool, error) {
				text := f.semantics.text(fieldValue)
				switch operator {
				case weave.OperatorContains:
					return strings.Contains(text, operand), nil
				case weave.OperatorHasPrefix:
					return strings.HasPrefix(text, operand), nil
				case weave.OperatorHasSuffix:
					return strings.HasSuffix(text, operand), nil
				default:
					return false, errInvalidOperator
				}
			})
		}
	}
}

func (f Field[R, V]) evaluateValue(
	record R,
	evaluate func(V) (bool, error),
) (bool, error) {
	value, state := f.accessor(record)
	switch state {
	case StateValue:
		return evaluate(value)
	case StateNull, StateMissing:
		return false, nil
	default:
		return false, ErrInvalidState
	}
}

func orderingMatches(
	ordering Ordering,
	operator weave.Operator,
) (bool, error) {
	if !ordering.Valid() {
		return false, ErrInvalidOrdering
	}
	if ordering == OrderUnordered {
		return false, nil
	}

	switch operator {
	case weave.OperatorLT:
		return ordering == OrderLess, nil
	case weave.OperatorLTE:
		return ordering == OrderLess || ordering == OrderEqual, nil
	case weave.OperatorGT:
		return ordering == OrderGreater, nil
	case weave.OperatorGTE:
		return ordering == OrderGreater || ordering == OrderEqual, nil
	default:
		return false, errInvalidOperator
	}
}
