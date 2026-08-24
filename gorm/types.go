package gorm

import (
	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

// Condition is the single native GORM expression produced by a Compiler.
// A Condition can be passed directly to DB.Where or to GORM's generic Where
// chain.
type Condition = clause.Expression

// Expression is the native GORM expression carrier used by Weave Expr nodes.
// clause.Expression is an open interface and also admits values that are not
// Boolean filters; callers are responsible for supplying a valid WHERE
// expression.
type Expression = clause.Expression

// Factory binds a native GORM Compiler to Weave predicate construction.
type Factory = weave.Factory[Condition, Expression]

// Group is the native GORM expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the native GORM expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]
