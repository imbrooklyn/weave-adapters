package goqu

import (
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

// Expressions is the compiled expression list accepted by Dataset.Where.
// Every successful Compile owns a new top-level slice. Its expression payloads
// and their nested state remain borrowed.
type Expressions []exp.Expression

// Expression is the native goqu expression carrier used by Weave Expr nodes.
// The carrier also admits non-Boolean expressions; callers are responsible
// for supplying a non-nil Boolean WHERE expression valid for the selected
// dialect and for keeping its borrowed state immutable during compilation.
type Expression = exp.Expression

// Factory binds a goqu Compiler to Weave predicate construction. The Factory
// and its immutable predicate snapshots may be compiled concurrently; Builder
// values remain request-local as documented by Weave.
type Factory = weave.Factory[Expressions, Expression]

// Group is the goqu expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the goqu expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]

// ExpressionsOf returns a shallow copy of expressions. Expression payloads
// and their nested references remain borrowed and must stay immutable while
// the result or a predicate containing it may be compiled.
func ExpressionsOf(expressions ...exp.Expression) Expressions {
	cloned := make(Expressions, len(expressions))
	copy(cloned, expressions)
	return cloned
}
