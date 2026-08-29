package elasticsearch

import (
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave"
)

// Query is the Elasticsearch typed Query DSL value produced by compilation.
// Its zero value is nil. A successful query can be passed directly to typed
// client methods that accept types.QueryVariant. Standard operators produce
// package-owned query state; Native and raw Expr queries remain borrowed
// upstream values and are not deep-copied.
type Query = *types.Query

// Expression is the public upstream query carrier accepted by Weave Expr
// nodes. Both *types.Query and the builders returned by typedapi/esdsl satisfy
// this interface. The Adapter does not add a package-owned Boolean wrapper.
// Callers must not mutate a borrowed Expression while Compile reads it or
// while the returned Query is in use.
type Expression = types.QueryVariant

// Factory binds an Elasticsearch Compiler to Weave predicate construction.
type Factory = weave.Factory[Query, Expression]

// Group is the Elasticsearch expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the Elasticsearch expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]
