// Package gormgen compiles Weave predicates into GORM Gen generated-DAO
// conditions for MySQL and PostgreSQL profiles.
//
// Conditions can be passed directly to a generated DAO's variadic Where
// method. The Compiler validates a normalized predicate completely before it
// emits fixed, parameterized templates. Ordinary SQL leaves include non-NULL
// guards so Boolean negation preserves Weave's two-valued match-set semantics.
//
// Expression is GORM Gen's field.Expr escape-hatch carrier. Callers must
// provide non-nil expressions with nil CondError values that are valid Boolean
// filters for the selected database profile.
package gormgen
