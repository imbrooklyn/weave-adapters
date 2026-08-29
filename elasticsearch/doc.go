// Package elasticsearch defines the immutable mapping and typed-query boundary
// used to compile Weave predicates into Elasticsearch Query DSL queries.
//
// Query is *types.Query from the official go-elasticsearch v9 typed API, and
// Expression is its public types.QueryVariant interface. A Compiler owns only
// immutable Profile and Mapping state. It never owns a client, transport,
// index, context, credential, request builder, or per-request query value.
//
// Mapping declarations are application assertions about an explicitly managed
// Elasticsearch mapping. This package does not perform cluster mapping
// discovery.
package elasticsearch
