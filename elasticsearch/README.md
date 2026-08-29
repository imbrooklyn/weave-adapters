# Weave Elasticsearch Adapter

This independent Go module defines the immutable mapping and typed-query contract used to compile [Weave](https://github.com/imbrooklyn/weave) predicates into Elasticsearch Query DSL. It uses the official `github.com/elastic/go-elasticsearch/v9` typed API and does not represent queries as adapter-owned JSON maps.

The module is in pre-release development. Its public module file contains no local `replace` directive. A Compiler owns mapping semantics only; it never connects to or discovers configuration from a cluster.

## Version matrix

| Component | Version or contract |
| --- | --- |
| Weave Elasticsearch Adapter | Unreleased pre-release source |
| Go | 1.27 or newer |
| Weave | `v0.1.0-alpha.1` |
| go-elasticsearch | `v9.5.1` |
| Typed API specification revision | `abf9c2c6bb21328339daa197aae15af2ecbc46f0` |
| Elasticsearch server contract pin | `9.5.2` |
| Query profiles | Elasticsearch 9.5 with expensive queries allowed or disallowed |

The client and server pins are separate compatibility inputs. Selecting a Profile is an application assertion about the target cluster; it does not perform a version request or mapping discovery.

## Type boundary

```go
type Query = *types.Query
type Expression = types.QueryVariant
```

`Query` is the actual typed Query DSL container. Its zero value is `nil`, and every compilation failure returns that zero. A successful Query directly satisfies the `types.QueryVariant` parameter accepted by typed search methods.

`Expression` is the public upstream interface implemented by both `*types.Query` and builders returned by `typedapi/esdsl`. Expr therefore accepts legal upstream query carriers directly and remains nestable; the Adapter adds no Boolean wrapper. Native remains root-only and carries `*types.Query`.

An `esdsl` builder must be converted through `QueryCaster()` before JSON encoding. Its concrete builder type has only unexported state and marshals directly as `{}`. The Compiler performs that conversion for Expr. Calling code that marshals an upstream builder itself has the same responsibility.

## Compiler contract

`Compile` performs a complete stable preorder validation pass before a separate deterministic emission pass. Validation checks Predicate structure and requirements, maximum depth, exact Mapping identity, dynamic Go value type, finite floating-point values, reserved `null_value` sentinels, field applicability, Native scope, and non-nil Expr conversion. The first validation failure is stable, structured, and redacted; every failure returns a nil Query.

The root is an implicit AllOf. Empty roots and constants become typed `match_all` or `match_none` queries. AllOf uses bool `filter`, AnyOf uses `should` with `minimum_should_match: 1`, NoneOf uses `must_not` over each child, and NotAllOf negates one bool-filter conjunction. Standard leaves are emitted only with the official typed term, terms, range, exists, prefix, and wildcard variants.

## Actual upstream query shapes

The locked typed API exposes these concrete shapes:

| Query DSL form | go-elasticsearch v9.5.1 type |
| --- | --- |
| query carrier | `types.QueryVariant`, converted to `*types.Query` with `QueryCaster()` |
| bool | `*types.BoolQuery` in `types.Query.Bool` |
| bool clauses | `[]types.Query` in `Filter`, `Must`, `MustNot`, and `Should` |
| minimum should match | `types.MinimumShouldMatch`, set with `esdsl.NewMinimumShouldMatch().Int(1)` |
| term | `map[string]types.TermQuery` |
| terms | `*types.TermsQuery`, whose field entry is a `types.TermsQueryField` containing `[]types.FieldValue` |
| range | `map[string]types.RangeQuery` with `types.LongNumberRangeQuery`, `types.NumberRangeQuery`, or `types.DateRangeQuery` |
| exists | `*types.ExistsQuery` |
| prefix | `map[string]types.PrefixQuery` |
| wildcard | `map[string]types.WildcardQuery` |

The upstream package internally models specification unions such as `FieldValue`, `TermsQueryField`, `RangeQuery`, and `MinimumShouldMatch` with generated union storage. Standard Adapter paths select their published typed variants or `esdsl` union builders; they do not introduce another `any` query carrier or a JSON map abstraction.

## Mapping declarations

`Field[T]` is an immutable typed declaration created from `FieldSpec[T]`. A spec fixes:

- a conservative canonical dot-separated field path;
- one mapping family and its compatible logical scalar;
- analyzed, multi-valued, and nested status;
- `CompleteValueIndex`, asserting that every logical non-null Value produces its expected searchable indexed term or point;
- the exact keyword normalizer name, or no normalizer;
- a field-level opt-in for expensive leading-wildcard operations;
- no NULL proof, a typed same-field `null_value`, or a companion NULL/Value state marker;
- the exact immutable `weave.OperatorSet` declared for the field.

The supported type mapping is:

| Mapping type | Logical scalar and Go query type | Analyzed | Normalizer |
| --- | --- | --- | --- |
| `MappingKeyword` | string or defined string | No | Optional exact name |
| `MappingWildcard` | string or defined string | No | Not accepted |
| `MappingLong` | signed integer or defined signed integer | No | Not applicable |
| `MappingDouble` | float32/float64 or a defined float | No | Not applicable |
| `MappingDate` | `time.Time` | No | Not applicable |
| `MappingBoolean` | bool or defined bool | No | Not applicable |
| `MappingText` | string or defined string | Yes | Not applicable |

`CompleteValueIndex` must cover mapping and ingestion loss boundaries such as disabled indexing, `ignore_above`, `ignore_malformed`, or an equivalent omission. It is an application assertion, not cluster discovery.

`NewMapping` builds an immutable heterogeneous registry from sealed `MappedField` values. It rejects zero, forged, duplicate-path, and duplicate-identity declarations. A companion marker field must also be registered. A Compiler accepts only Field identities from its own Mapping, even when another Field has the same path and shape.

## Field capability matrix

Every listed capability also requires `CompleteValueIndex`, a single-valued non-nested field, and explicit inclusion in `FieldSpec.Operators`.

| Operator | Keyword | Wildcard | Long / Double | Date | Boolean | Text, multi-valued, or nested |
| --- | --- | --- | --- | --- | --- | --- |
| EQ, NEQ | Yes | Yes | Yes | Yes | Yes | No |
| LT, LTE, GT, GTE | No | No | Yes | Yes | No | No |
| In, NotIn | Yes | Yes | Yes | Yes | Yes | No |
| Between | No | No | Yes, inclusive | No | No | No |
| IsNull, NotNull | Marker only | Marker only | Marker only | Marker only | Marker only | No |
| Contains | Field opt-in and expensive-query Profile | Yes | No | No | No | No |
| HasPrefix | Expensive-query Profile | Yes | No | No | No | No |
| HasSuffix | Field opt-in and expensive-query Profile | Yes | No | No | No | No |

`Compiler.CapabilitiesFor` intersects the declared set with the configured Profile. `Elasticsearch95NoExpensiveQueries` removes keyword Contains, HasPrefix, and HasSuffix because their wildcard or prefix queries can be rejected by `search.allow_expensive_queries=false`. The Elasticsearch `wildcard` mapping keeps its literal-text operations because that mapping family is designed for those term patterns.

Analyzed text is never made to look like literal EQ or substring semantics. Multi-valued fields are not made to look like one logical scalar. Nested scope is not silently flattened. These cases use reviewed Expr queries or future Elasticsearch-specific helpers.

At the locked Weave core revision, `Between` is constrained to numeric query values. Date intervals therefore use GTE and LTE in an AllOf group; the Adapter does not advertise or simulate date Between.

## NULL, missing, and binary match sets

An Elasticsearch `exists` query reports indexed existence. Source `null`, an empty array, disabled indexing, ignored oversized values, and ignored malformed values can all produce no indexed value. Consequently, a field without a searchable NULL proof advertises neither IsNull nor NotNull.

`IndexNullAs(value)` declares a reserved same-field `null_value` sentinel. Under that contract:

- IsNull is the sentinel term;
- NotNull is indexed existence excluding the sentinel;
- ordinary leaves include indexed existence and exclude the sentinel;
- the application must prevent a real Value from using the sentinel's indexed term.

`NewCompanionMarker` instead binds a complete, single-valued, non-nested keyword field with distinct reserved terms for explicit NULL and Value. It requires no normalizer, no own NULL mapping, and no standard operators. The companion field must be in the same Mapping. IsNull uses its NULL term; NotNull and ordinary leaves require its Value term. A missing document matches neither marker term.

Ordinary EQ, order, membership, numeric Between, and literal-text leaves always use a Value guard before their typed term, terms, range, prefix, or wildcard query. NEQ and NotIn are not emitted as bare `must_not`: they retain the Value guard and negate only the positive term match. This gives each child a two-valued match set before an outer NoneOf or NotAllOf complement is applied.

## Literal text and query cost

HasPrefix uses a typed prefix query and passes the literal prefix as data. Contains and HasSuffix use typed wildcard queries. Before adding Adapter-owned wildcard operators, the Compiler escapes each literal `\\`, `*`, and `?` with a leading backslash. JSON encoding then performs the separate JSON backslash escaping.

Contains and HasSuffix on a keyword field require both its `AllowExpensiveWildcard` declaration and a Profile that asserts expensive queries are enabled. Keyword HasPrefix also requires that Profile because Elasticsearch can classify prefix queries as expensive. A `MappingWildcard` field does not need the keyword opt-in. Cluster execution can still fail for operational limits outside the immutable Profile; the Compiler does not execute requests.

The Adapter records a keyword normalizer name as part of the immutable mapping assertion but does not execute that normalizer locally. Term-level and pattern inputs must already follow the indexed-term convention established by the application mapping.

## Expr, ownership, and scope

Expr may carry any legal upstream `types.QueryVariant`, including full-text, nested, geo, script, or query-string builders. Those query families are not standard Weave Operators. The caller is responsible for Boolean query suitability, mapping compatibility, scripts or query-string input, cluster cost, and keeping borrowed upstream state immutable while compilation reads it and while a returned query is used.

The generated `esdsl` builder implementations return a fresh top-level `*types.Query` from `QueryCaster()`. In contrast, `(*types.Query).QueryCaster()` returns the receiver. The Adapter does not promise a recursive deep copy of Native or Expr payloads. It never retains a predicate or per-call query after Compile returns.

A configured Compiler retains only immutable Profile, Mapping indexes, and capability snapshots. It does not retain an Elasticsearch client, transport, index, context, credential, request builder, logger, session, or query value.

## Verification

From this directory, with dependencies available in the active Go environment:

```sh
gofmt -w *.go
GOWORK=off go mod verify
GOWORK=off go mod tidy -diff
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go test -run '^$' -fuzz '^FuzzEscapeWildcardLiteral$' -fuzztime=10s
GOWORK=off go test -run '^$' -fuzz '^FuzzCompileLiteralAndRedaction$' -fuzztime=10s
GOWORK=off go test -run '^$' -bench '^Benchmark(Query|Compile)' -benchmem
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

The unit suite fixes the upstream QueryVariant/search method seam, direct-builder marshal boundary, exact bool/term/terms/range/exists/prefix/wildcard JSON shapes, typed descriptor invariants, Mapping identity, NULL marker prerequisites, Profile capability intersections, literal wildcard escaping, stable two-pass failures, zero Query failures, fresh standard-query ownership, and concurrent deterministic compilation. Fuzz targets cover wildcard escaping plus literal compilation and error redaction. Benchmarks isolate typed emission, JSON marshal, 1,024-value terms, depth-128 Boolean structure, and repeated/concurrent Compile. Baselines detect trends and are not portable absolute thresholds.

This directory is independently buildable with `GOWORK=off`; its public `go.mod` contains no local `replace`. After a release coordinate is resolvable, the repository module-zip check must download that artifact and verify its inherited root Apache-2.0 LICENSE. A source workspace cannot replace that released-artifact check.

The sibling integration testbed pins Elasticsearch 9.5.2 and validates the emitted queries against an explicit live mapping; a successful unit suite alone is not evidence of server semantics.

## License

Repository-owned code and documentation are licensed under the [Apache License 2.0](../LICENSE). The locked `github.com/elastic/go-elasticsearch/v9` client and its transport are also distributed under Apache License 2.0; their exact-version attribution is retained in [Third-Party Notices](THIRD_PARTY_NOTICES.md). Elastic, Elasticsearch, and the upstream client are independent from Weave; their names identify dependency and compatibility boundaries only.
