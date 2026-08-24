package gormgen

import (
	"github.com/imbrooklyn/weave"
	"gorm.io/gen"
	"gorm.io/gen/field"
)

// Conditions is the compiled condition list accepted by a generated DAO's
// Where method.
type Conditions []gen.Condition

// Expression is the native GORM Gen expression carrier used by Weave Expr
// nodes. The carrier also admits non-Boolean expressions; callers are
// responsible for supplying a valid WHERE expression.
type Expression = field.Expr

// Factory binds a gormgen Compiler to Weave predicate construction.
type Factory = weave.Factory[Conditions, Expression]

// Group is the GORM Gen expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the GORM Gen expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]

// ConditionsOf returns a shallow copy of conditions. The condition payloads
// and their nested references remain borrowed.
func ConditionsOf(conditions ...gen.Condition) Conditions {
	cloned := make(Conditions, len(conditions))
	copy(cloned, conditions)
	return cloned
}
