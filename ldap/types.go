package ldap

import "github.com/imbrooklyn/weave"

// Filter is an immutable canonical RFC 4515 filter bound to the Schema that
// validated it. The zero value is invalid. String returns the text accepted by
// github.com/go-ldap/ldap/v3 search requests. A valid Filter can contain query
// data and should not be logged without an application-specific redaction
// policy.
type Filter struct {
	state *filterState
}

type filterState struct {
	schema *schemaState
	text   string
}

// Valid reports whether filter was constructed and validated by this package.
func (filter Filter) Valid() bool {
	return filter.state != nil && filter.state.schema != nil && filter.state.text != ""
}

// String returns the canonical RFC 4515 filter text. It returns an empty
// string for the zero or otherwise invalid Filter. The returned text can
// contain assertion values; callers own its logging and transport boundary.
func (filter Filter) String() string {
	if !filter.Valid() {
		return ""
	}
	return filter.state.text
}

// Expression is the open RFC 4515 string carrier used by Weave Expr nodes.
// It is not a Boolean wrapper. The Compiler validates its fixed grammar and
// Schema allowlists, while callers remain responsible for escaping assertion
// values before assembling raw text, business semantics, directory
// authorization, logging, and server cost. The Compiler does not escape
// already assembled Expr text.
type Expression = string

// Factory binds an LDAP Compiler to Weave predicate construction.
type Factory = weave.Factory[Filter, Expression]

// Group is the LDAP expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the LDAP expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]
