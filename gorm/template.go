package gorm

import (
	"strings"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

const (
	trueTemplate        = "1 = 1"
	falseTemplate       = "1 = 0"
	literalLikeTemplate = "? LIKE ? ESCAPE '!'"
)

func newConstant(value bool) clause.Expression {
	if value {
		return clause.Expr{SQL: trueTemplate}
	}
	return clause.Expr{SQL: falseTemplate}
}

func newGuarded(
	column clause.Column,
	expression clause.Expression,
) clause.Expression {
	return clause.And(
		clause.Neq{Column: column, Value: nil},
		expression,
	)
}

func newComparison(
	operator weave.Operator,
	column clause.Column,
	value any,
) (clause.Expression, bool) {
	var expression clause.Expression
	switch operator {
	case weave.OperatorEQ:
		expression = clause.Eq{Column: column, Value: value}
	case weave.OperatorNEQ:
		expression = clause.Neq{Column: column, Value: value}
	case weave.OperatorLT:
		expression = clause.Lt{Column: column, Value: value}
	case weave.OperatorLTE:
		expression = clause.Lte{Column: column, Value: value}
	case weave.OperatorGT:
		expression = clause.Gt{Column: column, Value: value}
	case weave.OperatorGTE:
		expression = clause.Gte{Column: column, Value: value}
	default:
		return nil, false
	}
	return newGuarded(column, expression), true
}

func newMembership(
	operator weave.Operator,
	column clause.Column,
	values []any,
) (clause.Expression, bool) {
	if len(values) == 0 || !membershipOperator(operator) {
		return nil, false
	}
	expression := clause.Expression(clause.IN{
		Column: column,
		Values: append([]any(nil), values...),
	})
	if operator == weave.OperatorNotIn {
		expression = clause.Not(expression)
	}
	return newGuarded(column, expression), true
}

func newBetween(
	column clause.Column,
	lower any,
	upper any,
) clause.Expression {
	return newGuarded(
		column,
		clause.And(
			clause.Gte{Column: column, Value: lower},
			clause.Lte{Column: column, Value: upper},
		),
	)
}

func newNullTest(
	operator weave.Operator,
	column clause.Column,
) (clause.Expression, bool) {
	switch operator {
	case weave.OperatorIsNull:
		return clause.Eq{Column: column, Value: nil}, true
	case weave.OperatorNotNull:
		return clause.Neq{Column: column, Value: nil}, true
	default:
		return nil, false
	}
}

func newLiteralLike(
	column clause.Column,
	pattern string,
) clause.Expression {
	return newGuarded(column, clause.Expr{
		SQL:  literalLikeTemplate,
		Vars: []any{column, pattern},
	})
}

func newText(
	operator weave.Operator,
	column clause.Column,
	value string,
) (clause.Expression, bool) {
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
	return newLiteralLike(column, pattern), true
}

func escapeLiteralText(value string) string {
	escaped := strings.ReplaceAll(value, "!", "!!")
	escaped = strings.ReplaceAll(escaped, "%", "!%")
	return strings.ReplaceAll(escaped, "_", "!_")
}

// negateWhole applies NOT to one complete expression. The one-child
// OrConditions identity prevents GORM v1.31.2 from expanding AndConditions
// and negating each child independently.
func negateWhole(expression clause.Expression) clause.Expression {
	return clause.Not(clause.Or(expression))
}
