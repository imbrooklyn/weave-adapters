// Package mongo compiles Weave predicates into deterministic ordered BSON
// filters for MongoDB 6.0 and newer servers.
//
// Filter and Expression are both bson.D. The shared carrier does not erase the
// semantic boundary: Native remains root-only, while Expr is nestable and is
// intentionally opaque. The package does not add a forced Boolean wrapper to
// Expr; callers own the validity, totality, and safety of escape-hatch BSON.
//
// Standard leaves accept only immutable typed Field values created by this
// module. Safe field constructors accept conservative dot-separated paths and
// reject BSON operator or positional fragments. UnsafeField is reserved for
// trusted schema constants and still enforces structural path safety.
//
// Comparison, membership, range, and literal-text leaves include explicit
// field-existence and non-null guards. IsNull matches only a present explicit
// BSON null, and NotNull matches only a present non-null value. These guards
// preserve Weave's two-valued match-set semantics through all Boolean groups
// and their complements.
//
// Literal-text operations use regexp.QuoteMeta and fixed PCRE-compatible
// patterns. Prefix and suffix matching use the absolute \A and \z anchors. No
// user input is interpreted as a regular expression and no $options are added.
//
// Compile performs a complete stable preorder validation pass before a
// separate emission pass. Standard output uses bson.D and bson.A exclusively.
// Generated standard BSON topology is newly owned on every successful call.
// Ordinary query values remain BSON values and are not recursively cloned.
// Native and Expr documents are shallow-cloned at their top level. All nested
// reference state remains borrowed and must stay immutable while in use. A
// caller that needs deterministic final BSON bytes must use an ordered
// representation such as bson.D, rather than bson.M or a Go map, for any
// document-valued query input. Every failure returns a nil Filter and a
// structured, location-aware error that does not format field paths, query
// values, or escape-hatch payloads. A Compiler retains only an immutable
// Profile; it does not retain a client, collection, database, context, session,
// logger, transaction, or query value. Configured Compiler values and immutable
// predicates are safe for concurrent Compile calls.
package mongo
