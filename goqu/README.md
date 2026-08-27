# Weave goqu Adapter

This independent Go module compiles [Weave](https://github.com/imbrooklyn/weave) predicates into native [goqu v9](https://github.com/doug-martin/goqu) expressions. Its output can be expanded directly into `Dataset.Where`.

The module is in pre-release development. Its public module file has no local `replace` directive.

## Version matrix

| Component | Version or status |
| --- | --- |
| Weave goqu Adapter | Unreleased pre-release source |
| Go | 1.27 or newer |
| Weave | `v0.1.0-alpha.1` |
| goqu | `v9.19.0` |
| MySQL semantic baseline | MySQL 8.0.40 |
| PostgreSQL semantic baseline | PostgreSQL 15.12 |

The database rows record versions exercised by the real integration suite. They do not claim compatibility with untested goqu dialects or every server version.

## Type boundary

```go
type Expressions []exp.Expression
type Expression = exp.Expression
```

`Expressions` is the final root condition and owns a new top-level slice on every successful compile. `Expression` intentionally remains goqu's broad interface. The Adapter does not add a Boolean wrapper to Expr payloads.

## Example

```go
package example

import (
    "fmt"

    sqlbuilder "github.com/doug-martin/goqu/v9"
    weavegoqu "github.com/imbrooklyn/weave-adapters/goqu"
)

func Example() {
    name, err := weavegoqu.NewField[string](
        sqlbuilder.T("users").Col("name"),
    )
    if err != nil {
        panic(err)
    }
    active, err := weavegoqu.NewField[bool](
        sqlbuilder.T("users").Col("active"),
    )
    if err != nil {
        panic(err)
    }
    factory, err := weavegoqu.NewFactory(weavegoqu.PostgreSQL)
    if err != nil {
        panic(err)
    }

    expressions, err := factory.New().
        Contains(name, "50%_off!").
        EQ(active, true).
        Build()
    if err != nil {
        panic(err)
    }

    sqlText, arguments, err := sqlbuilder.
        Dialect("postgres").
        From("users").
        Where(expressions...).
        Prepared(true).
        ToSQL()
    if err != nil {
        panic(err)
    }
    fmt.Println(sqlText, arguments)
}
```

Use the MySQL profile with goqu's `mysql` dialect and the PostgreSQL profile with its `postgres` dialect. A Compiler does not own or infer a database, dataset, context, logger, transaction, or query value.

## Typed fields

Standard leaves accept only `Field[T]` values created by this module. `NewField` extracts ordinary schema, table, and column strings from an `exp.IdentifierExpression`, validates each segment, and reconstructs a canonical goqu identifier. It rejects empty or wildcard columns, raw literal columns, dotted fragments inside a segment, control characters, and other SQL-fragment-shaped input.

Field declarations must come from application-controlled metadata. Do not turn an untrusted request field name directly into a Field; map external names through an allowlist of predeclared values.

`NewField` infers this conservative operator set:

| Go field type | Default standard operators |
| --- | --- |
| Boolean | equality, membership, null |
| Numeric or defined numeric type | equality, membership, null, ordering, Between |
| String or defined string type | equality, membership, null, ordering, literal text |
| `time.Time` | equality, membership, null, ordering |
| `[]byte` | equality, membership, null |
| Other | equality, membership, null |

`NewFieldWithOperators` accepts a non-empty complete replacement set. It rejects unknown operators and obvious type conflicts such as Between on a non-numeric field or literal-text operations on a non-string field. `Compiler.CapabilitiesFor` reports the exact immutable set.

## Operator and feature matrix

Both configured profiles report the same exact Compiler capabilities. Field applicability remains narrower according to the typed-field table above.

| Operation or feature | MySQL | PostgreSQL | Boundary |
| --- | --- | --- | --- |
| EQ, NEQ | Yes | Yes | Standard typed field |
| LT, LTE, GT, GTE | Yes | Yes | Applicable typed field |
| In, NotIn | Yes | Yes | Non-empty normalized membership |
| Between | Yes | Yes | Numeric typed field; inclusive |
| IsNull, NotNull | Yes | Yes | Typed field |
| Contains | Yes | Yes | String typed field; literal text |
| HasPrefix | Yes | Yes | String typed field; literal text |
| HasSuffix | Yes | Yes | String typed field; literal text |
| AllOf, AnyOf | Yes | Yes | Boolean grouping |
| NoneOf, NotAllOf | Yes | Yes | Whole-group negation |
| Normalized constants | Yes | Yes | Fixed internal SQL literals |
| Native condition | Yes | Yes | Root-only `Expressions` |
| Expr expression | Yes | Yes | Nestable `exp.Expression` |

Compilation uses a complete validation pass followed by a separate emission pass. Values must be assignable to the Field's declared Go type. The Adapter does not perform arbitrary conversions, stringify values, or make lossy coercions. Expression-shaped standard values are rejected so caller-provided SQL remains confined to Native and Expr. A failure returns nil `Expressions` and a stable, location-aware, redacted `*weave.Error`; validation reports the first failure in preorder depth-first traversal.

## SQL and NULL behavior

Comparison, membership, range, and literal-text leaves include an explicit `IS NOT NULL` guard before Boolean grouping or negation. IsNull and NotNull retain their direct SQL meaning. Between is emitted as an inclusive guarded `>=` and `<=` pair.

Literal-text operators use one fixed parameterized `LIKE ... ESCAPE '!'` template. Input `!`, `%`, and `_` are escaped as literal characters. With `Prepared(true)`, the resulting pattern and other ordinary values remain bound arguments, including Boolean values and a single `[]byte` membership element.

## Backend boundary

Prepared rendering is covered for exactly two profiles:

| Profile | Required goqu dialect | Identifier quoting | Placeholders | Real semantic baseline |
| --- | --- | --- | --- | --- |
| MySQL | `mysql` | Backticks | `?` | MySQL 8.0.40 |
| PostgreSQL | `postgres` | Double quotes | `$1`, `$2`, ... | PostgreSQL 15.12 |

The caller selects the matching goqu dialect and owns the Dataset, driver, connection, transaction, context, execution, and result decoding. The Adapter neither selects a driver nor inspects a live server. Other goqu dialects are outside its declared support boundary.

## Native and Expr boundary

Native accepts a non-nil `Expressions` slice directly below the predicate root and expands its elements in place; every element must be non-nil. Expr accepts one non-nil `exp.Expression` and can be nested in Boolean groups. Native and Expr payload identities are borrowed; callers must keep their nested state immutable while a predicate may be compiled.

Because `exp.Expression` includes non-Boolean expressions, callers are responsible for providing a valid WHERE expression for the selected dialect. They are also responsible for dialect compatibility and for avoiding unsafe caller-built literal SQL. The Adapter validates nil shape and placement but cannot prove these opaque properties.

## Concurrency and ownership

A configured Compiler retains only its immutable Profile and is safe for concurrent compilation. Each successful result owns a new top-level slice. Native's top-level slice is shallow-cloned; nested expression state remains borrowed.

## Verification

From this directory, with dependencies available in the active Go environment:

```sh
gofmt -w *.go
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off go test -run '^$' -fuzz FuzzCompilePreparedLiteralText -fuzztime=5s
GOWORK=off go test -run '^$' -bench . -benchmem
```

The module tests exercise upstream expression interfaces, both dialects'
prepared SQL and argument structure, every standard operator, fixed literal
templates, field and value validation, deterministic errors, ownership, and
concurrent Compiler reuse. Benchmarks cover validated expression emission,
prepared SQL rendering, 100- and 1,000-element In compilation, and repeated
and concurrent Compile calls.

The integration testbed additionally executes the prepared queries against
MySQL 8.0.40 and PostgreSQL 15.12. For each backend, all 30 SQL-representable
canonical scenarios pass and the one missing-only scenario is skipped because
SQL materializes missing as null. The final record-ID sets agree with memory,
GORM Gen, and GORM under the fixture's controlled binary/`C` collations.

## License

Repository-owned code and documentation are licensed under the [Apache License 2.0](../LICENSE). The locked goqu dependency is distributed under the MIT License; its copyright attribution and license text are retained in [Third-Party Notices](THIRD_PARTY_NOTICES.md). The dependency name identifies compatibility only and does not imply upstream sponsorship or endorsement.
