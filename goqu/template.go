package goqu

import (
	"strings"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

const (
	trueTemplate        = "1 = 1"
	falseTemplate       = "1 = 0"
	wholeNotTemplate    = "NOT (?)"
	literalLikeTemplate = "(? IS NOT NULL AND ? LIKE ? ESCAPE '!')"
)

func newConstant(value bool) exp.Expression {
	if value {
		return sqlbuilder.L(trueTemplate)
	}
	return sqlbuilder.L(falseTemplate)
}

func newGuarded(
	identifier exp.IdentifierExpression,
	expression exp.Expression,
) exp.Expression {
	return sqlbuilder.And(identifier.IsNotNull(), expression)
}

func newComparison(
	operator weave.Operator,
	identifier exp.IdentifierExpression,
	value any,
) (exp.Expression, bool) {
	bound := sqlbuilder.V(value)
	var expression exp.Expression
	switch operator {
	case weave.OperatorEQ:
		expression = identifier.Eq(bound)
	case weave.OperatorNEQ:
		expression = identifier.Neq(bound)
	case weave.OperatorLT:
		expression = identifier.Lt(bound)
	case weave.OperatorLTE:
		expression = identifier.Lte(bound)
	case weave.OperatorGT:
		expression = identifier.Gt(bound)
	case weave.OperatorGTE:
		expression = identifier.Gte(bound)
	default:
		return nil, false
	}
	return newGuarded(identifier, expression), true
}

func newMembership(
	operator weave.Operator,
	identifier exp.IdentifierExpression,
	values []any,
) (exp.Expression, bool) {
	if len(values) == 0 || !membershipOperator(operator) {
		return nil, false
	}
	arguments := make(exp.Vals, len(values))
	copy(arguments, values)
	var expression exp.Expression
	if operator == weave.OperatorIn {
		expression = identifier.In(arguments)
	} else {
		expression = identifier.NotIn(arguments)
	}
	return newGuarded(identifier, expression), true
}

func newBetween(
	identifier exp.IdentifierExpression,
	lower any,
	upper any,
) exp.Expression {
	return sqlbuilder.And(
		identifier.IsNotNull(),
		identifier.Gte(sqlbuilder.V(lower)),
		identifier.Lte(sqlbuilder.V(upper)),
	)
}

func newNullTest(
	operator weave.Operator,
	identifier exp.IdentifierExpression,
) (exp.Expression, bool) {
	switch operator {
	case weave.OperatorIsNull:
		return identifier.IsNull(), true
	case weave.OperatorNotNull:
		return identifier.IsNotNull(), true
	default:
		return nil, false
	}
}

func newText(
	operator weave.Operator,
	identifier exp.IdentifierExpression,
	value string,
) (exp.Expression, bool) {
	escaped := escapeLiteralText(value)
	var pattern string
	switch operator {
	case weave.OperatorContains:
		pattern = "%" + escaped + "%"
	case weave.OperatorHasPrefix:
		pattern = escaped + "%"
	case weave.OperatorHasSuffix:
		pattern = "%" + escaped
	default:
		return nil, false
	}
	return sqlbuilder.L(
		literalLikeTemplate,
		identifier,
		identifier,
		pattern,
	), true
}

func escapeLiteralText(value string) string {
	escaped := strings.ReplaceAll(value, "!", "!!")
	escaped = strings.ReplaceAll(escaped, "%", "!%")
	return strings.ReplaceAll(escaped, "_", "!_")
}

func negateWhole(expression exp.Expression) exp.Expression {
	return sqlbuilder.L(wholeNotTemplate, expression)
}
