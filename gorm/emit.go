package gorm

import (
	"errors"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

var errUnexpectedEmitPlan = errors.New("gorm: invalid validated emission plan")

func emitPredicate(validated validatedNode) (Condition, error) {
	if validated.kind != weave.KindGroup ||
		validated.logic != weave.LogicAllOf {
		return nil, emissionError(validated)
	}
	if len(validated.children) == 0 {
		return newConstant(true), nil
	}

	children := make([]clause.Expression, len(validated.children))
	for index := range validated.children {
		child, err := emitExpression(validated.children[index])
		if err != nil {
			return nil, err
		}
		if isNilLike(child) {
			return nil, emissionError(validated.children[index])
		}
		children[index] = child
	}
	expression := clause.And(children...)
	if isNilLike(expression) {
		return nil, emissionError(validated)
	}
	return expression, nil
}

func emitExpression(validated validatedNode) (clause.Expression, error) {
	switch validated.kind {
	case weave.KindConstant:
		return newConstant(validated.constant), nil

	case weave.KindGroup:
		if !validLogic(validated.logic) || len(validated.children) == 0 {
			return nil, emissionError(validated)
		}
		children := make([]clause.Expression, len(validated.children))
		for index := range validated.children {
			if validated.children[index].kind == weave.KindNativeCondition {
				return nil, emissionError(validated.children[index])
			}
			child, err := emitExpression(validated.children[index])
			if err != nil {
				return nil, err
			}
			if isNilLike(child) {
				return nil, emissionError(validated.children[index])
			}
			children[index] = child
		}
		expression := combineExpressions(validated.logic, children)
		if isNilLike(expression) {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindComparison:
		if len(validated.values) != 1 {
			return nil, emissionError(validated)
		}
		expression, ok := newComparison(
			validated.operator,
			validated.column,
			validated.values[0],
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindMembership:
		expression, ok := newMembership(
			validated.operator,
			validated.column,
			validated.values,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindRange:
		if validated.operator != weave.OperatorBetween ||
			len(validated.values) != 2 {
			return nil, emissionError(validated)
		}
		return newBetween(
			validated.column,
			validated.values[0],
			validated.values[1],
		), nil

	case weave.KindNull:
		expression, ok := newNullTest(validated.operator, validated.column)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindText:
		expression, ok := newText(
			validated.operator,
			validated.column,
			validated.text,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindNativeCondition:
		if validated.feature != weave.FeatureNativeCondition ||
			isNilLike(validated.native) {
			return nil, emissionError(validated)
		}
		return validated.native, nil

	case weave.KindNativeExpression:
		if validated.feature != weave.FeatureNativeExpression ||
			isNilLike(validated.expression) {
			return nil, emissionError(validated)
		}
		return validated.expression, nil

	default:
		return nil, emissionError(validated)
	}
}

func combineExpressions(
	logic weave.Logic,
	children []clause.Expression,
) clause.Expression {
	if len(children) == 0 {
		return nil
	}
	switch logic {
	case weave.LogicAllOf:
		return clause.And(children...)
	case weave.LogicAnyOf:
		return clause.Or(children...)
	case weave.LogicNoneOf:
		return negateWhole(clause.Or(children...))
	case weave.LogicNotAllOf:
		return negateWhole(clause.And(children...))
	default:
		return nil
	}
}

func emissionError(validated validatedNode) *weave.Error {
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
