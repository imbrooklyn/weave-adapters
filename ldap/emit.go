package ldap

import (
	"errors"
	"strings"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/imbrooklyn/weave"
)

var errUnexpectedEmitPlan = errors.New("ldap: invalid validated emission plan")

func emitPredicate(validated validatedNode) (string, error) {
	if validated.kind != weave.KindGroup || validated.logic != weave.LogicAllOf {
		return "", emissionError(validated)
	}
	children := make([]string, len(validated.children))
	for index, child := range validated.children {
		if child.kind == weave.KindNativeCondition {
			if child.feature != weave.FeatureNativeCondition || !child.native.Valid() {
				return "", emissionError(child)
			}
			children[index] = child.native.String()
			continue
		}
		expression, err := emitExpression(child)
		if err != nil {
			return "", err
		}
		if expression == "" {
			return "", emissionError(child)
		}
		children[index] = expression
	}
	switch len(children) {
	case 0:
		return constantFilter(true), nil
	case 1:
		return children[0], nil
	default:
		return combine("&", children), nil
	}
}

func emitExpression(validated validatedNode) (string, error) {
	switch validated.kind {
	case weave.KindConstant:
		return constantFilter(validated.constant), nil
	case weave.KindGroup:
		if !validLogic(validated.logic) || len(validated.children) == 0 {
			return "", emissionError(validated)
		}
		children := make([]string, len(validated.children))
		for index, child := range validated.children {
			if child.kind == weave.KindNativeCondition {
				return "", emissionError(child)
			}
			expression, err := emitExpression(child)
			if err != nil {
				return "", err
			}
			if expression == "" {
				return "", emissionError(child)
			}
			children[index] = expression
		}
		return combineLogic(validated.logic, children), nil
	case weave.KindComparison:
		if len(validated.values) != 1 {
			return "", emissionError(validated)
		}
		operation, ok := comparisonFilter(
			validated.operator, validated.attribute, validated.values[0],
		)
		if !ok {
			return "", emissionError(validated)
		}
		return guardedFilter(validated.attribute, operation), nil
	case weave.KindMembership:
		operation, ok := membershipFilter(
			validated.operator, validated.attribute, validated.values,
		)
		if !ok {
			return "", emissionError(validated)
		}
		return guardedFilter(validated.attribute, operation), nil
	case weave.KindRange:
		if validated.operator != weave.OperatorBetween || len(validated.values) != 2 {
			return "", emissionError(validated)
		}
		return combine("&", []string{
			presenceFilter(validated.attribute),
			assertionFilter(validated.attribute, ">=", validated.values[0]),
			assertionFilter(validated.attribute, "<=", validated.values[1]),
		}), nil
	case weave.KindNull:
		if validated.operator != weave.OperatorNotNull {
			return "", emissionError(validated)
		}
		return presenceFilter(validated.attribute), nil
	case weave.KindText:
		return textFilter(validated.operator, validated.attribute, validated.text)
	case weave.KindNativeExpression:
		if validated.feature != weave.FeatureNativeExpression ||
			validated.expression == "" {
			return "", emissionError(validated)
		}
		return validated.expression, nil
	default:
		return "", emissionError(validated)
	}
}

func comparisonFilter(
	operator weave.Operator,
	attribute string,
	value string,
) (string, bool) {
	var ldapOperator string
	switch operator {
	case weave.OperatorEQ:
		ldapOperator = "="
	case weave.OperatorNEQ:
		return negate(assertionFilter(attribute, "=", value)), true
	case weave.OperatorLTE:
		ldapOperator = "<="
	case weave.OperatorGTE:
		ldapOperator = ">="
	default:
		return "", false
	}
	return assertionFilter(attribute, ldapOperator, value), true
}

func membershipFilter(
	operator weave.Operator,
	attribute string,
	values []string,
) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	children := make([]string, len(values))
	for index, value := range values {
		children[index] = assertionFilter(attribute, "=", value)
	}
	operation := combine("|", children)
	switch operator {
	case weave.OperatorIn:
		return operation, true
	case weave.OperatorNotIn:
		return negate(operation), true
	default:
		return "", false
	}
}

func textFilter(operator weave.Operator, attribute, value string) (string, error) {
	if value == "" {
		return presenceFilter(attribute), nil
	}
	escaped := ldapv3.EscapeFilter(value)
	var assertion string
	switch operator {
	case weave.OperatorContains:
		assertion = "(" + attribute + "=*" + escaped + "*)"
	case weave.OperatorHasPrefix:
		assertion = "(" + attribute + "=" + escaped + "*)"
	case weave.OperatorHasSuffix:
		assertion = "(" + attribute + "=*" + escaped + ")"
	default:
		return "", emissionError(validatedNode{operator: operator})
	}
	return guardedFilter(attribute, assertion), nil
}

func assertionFilter(attribute, operator, value string) string {
	return "(" + attribute + operator + ldapv3.EscapeFilter(value) + ")"
}

func presenceFilter(attribute string) string {
	return "(" + attribute + "=*)"
}

func guardedFilter(attribute, operation string) string {
	return combine("&", []string{presenceFilter(attribute), operation})
}

func constantFilter(value bool) string {
	anchor := presenceFilter(objectClassOID)
	if value {
		return combine("|", []string{anchor, negate(anchor)})
	}
	return combine("&", []string{anchor, negate(anchor)})
}

func combineLogic(logic weave.Logic, children []string) string {
	switch logic {
	case weave.LogicAllOf:
		return combine("&", children)
	case weave.LogicAnyOf:
		return combine("|", children)
	case weave.LogicNoneOf:
		return negate(combine("|", children))
	case weave.LogicNotAllOf:
		return negate(combine("&", children))
	default:
		return ""
	}
}

func combine(operator string, children []string) string {
	var builder strings.Builder
	builder.WriteByte('(')
	builder.WriteString(operator)
	for _, child := range children {
		builder.WriteString(child)
	}
	builder.WriteByte(')')
	return builder.String()
}

func negate(filter string) string {
	return "(!" + filter + ")"
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
