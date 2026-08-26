# Weave GORM Adapter

The `gorm` module compiles [Weave](https://github.com/imbrooklyn/weave) predicates into native GORM clause expressions.

## Current behavior

The module currently provides:

- `Condition` and `Expression` as direct aliases of `gorm.io/gorm/clause.Expression`.
- `Factory`, `Group`, and `Scope` specializations for the native GORM expression carrier.
- Immutable `MySQL` and `PostgreSQL` compiler profiles.
- `Field[T]`, qualified and unqualified field constructors, sealed `FieldOption` values, and explicit operator declarations.
- Conservative default field applicability, exact per-field validation, and `CapabilitiesFor` discovery.
- All 14 standard operators, constants, and all four Boolean group logics.
- Root-only Native conditions and nestable Expr values.
- Stable preorder validation followed by a separate emission pass; every failure returns a nil `Condition`.
- SQL NULL totalization for ordinary leaves and fixed parameterized literal-text lowering.
- Request-stateless, immutable, concurrently reusable `Compiler` values.
- Compile fixtures for traditional `DB.Where` and GORM's generic `Where` chain.
- MySQL and PostgreSQL DryRun tests for clause shape, group precedence, NULL behavior, quoted columns, fixed LIKE templates, and final SQL/Vars boundaries.
- A shared stable-ID semantic suite executed through both public GORM query APIs on real MySQL and PostgreSQL.
- Emit, Compile, concurrent Compile, DryRun build, and 100/1000-value membership benchmarks, plus literal-text fuzz coverage.

A valid `Compiler` advertises all standard operators plus `FeatureNativeCondition` and `FeatureNativeExpression`. Global support does not widen a field's immutable operator set.

## Locked versions

| Component | Version |
| --- | --- |
| Go | `1.27` |
| Weave | `v0.1.0-alpha.1` module requirement |
| GORM | `v1.31.2` |
| GORM MySQL driver | `v1.6.0` |
| GORM PostgreSQL driver | `v1.6.0` |

The Weave core prerelease in the table is published and independently resolvable with `GOWORK=off`. This module's `go.mod` contains no local `replace` directive.

## Compatibility and capabilities

The integration suite pins and verifies these profile/backend combinations. The collation is part of the fixture semantics; broader server versions and application collations are not implied by this evidence.

| Profile | GORM driver | Verified database | Controlled text collation |
| --- | --- | --- | --- |
| `MySQL` | `gorm.io/driver/mysql v1.6.0` | MySQL `8.0.40` | `utf8mb4_bin` |
| `PostgreSQL` | `gorm.io/driver/postgres v1.6.0` | PostgreSQL `15.12` | `C` |

`Profile.String()` returns the stable diagnostic identifiers `mysql` and `postgresql`; zero and unknown values use `profile(n)`. A Profile's integer representation is an implementation detail, not a persistence, serialization, or interchange protocol.

Both profiles expose the same global Compiler capabilities:

| Capability | Supported | Field requirement or boundary |
| --- | --- | --- |
| `EQ`, `NEQ`, `LT`, `LTE`, `GT`, `GTE` | Yes | Operator must be declared by the typed field. |
| `In`, `NotIn` | Yes | Operator must be declared; nullable input is normalized. |
| `Between` | Yes | Numeric typed field with `Between` declared. |
| `IsNull`, `NotNull` | Yes | Operator must be declared by the typed field. |
| `Contains`, `HasPrefix`, `HasSuffix` | Yes | String or defined-string typed field with the operator declared. |
| Constants and `AllOf`, `AnyOf`, `NoneOf`, `NotAllOf` | Yes | No field-specific widening. |
| Native condition | Yes | Root only; caller-owned Boolean WHERE expression. |
| Expr | Yes | Nestable; caller-owned Boolean WHERE expression. |

SQL storage has no separate missing state: missing and explicit NULL collapse to SQL NULL. The shared suite selects canonical missing-collapsed expected ID sets for its NULL and nullable-membership cases; only the case that identifies missing itself is skipped. The `memory` Adapter remains the reference for a distinct missing state.

## Native GORM type boundary

Both Adapter carriers are the upstream interface without a package-specific Boolean wrapper:

```go
type Condition = clause.Expression
type Expression = clause.Expression
```

The locked GORM version accepts a result compiled through the public Adapter API in both query APIs:

```go
factory, err := weavegorm.NewFactory(weavegorm.PostgreSQL)
if err != nil {
    return err
}
status := weavegorm.MustQualifiedField[string]("users", "status")
condition, err := factory.New().EQ(status, "active").Build()
if err != nil {
    return err
}

var traditional []Record
if err := database.Where(condition).Find(&traditional).Error; err != nil {
    return err
}

generic, err := gorm.G[Record](database).
    Where(condition).
    Find(ctx)
```

A compiled result is one expression rather than an expression slice. Native and Expr retain distinct Weave positions even though C and E use the same Go interface.

## Profiles and lifecycle

Create immutable compilation semantics during application assembly:

```go
compiler, err := weavegorm.NewCompiler(weavegorm.PostgreSQL)
if err != nil {
    return err
}

factory, err := weavegorm.NewFactory(weavegorm.PostgreSQL)
if err != nil {
    return err
}
```

A `Compiler` stores only its `Profile`. It does not accept, inspect, or retain a `*gorm.DB`, Dialector, session, context, logger, transaction, model, or query value. The application must pair a profile with a matching database dialect.

`Compile` first validates the complete normalized tree in stable preorder depth-first order, including its typed fields, assignable value types, exact field operator sets, and native payload preconditions. Only a successful validation pass reaches emission. Errors use structured Weave paths, origins, types, and phases without formatting field values, query values, or native payloads. Every failure returns the nil interface, which is the zero value of `Condition`; no partial clause tree is returned.

## Typed fields

Standard Weave fields must be created by this module:

```go
name, err := weavegorm.NewQualifiedField[string]("users", "name")
if err != nil {
    return err
}

score := weavegorm.MustField[int64]("score")
```

`NewField` accepts one column segment. `NewQualifiedField` accepts table and column as separate segments; it never guesses that `table.column` is two identifiers. Segments must begin with a Unicode letter or underscore and continue with Unicode letters, digits, underscores, or `$`. Empty, dotted, wildcard, whitespace-padded, control-character, and SQL-fragment-shaped values are rejected. Every stored `clause.Column` has `Raw=false` and no alias.

Field descriptors are immutable query-type and applicability declarations. They are not field authorization, schema migration, or database introspection.

Default applicability is conservative:

| `T` family | Default standard operator families |
| --- | --- |
| Signed, unsigned, and floating-point types | equality, membership, null, ordering, `Between` |
| String or a defined string type | equality, membership, null, literal text |
| `bool`, `time.Time`, and `[]byte` | equality, membership, null |
| Other types | null only |

String and custom ordering require an explicit declaration. `WithOperators` supplies a complete non-empty replacement set:

```go
createdAt, err := weavegorm.NewField[time.Time](
    "created_at",
    weavegorm.WithOperators(
        weave.OperatorEQ,
        weave.OperatorLT,
        weave.OperatorGTE,
        weave.OperatorIsNull,
        weave.OperatorNotNull,
    ),
)
```

An explicit declaration is the application's assertion that the single-valued database field has those semantics. Unknown operators are rejected. `Between` still requires a numeric `T`, and literal-text operators require a string or defined-string `T`.

Field capabilities can be read directly or through a configured Compiler:

```go
fieldCapabilities := name.Capabilities()

fieldCapabilities, err = compiler.CapabilitiesFor(name)
```

Raw `clause.Column`, strings, fields from other Adapter packages, zero `Field[T]` values, pointers, and embedding wrappers are rejected by `CapabilitiesFor`. Use the immutable descriptor value returned by this module's constructors.

Compilation applies the same boundary. Values must be assignable to `T`; merely convertible values are rejected. The declared field `OperatorSet` is checked exactly for every standard leaf.

## Lowering and SQL NULL

The Compiler emits one non-nil `clause.Expression`. An empty root becomes the fixed true expression `1 = 1`. Standard comparisons use `clause.Eq`, `Neq`, `Lt`, `Lte`, `Gt`, and `Gte`; membership uses `clause.IN`; `Between` uses inclusive `Gte` and `Lte`; explicit NULL predicates use `clause.Eq{Value:nil}` and `clause.Neq{Value:nil}`.

Ordinary comparison, membership, range, and literal-text leaves are totalized before they enter a Boolean group:

```sql
column IS NOT NULL AND <ordinary operation>
```

`IsNull` and `NotNull` are already two-valued and lower directly. This preserves Weave's two-valued match-set complement when a standard leaf appears below `NoneOf` or `NotAllOf`. The four group mappings are AND, OR, whole-expression `NOT (OR ...)`, and whole-expression `NOT (AND ...)`. The implementation uses a one-child `OrConditions` identity before whole-expression NOT because locked GORM otherwise expands `AndConditions` and invokes child negation builders.

Literal text escapes input in this order: `!` to `!!`, `%` to `!%`, then `_` to `!_`. It adds only the wildcard positions required by `Contains`, `HasPrefix`, or `HasSuffix`, then emits the fixed internal template:

```go
clause.Expr{
    SQL:  "? LIKE ? ESCAPE '!'",
    Vars: []any{column, escapedPattern},
}
```

The non-raw `clause.Column` is quoted by the selected GORM Dialector, and only the pattern remains in final bound Vars. Neither field names nor query values are formatted into SQL templates.

## Verified upstream expression seams

The locked GORM API exposes `clause.Eq`, `Neq`, `Lt`, `Lte`, `Gt`, `Gte`, and `IN`. `clause.Eq{Value:nil}` and `clause.Neq{Value:nil}` build `IS NULL` and `IS NOT NULL` respectively.

For two operands, `clause.And`, `clause.Or`, and `clause.Not` produce `clause.AndConditions`, `clause.OrConditions`, and `clause.NotConditions` at the tested call sites. GORM special-cases `Not(AndConditions)` by expanding the AND and invoking child negation builders. A whole-expression negation must therefore wrap the complete expression as a one-child `OrConditions` identity before calling `clause.Not`; the checked-in seam test preserves that requirement.

A non-raw `clause.Column` supplied through an upstream clause builder is quoted as an identifier and removed from final bound variables. The fixed template below produces a quoted column followed by one bound pattern in both locked Dialectors:

```go
clause.Expr{
    SQL:  "? LIKE ? ESCAPE '!'",
    Vars: []any{column, pattern},
}
```

The template is a verified package implementation boundary, not a public raw-SQL constructor. Ordinary callers remain responsible for any raw `clause.Expr` values they supply through Native or Expr.

## Native and Expr preconditions

`Expression` remains the open `clause.Expression` interface. It can represent valid Boolean filters, but it can also carry order, select, assignment, or custom expressions that are not WHERE predicates. Callers must ensure every Expr value:

- is non-nil, including no typed nil;
- is a Boolean WHERE expression for the selected profile and GORM version;
- uses GORM parameter binding for untrusted values;
- remains unchanged while its Predicate may be read or compiled concurrently.

The module does not add a Boolean wrapper, inspect arbitrary expression internals, deep-copy expression state, or make raw SQL safe. Native conditions have the same upstream safety and ownership obligations and remain root-only under Weave's core contract.

Native and Expr are emitted without inspection, rewriting, totalization, or deep copy. Native combines only at the implicit root; Expr retains its exact nested position. Both reject nil and typed nil. A custom expression that violates the documented preconditions is caller error even if it implements `clause.Expression`.

## Verification

With a compatible Weave core available in the active development workspace, run:

```sh
go test -mod=readonly ./...
go test -mod=readonly -race ./...
go vet -mod=readonly ./...
go test -mod=readonly ./internal/fixture/usage
go test -mod=readonly -run '^$' -bench . -benchmem
go test -mod=readonly -run '^$' -fuzz '^FuzzCompileLiteralText$' -fuzztime=3s
```

The usage fixture compiles the real traditional and generic GORM calls. Unit tests use both locked Dialectors in DryRun mode and assert clause shape, quoting, placeholders, and final SQL/Vars boundaries without opening a database connection.

For real match-set semantics, Docker, and the locked database images, run:

```sh
./scripts/test-integration.sh
```

The runner starts temporary MySQL and PostgreSQL containers with tmpfs data, asserts the server versions and controlled collations shown above, runs the race-enabled shared `compilertest` stable-ID fixture, and removes both containers on exit. Every semantic execution runs the same compiled `Condition` through traditional `DB.Where` and generic `gorm.G[T].Where` and requires identical ordered IDs. Coverage includes every standard operator, value and NULL behavior, complements, nullable membership, all four Boolean logics, literal `%`, `_`, `!`, Unicode, Native, Expr, empty groups, and deep nesting. SQL/Vars inspection and an injection-shaped literal probe run alongside the real queries; DryRun is not used as a substitute for their match-set assertions.

## License

This module is licensed under the repository's [Apache License 2.0](../LICENSE). The locked GORM projects are distributed under their upstream MIT licenses; see [Third-Party Notices](THIRD_PARTY_NOTICES.md) for versions, attribution, and the applicable MIT text. Dependency names identify compatibility only and do not imply upstream sponsorship or endorsement.
