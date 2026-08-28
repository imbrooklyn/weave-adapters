package mongo

import (
	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Filter is the ordered BSON document accepted by MongoDB collection query
// methods. Every successful Compile returns newly owned generated document and
// array topology. Standard query values and nested state supplied through
// Native or Expr follow Weave's borrowed-payload ownership contract.
type Filter = bson.D

// Expression is the ordered BSON document carrier used by Weave Expr nodes.
// It deliberately remains an opaque escape hatch and can contain documents
// that are not Boolean match expressions. Callers must supply a valid MongoDB
// Boolean filter expression and keep its nested state immutable while used.
type Expression = bson.D

// Factory binds a MongoDB Compiler to Weave predicate construction. The
// Factory and immutable predicate snapshots may be compiled concurrently;
// Builder values remain request-local as documented by Weave.
type Factory = weave.Factory[Filter, Expression]

// Group is the MongoDB expression specialization of weave.Group.
type Group = weave.Group[Expression]

// Scope is the MongoDB expression specialization of weave.Scope.
type Scope = weave.Scope[Expression]

// FilterOf constructs an ordered BSON filter and copies the supplied top-level
// elements. Element values and their nested state remain borrowed.
func FilterOf(elements ...bson.E) Filter {
	cloned := make(Filter, len(elements))
	copy(cloned, elements)
	return cloned
}

func cloneDocument(document bson.D) bson.D {
	cloned := make(bson.D, len(document))
	copy(cloned, document)
	return cloned
}
