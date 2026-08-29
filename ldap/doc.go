// Package ldap compiles Weave predicates into deterministic RFC 4515 LDAP
// search filters.
//
// Standard leaves accept only immutable typed Attribute values registered in
// the Compiler's Schema. Each descriptor fixes an attribute description,
// numeric OID, cardinality, LDAP syntax, matching rules, and exact standard
// operator set. Generated standard filters use numeric attribute OIDs and
// explicit presence guards. LDAP has no portable explicit NULL value, so the
// package supports NotNull as attribute presence and deliberately does not
// advertise IsNull. Strict LT and GT also remain unsupported because LDAP
// filter grammar has no exact strict-order assertion.
//
// Filter is an immutable, Schema-bound canonical filter value. Expression is
// deliberately the open string carrier used by Weave Expr nodes; no Boolean
// wrapper is imposed. Expr text is still checked against this package's fixed
// RFC 4515 subset, attribute allowlist, and matching-rule allowlist before it
// can be emitted. Because Expr is already complete filter text, the Compiler
// cannot distinguish its assertion values and does not escape them for the
// caller. Native accepts only a valid Filter created for the same Schema and
// remains root-only under Weave's contract. Filter and Expression values can
// contain query data and should not be logged blindly.
//
// A Compiler retains only an immutable Profile and Schema. It does not retain
// an LDAP connection, search request, context, bind credential, logger,
// session, or query value, and configured Compiler values are safe for
// concurrent use. Compile does not retain its predicate or per-call emission
// plan. Validation and emission errors are structured and omit filter text,
// assertion values, attribute identifiers, and credentials.
package ldap
