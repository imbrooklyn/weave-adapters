# Weave MongoDB Adapter

This independent Go module compiles [Weave](https://github.com/imbrooklyn/weave) predicates into ordered BSON filter documents for MongoDB 6.0 and newer. The result can be passed directly to MongoDB Go Driver collection query methods.

The module is in pre-release development. Its public module file has no local `replace` directive.

## Version matrix

| Component | Version or status |
| --- | --- |
| Weave MongoDB Adapter | Unreleased pre-release source |
| Go | 1.27 or newer |
| Weave | `v0.1.0-alpha.1` |
| MongoDB Go Driver | `v2.8.2` |
| MongoDB semantic profile | `MongoDB60Plus` for MongoDB 6.0 or newer |

The Compiler emits filter semantics for the selected baseline. It does not connect to, inspect, or negotiate with a live server.

## Type boundary

```go
type Filter = bson.D
type Expression = bson.D
```

Both carriers are ordered BSON documents. Their shared representation does not erase the Weave boundary: Native is root-only, while Expr is nestable. Expr intentionally remains an opaque escape hatch, and the Adapter does not add a forced Boolean wrapper.

## Example

```go
package example

import (
    weavemongo "github.com/imbrooklyn/weave-adapters/mongo"
)

func userFilter() (weavemongo.Filter, error) {
    name, err := weavemongo.NewField[string]("profile.name")
    if err != nil {
        return nil, err
    }
    score, err := weavemongo.NewField[int64]("score")
    if err != nil {
        return nil, err
    }
    factory, err := weavemongo.NewFactory(weavemongo.MongoDB60Plus)
    if err != nil {
        return nil, err
    }

    return factory.New().
        HasPrefix(name, "A.*").
        Between(score, int64(10), int64(20)).
        Build()
}
```

The text `A.*` is matched literally; it is not treated as a caller-provided regular expression. A Compiler retains only its immutable Profile and does not own a client, collection, database, context, session, logger, transaction, or query value.

## Typed field paths

Standard leaves accept only `Field[T]` values created by this module. `NewField` and `NewFieldWithOperators` validate a dot-separated path whose segments begin with a Unicode letter or underscore and continue with letters, digits, or underscores. They reject empty segments, invalid UTF-8, NUL and control characters, whitespace or punctuation fragments, `$`-prefixed operators, and positional syntax.

Field declarations must come from application-controlled metadata. Do not turn an untrusted request field name directly into a Field; map external names through an allowlist of predeclared values.

A standard Field declares a single-valued BSON field with value, explicit-null, or missing state. MongoDB multikey array semantics are outside this standard boundary; use a reviewed Expr document for intentional array-specific behavior.

`UnsafeField` is an explicit trusted-schema escape hatch for ordinary MongoDB field names that need a wider character set. It still rejects invalid UTF-8, NUL/control characters, surrounding whitespace, empty segments, and `$`-prefixed segments. It returns a zero Field for invalid input and must never receive an untrusted request value.

`NewField` infers this conservative operator set:

| Go field type | Default standard operators |
| --- | --- |
| Boolean | equality, membership, null |
| Numeric or defined numeric type | equality, membership, null, ordering, Between |
| String or defined string type | equality, membership, null, ordering, literal text |
| `time.Time` | equality, membership, null, ordering |
| `[]byte` | equality, membership, null |
| Other BSON value | equality, membership, null |

`NewFieldWithOperators` accepts a non-empty complete replacement set. It rejects unknown operators and direct type conflicts such as Between on a non-numeric field or literal-text operations on a non-string field. Ordering for BSON scalars such as ObjectID or Decimal128 can be enabled explicitly when the application owns that schema contract. `Compiler.CapabilitiesFor` reports the exact immutable set.

## Operator and feature matrix

| Operation or feature | `MongoDB60Plus` | Boundary |
| --- | --- | --- |
| EQ, NEQ | Yes | Standard typed field |
| LT, LTE, GT, GTE | Yes | Applicable typed field |
| In, NotIn | Yes | Non-empty normalized membership |
| Between | Yes | Numeric typed field; inclusive |
| IsNull, NotNull | Yes | Typed field; explicit missing distinction |
| Contains | Yes | String typed field; literal text |
| HasPrefix | Yes | String typed field; absolute `\A` anchor |
| HasSuffix | Yes | String typed field; absolute `\z` anchor |
| AllOf, AnyOf | Yes | `$and`, `$or` |
| NoneOf, NotAllOf | Yes | `$nor` match-set complements |
| Normalized constants | Yes | `{}` and `{$expr: false}` |
| Native condition | Yes | Root-only `bson.D` |
| Expr expression | Yes | Nestable opaque `bson.D` |

Compilation uses a complete validation pass followed by a separate emission pass. Values must be assignable to the Field's declared Go type and encodable by the driver's default BSON registry. The Adapter does not perform conversions or use a client-specific registry. A failure returns a nil Filter and a stable, location-aware, redacted `*weave.Error`; validation reports the first failure in preorder depth-first traversal.

## BSON, null, and missing behavior

Standard output uses `bson.D` for every document and `bson.A` for every array. `bson.M` and Go maps are not used for generated structure, so key, operator, child, and root order remain deterministic.

Ordinary query values remain BSON values; the Adapter does not reinterpret them as keys or recursively clone them. Nested reference state in those values is borrowed under the Weave ownership contract. For deterministic final encoded bytes, use an ordered representation such as `bson.D`, rather than `bson.M` or a Go map, for a document-valued query input and keep all borrowed state immutable.

Ordinary comparison, membership, range, and literal-text leaves have this shape:

```text
{$and: [
    {field: {$exists: true}},
    {field: {$ne: null}},
    operation(field)
]}
```

The explicit guards make each standard leaf two-valued before grouping and negation. This is especially important for MongoDB operators such as non-null `$ne` and `$nin`, which can otherwise include a missing field.

| Weave operation | Matches explicit BSON null | Matches missing | Matches present non-null |
| --- | ---: | ---: | ---: |
| IsNull | Yes | No | No |
| NotNull | No | No | Yes |
| Ordinary standard leaf | No | No | According to its operation |

IsNull emits `$exists: true` plus `$eq: null`; NotNull emits `$exists: true` plus `$ne: null`. A bare `{field: null}` is not used because MongoDB also matches missing fields for that form.

## Literal text and PCRE boundary

Contains, HasPrefix, and HasSuffix always treat input as literal text. The Adapter applies Go's `regexp.QuoteMeta`, then uses these fixed pattern forms:

```text
Contains  = QuoteMeta(value)
HasPrefix = \A + QuoteMeta(value)
HasSuffix = QuoteMeta(value) + \z
```

The pattern is emitted as a string under `$regex`. No `$options` value is added, so caller input cannot enable `i`, `m`, `s`, or another mode. Invalid UTF-8 and NUL are rejected. MongoDB 6.0 uses the original PCRE library and MongoDB 6.1 and newer use PCRE2; `\A` and `\z` are absolute subject anchors in both families.

## Native and Expr boundary

Native accepts one non-nil `Filter` document directly below the predicate root. Expr accepts one non-nil `Expression` document and can be nested in Boolean groups. A non-nil empty document is valid and represents true.

The Adapter validates placement and nil shape but deliberately does not recursively marshal or classify escape-hatch operators. Callers are responsible for supplying filter documents accepted by the target server, for ensuring Expr is Boolean and two-valued where required, and for preventing unsafe caller-built operators.

## Concurrency and ownership

A configured Compiler is request-stateless and safe for concurrent compilation. Every successful compile creates fresh generated BSON topology and shallow-clones the top-level document of each Native and Expr payload. Nested references inside ordinary values and Native/Expr payloads remain borrowed, so callers must keep that state immutable while a predicate may be compiled or used. Repeated compilation preserves generated BSON element and array order.

## Verification

From this directory, with dependencies available in the active Go environment:

```sh
gofmt -w *.go
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go mod verify
GOWORK=off go mod tidy -diff
GOWORK=off go test -run '^$' -fuzz FuzzLiteralTextAlwaysStaysARegexString -fuzztime=5s
GOWORK=off go test -run '^$' -fuzz FuzzSafeFieldPathNeverBecomesAnOperatorFragment -fuzztime=5s
GOWORK=off go test -run '^$' -bench . -benchmem ./...
```

The tests cover every standard operator, BSON document and array structure, deterministic BSON bytes, the four Boolean logic forms, constants, root order, existence and non-null guards, explicit null versus missing shapes, literal quoting and absolute anchors, typed path rejection, default-registry failures, redacted stable errors, escape-hatch ownership, and concurrent Compiler reuse.

The benchmark suite isolates BSON emission and records baselines for 100- and 1,000-value In predicates, maximum-depth mixed Logic, repeated Compile, and concurrent Compile. Baselines are used to detect trends rather than as portable absolute thresholds.

This directory is independently buildable with `GOWORK=off`. Its `go.mod` pins Weave and MongoDB Go Driver versions and contains no local `replace`. After a release tag exists, the repository-level module-zip check must download that public coordinate and verify that the inherited root Apache-2.0 LICENSE is present; a local workspace result cannot replace that release check.

## Real-service validation

The companion [Weave Integration Testbed](https://github.com/imbrooklyn/weave-integration-testbed) applies these filters through MongoDB Go Driver v2.8.2 `Collection.Find` calls. Its fixed service matrix covers MongoDB 6.0.28 and 8.3.8. Both versions execute all 31 canonical `compilertest` match sets without a skip, including explicit null, missing, nullable membership, empty groups, three-level Logic, Native, and Expr.

Dedicated server regressions demonstrate why the standard guards are required: on the shared fixture, a bare `$ne: 2` includes explicit-null and missing documents, while compiled NEQ returns only the present non-null memory-reference set; bare `$nin: [2, 5]` behaves similarly. Literal-text probes cover regex metacharacters, a backslash, Unicode, embedded and trailing newlines, and confirm that `\z` is a strict subject-end anchor where raw `$` can also match before a final newline. The testbed also checks typed-path and value injection boundaries, zero-document redacted failures, identical repeated BSON bytes, and concurrent Compile/Find results.

These are final match-set assertions against real services. BSON shape tests remain useful structural checks but are not used as a substitute for server execution.

## License

Repository-owned code and documentation are licensed under the [Apache License 2.0](../LICENSE). The locked MongoDB Go Driver dependency is also distributed under Apache License 2.0 and carries additional third-party notices; exact-version attribution is retained in [Third-Party Notices](THIRD_PARTY_NOTICES.md). MongoDB and its driver are independent from Weave. Their names identify compatibility only and do not imply sponsorship or endorsement.
