package gormgen

import (
	"errors"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen/field"
)

var errUnexpectedEmitPlan = errors.New("gormgen: invalid validated emission plan")

func emitPredicate(validated validatedNode) (Conditions, error) {
	if validated.kind != weave.KindGroup ||
		validated.logic != weave.LogicAllOf {
		return nil, emissionError(validated)
	}

	conditions := make(Conditions, 0, len(validated.children))
	for _, child := range validated.children {
		if child.kind == weave.KindNativeCondition {
			if child.feature != weave.FeatureNativeCondition {
				return nil, emissionError(child)
			}
			conditions = append(conditions, child.native...)
			continue
		}
		expression, err := emitExpression(child)
		if err != nil {
			return nil, err
		}
		if isNilLike(expression) {
			return nil, emissionError(child)
		}
		conditions = append(conditions, expression)
	}
	return conditions, nil
}

func emitExpression(validated validatedNode) (field.Expr, error) {
	switch validated.kind {
	case weave.KindConstant:
		return newConstant(validated.constant), nil

	case weave.KindGroup:
		if !validLogic(validated.logic) || len(validated.children) == 0 {
			return nil, emissionError(validated)
		}
		children := make([]field.Expr, len(validated.children))
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
	children []field.Expr,
) field.Expr {
	if len(children) == 0 {
		return nil
	}
	switch logic {
	case weave.LogicAllOf:
		return field.And(children...)
	case weave.LogicAnyOf:
		return field.Or(children...)
	case weave.LogicNoneOf:
		return field.Not(field.Or(children...))
	case weave.LogicNotAllOf:
		return field.Not(field.And(children...))
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
