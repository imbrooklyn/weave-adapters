// Package memory provides typed, in-process predicate conditions for Weave.
//
// Fields are described by an [Accessor] and immutable [Semantics]. An accessor
// reports [StateValue], [StateNull], or [StateMissing], keeping explicit null
// and missing data distinct. [Condition] and [Expression] both evaluate one
// record and may return an execution error.
//
// Compiler implements every standard Weave operator, all Boolean group forms,
// constants, root Native conditions, and nestable Expr expressions. Standard
// leaves use two-valued match-set semantics: ordinary leaves reject null and
// missing states, while IsNull matches only explicit null and NotNull matches
// only a present value.
//
// Compilation is a complete validation pass followed by emission from the
// validated plan. Compiler holds no records or request state. Compilers and
// compiled Conditions may be reused concurrently when every borrowed
// Accessor, Semantics function, Native condition, Expr expression, and captured
// reference is itself safe for those concurrent calls.
package memory
