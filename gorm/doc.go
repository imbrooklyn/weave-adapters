// Package gorm compiles Weave predicates into native GORM clause expressions.
//
// Condition and Expression are direct aliases of clause.Expression. The
// carrier is intentionally open: callers that add Expr nodes are responsible
// for supplying non-nil Boolean WHERE expressions that are safe for the
// selected profile and remain unchanged while a predicate may be compiled.
//
// Standard leaves accept only immutable typed Field values created by this
// module. Ordinary comparison, membership, range, and literal-text leaves are
// made two-valued under SQL NULL by an explicit non-NULL guard before Boolean
// grouping and negation. Literal text uses a fixed parameterized LIKE template
// and treats %, _, and ! as literal input.
//
// Compile performs a complete stable preorder validation pass before a
// separate emission pass. Every failure returns a nil Condition. A Compiler
// retains only an immutable profile; it does not retain a database handle,
// Dialector, session, context, logger, transaction, or query value. Configured
// Compiler values are safe for concurrent use.
package gorm
