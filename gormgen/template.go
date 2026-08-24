package gormgen

import (
	"strings"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
)

const (
	trueTemplate              = "(1 = 1)"
	falseTemplate             = "(1 = 0)"
	guardedEqualityTemplate   = "(? IS NOT NULL AND ? = ?)"
	guardedInequalityTemplate = "(? IS NOT NULL AND ? <> ?)"
	guardedLTTemplate         = "(? IS NOT NULL AND ? < ?)"
	guardedLTETemplate        = "(? IS NOT NULL AND ? <= ?)"
	guardedGTTemplate         = "(? IS NOT NULL AND ? > ?)"
	guardedGTETemplate        = "(? IS NOT NULL AND ? >= ?)"
	guardedBetweenTemplate    = "(? IS NOT NULL AND ? BETWEEN ? AND ?)"
	isNullTemplate            = "(? IS NULL)"
	notNullTemplate           = "(? IS NOT NULL)"
	literalLikeTemplate       = "(? IS NOT NULL AND ? LIKE ? ESCAPE '!')"
)

var literalLikeEscaper = strings.NewReplacer(
	"!", "!!",
	"%", "!%",
	"_", "!_",
)

func newConstant(value bool) field.Expr {
	if value {
		return field.NewUnsafeFieldRaw(trueTemplate)
	}
	return field.NewUnsafeFieldRaw(falseTemplate)
}

func newGuardedEquality(column clause.Column, value any) field.Expr {
	return newGuardedComparison(guardedEqualityTemplate, column, value)
}

func newComparison(
	operator weave.Operator,
	column clause.Column,
	value any,
) (field.Expr, bool) {
	var template string
	switch operator {
	case weave.OperatorEQ:
		template = guardedEqualityTemplate
	case weave.OperatorNEQ:
		template = guardedInequalityTemplate
	case weave.OperatorLT:
		template = guardedLTTemplate
	case weave.OperatorLTE:
		template = guardedLTETemplate
	case weave.OperatorGT:
		template = guardedGTTemplate
	case weave.OperatorGTE:
		template = guardedGTETemplate
	default:
		return nil, false
	}
	return newGuardedComparison(template, column, value), true
}

func newGuardedComparison(
	template string,
	column clause.Column,
	value any,
) field.Expr {
	return field.NewUnsafeFieldRaw(template, column, column, value)
}

func newMembership(
	operator weave.Operator,
	column clause.Column,
	values []any,
) (field.Expr, bool) {
	if len(values) == 0 || !membershipOperator(operator) {
		return nil, false
	}
	template := membershipTemplate(operator, len(values))
	variables := make([]any, 0, len(values)+2)
	variables = append(variables, column, column)
	variables = append(variables, values...)
	return field.NewUnsafeFieldRaw(template, variables...), true
}

func membershipTemplate(operator weave.Operator, count int) string {
	var builder strings.Builder
	builder.Grow(34 + count*3)
	builder.WriteString("(? IS NOT NULL AND ? ")
	if operator == weave.OperatorNotIn {
		builder.WriteString("NOT IN (")
	} else {
		builder.WriteString("IN (")
	}
	for index := range count {
		if index != 0 {
			builder.WriteString(", ")
		}
		builder.WriteByte('?')
	}
	builder.WriteString("))")
	return builder.String()
}

func newBetween(
	column clause.Column,
	lower any,
	upper any,
) field.Expr {
	return field.NewUnsafeFieldRaw(
		guardedBetweenTemplate,
		column,
		column,
		lower,
		upper,
	)
}

func newNullTest(operator weave.Operator, column clause.Column) (field.Expr, bool) {
	switch operator {
	case weave.OperatorIsNull:
		return field.NewUnsafeFieldRaw(isNullTemplate, column), true
	case weave.OperatorNotNull:
		return field.NewUnsafeFieldRaw(notNullTemplate, column), true
	default:
		return nil, false
	}
}

func newLiteralLike(column clause.Column, pattern string) field.Expr {
	return field.NewUnsafeFieldRaw(
		literalLikeTemplate,
		column,
		column,
		pattern,
	)
}

func newText(
	operator weave.Operator,
	column clause.Column,
	value string,
) (field.Expr, bool) {
	escaped := literalLikeEscaper.Replace(value)
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
