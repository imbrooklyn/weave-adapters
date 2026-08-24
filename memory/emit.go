package memory

import (
	"errors"

	"github.com/imbrooklyn/weave"
)

var errUnexpectedEmitPlan = errors.New("memory: invalid validated emission plan")

func emitPredicate[R any](validated validatedNode[R]) (Condition[R], error) {
	switch validated.kind {
	case weave.KindConstant:
		value := validated.constant
		return func(R) (bool, error) { return value, nil }, nil

	case weave.KindGroup:
		children := make([]Condition[R], len(validated.children))
		for index := range validated.children {
			child, err := emitPredicate(validated.children[index])
			if err != nil {
				return nil, err
			}
			children[index] = child
		}
		condition := combineConditions(validated.logic, children)
		if condition == nil {
			return nil, emissionError(validated)
		}
		return condition, nil

	case weave.KindComparison,
		weave.KindMembership,
		weave.KindRange,
		weave.KindNull,
		weave.KindText,
		weave.KindNativeCondition,
		weave.KindNativeExpression:
		if validated.leaf == nil {
			return nil, emissionError(validated)
		}
		condition := validated.leaf()
		if condition == nil {
			return nil, emissionError(validated)
		}
		return condition, nil

	default:
		return nil, emissionError(validated)
	}
}

func combineConditions[R any](
	logic weave.Logic,
	children []Condition[R],
) Condition[R] {
	switch logic {
	case weave.LogicAllOf:
		return func(record R) (bool, error) {
			for _, child := range children {
				matched, err := child(record)
				if err != nil || !matched {
					return false, err
				}
			}
			return true, nil
		}

	case weave.LogicAnyOf:
		return func(record R) (bool, error) {
			for _, child := range children {
				matched, err := child(record)
				if err != nil {
					return false, err
				}
				if matched {
					return true, nil
				}
			}
			return false, nil
		}

	case weave.LogicNoneOf:
		return func(record R) (bool, error) {
			for _, child := range children {
				matched, err := child(record)
				if err != nil {
					return false, err
				}
				if matched {
					return false, nil
				}
			}
			return true, nil
		}

	case weave.LogicNotAllOf:
		return func(record R) (bool, error) {
			for _, child := range children {
				matched, err := child(record)
				if err != nil {
					return false, err
				}
				if !matched {
					return true, nil
				}
			}
			return false, nil
		}

	default:
		return nil
	}
}

func emissionError[R any](validated validatedNode[R]) *weave.Error {
	return &weave.Error{
		Code:     weave.CodeCompileFailure,
		Phase:    weave.PhaseEmit,
		Path:     validated.path,
		Origin:   validated.origin,
		Operator: validated.operator,
		Feature:  validated.feature,
		Cause:    errUnexpectedEmitPlan,
	}
}
