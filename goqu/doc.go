// Package goqu compiles Weave predicates into goqu expression slices.
//
// Expressions is accepted directly by a goqu Dataset Where method, and
// Expression is the open exp.Expression interface. Native values must be
// non-nil slices containing only non-nil Boolean WHERE expressions. Expr
// values must be non-nil Boolean WHERE expressions. Callers are responsible
// for the selected dialect's validity and safety of both opaque boundaries,
// and must keep their borrowed nested state unchanged while a predicate may
// be compiled.
//
// Standard leaves accept only immutable typed Field values created by this
// module. Ordinary comparison, membership, range, and literal-text leaves are
// made two-valued under SQL NULL by an explicit non-NULL guard before Boolean
// grouping and negation. Literal text uses a fixed parameterized LIKE
// template and treats %, _, and ! as literal input. Standard query values that
// implement exp.Expression are rejected so caller-provided SQL remains limited
// to the explicit Native and Expr boundaries.
//
// Compile performs a complete stable preorder validation pass before a
// separate emission pass. Every failure returns a nil Expressions value. A
// Compiler retains only an immutable profile; it does not retain a database,
// dataset, context, logger, transaction, or query value. Configured Compiler
// values and immutable predicates are safe for concurrent Compile calls.
package goqu
