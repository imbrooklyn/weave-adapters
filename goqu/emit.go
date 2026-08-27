package goqu

import (
	"errors"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

var errUnexpectedEmitPlan = errors.New("goqu: invalid validated emission plan")

func emitPredicate(validated validatedNode) (Expressions, error) {
	if validated.kind != weave.KindGroup ||
		validated.logic != weave.LogicAllOf {
		return nil, emissionError(validated)
	}

	expressions := make(Expressions, 0, len(validated.children))
	for _, child := range validated.children {
		if child.kind == weave.KindNativeCondition {
			if child.feature != weave.FeatureNativeCondition ||
				isNilLike(child.native) {
				return nil, emissionError(child)
			}
			expressions = append(expressions, child.native...)
			continue
		}
		expression, err := emitExpression(child)
		if err != nil {
			return nil, err
		}
		if isNilLike(expression) {
			return nil, emissionError(child)
		}
		expressions = append(expressions, expression)
	}
	return expressions, nil
}

func emitExpression(validated validatedNode) (exp.Expression, error) {
	switch validated.kind {
	case weave.KindConstant:
		return newConstant(validated.constant), nil

	case weave.KindGroup:
		if !validLogic(validated.logic) || len(validated.children) == 0 {
			return nil, emissionError(validated)
		}
		children := make([]exp.Expression, len(validated.children))
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
			validated.identifier,
			validated.values[0],
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindMembership:
		expression, ok := newMembership(
			validated.operator,
			validated.identifier,
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
			validated.identifier,
			validated.values[0],
			validated.values[1],
		), nil

	case weave.KindNull:
		expression, ok := newNullTest(
			validated.operator,
			validated.identifier,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return expression, nil

	case weave.KindText:
		expression, ok := newText(
			validated.operator,
			validated.identifier,
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
	children []exp.Expression,
) exp.Expression {
	if len(children) == 0 {
		return nil
	}
	switch logic {
	case weave.LogicAllOf:
		return sqlbuilder.And(children...)
	case weave.LogicAnyOf:
		return sqlbuilder.Or(children...)
	case weave.LogicNoneOf:
		return negateWhole(sqlbuilder.Or(children...))
	case weave.LogicNotAllOf:
		return negateWhole(sqlbuilder.And(children...))
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
