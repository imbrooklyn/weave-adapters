# Weave GORM Gen Adapter

The `gormgen` module provides the GORM Gen type boundary and immutable compiler configuration for [Weave](https://github.com/imbrooklyn/weave). Compiled `Conditions` are compatible with a generated DAO's variadic `Where` method.

## Current behavior

The module currently provides:

- `Conditions`, `Expression`, `Factory`, `Group`, and `Scope` specializations for GORM Gen.
- `MySQL` and `PostgreSQL` immutable compiler profiles.
- Pure generated-column discovery through `field.Expr.RawExpr()` and the public `Eq` method signature.
- Conservative generated-field operator inference.
- Immutable `FieldSpec` overrides and optional registered-fields-only enforcement.
- Request-stateless `NewCompiler` and `NewFactory` constructors.
- A complete validation pass followed by a separate emission pass for every standard Weave operator, Boolean constant, and Boolean group.
- Root-only Native condition slices and nestable borrowed `field.Expr` values.
- Fixed internal SQL templates whose columns and query values are passed only as GORM variables.
- Non-NULL guards for ordinary leaves and literal LIKE escaping with `ESCAPE '!'`.
- A reproducible generated DAO fixture, a compile-checked `Where(conditions...)` usage package, and real MySQL/PostgreSQL semantic coverage through Weave's shared `compilertest` fixture.

Both profiles advertise all 14 standard operators, Native conditions, and Expr. Field-level applicability is narrower and is reported through `CapabilitiesFor`.

## Locked versions

| Component | Version |
| --- | --- |
| Go | `1.27` |
| Weave | `v0.1.0` module requirement |
| GORM Gen | `v0.3.28` |
| GORM | `v1.31.2` |
| GORM MySQL driver | `v1.6.0` |
| GORM PostgreSQL driver | `v1.6.0` |
| GORM DBResolver | `v1.6.2` |
| `golang.org/x/tools` | `v0.49.0` |

GORM Gen `v0.3.28` declares older DBResolver and `x/tools` versions. This module pins the versions in the table because the complete graph is compile-verified with Go 1.27 and GORM `v1.31.2`.

The compatible Weave core line has not been tagged. Consequently, the module cannot yet pass an independent `GOWORK=off` dependency test even though its `go.mod` contains no local `replace` directive.

## Profiles and verified backends

A profile fixes SQL compilation semantics; it does not hold a database connection, inspect a GORM Dialector, or verify that the caller executes the result on a matching backend.

| Profile | Real-backend baseline | Controlled fixture text semantics |
| --- | --- | --- |
| `MySQL` | MySQL `8.0.40` | `utf8mb4_bin` table and text columns |
| `PostgreSQL` | PostgreSQL `15.12` | `COLLATE "C"` on identifier and text columns |

The local integration runner locks the corresponding official container image digests. These versions are verified baselines, not a claim that every server version or deployment collation has identical behavior.

## Generated DAO usage

`Conditions` has the exact element type accepted by generated DAO `Where` methods:

```go
queries := query.Use(database)
factory, err := gormgen.NewFactory(gormgen.MySQL)
if err != nil {
    return nil, err
}
conditions, err := factory.New().
    EQ(queries.User.Name, "alice").
    NotNull(queries.User.Active).
    Build()
if err != nil {
    return nil, err
}

users, err := queries.User.Where(conditions...).Find()
```

`ConditionsOf` can package caller-owned Native conditions and shallow-clones the top-level slice. Compiled output also owns a fresh top-level slice. Individual GORM Gen conditions, Expr values, and their nested references remain borrowed.

## Generated fields and FieldSpec

A standard field must be a non-nil `field.Expr` whose `RawExpr()` is a non-raw, unaliased `clause.Column` with a non-empty name. Aggregate, arithmetic, ordering, aliased, asterisk, and raw expressions are rejected as standard fields.

The module reads the generated field's public `Eq` method with reflection. The method must have one value argument and return `field.Expr`. No private GORM Gen struct field is inspected.

Default applicability is conservative:

| Generated `Eq` value type | Inferred standard operator families |
| --- | --- |
| Boolean and other recognized scalar types | equality, membership, null |
| Signed, unsigned, and floating-point types | equality, membership, null, ordering, `Between` |
| String or a defined string type | equality, membership, null, ordering, literal text |
| `time.Time` | equality, membership, null, ordering |
| `[]byte` | equality, membership, null |

Use `NewFieldSpec[T]` to narrow the operator set or declare an exact custom value type:

```go
nameSpec, err := gormgen.NewFieldSpec[string](
    queries.User.Name,
    weave.OperatorEQ,
    weave.OperatorContains,
    weave.OperatorIsNull,
)
if err != nil {
    return err
}

factory, err := gormgen.NewFactory(
    gormgen.PostgreSQL,
    gormgen.WithFieldSpecs(nameSpec),
    gormgen.WithRegisteredFieldsOnly(),
)
```

`T` must be directly assignable to the generated `Eq` argument type. The Compiler performs no reflection conversion, numeric narrowing, or string coercion. A non-empty operator list is a complete replacement set. Registered-fields-only mode requires at least one valid FieldSpec and rejects columns outside the immutable registry.

Field descriptors and registries describe query type and applicability; they are not an authorization system.

## Operator and feature matrix

| Surface | `MySQL` | `PostgreSQL` | Commitment |
| --- | --- | --- | --- |
| Comparison | Yes | Yes | `EQ`, `NEQ`, `LT`, `LTE`, `GT`, and `GTE`, subject to field applicability |
| Membership | Yes | Yes | `In` and `NotIn` with individually bound, normalized non-null values |
| Range | Yes | Yes | Inclusive numeric `Between` |
| Null | Yes | Yes | `IsNull` and `NotNull` |
| Literal text | Yes | Yes | `Contains`, `HasPrefix`, and `HasSuffix` for string fields |
| Boolean structure | Yes | Yes | constants, `AllOf`, `AnyOf`, `NoneOf`, and `NotAllOf` |
| Native condition | Yes | Yes | root-only `Conditions` slice |
| Native expression | Yes | Yes | nestable borrowed `field.Expr` |

Every ordinary comparison, membership, range, and text leaf includes an explicit `column IS NOT NULL` guard. Consequently, applying `Not`, `NoneOf`, or `NotAllOf` to those leaves preserves Weave's two-valued match-set behavior instead of exposing SQL UNKNOWN. `IsNull` and `NotNull` use their direct total predicates.

## Compiler lifecycle

A Compiler stores only its profile and immutable field registry. It does not accept or retain a database handle, GORM session, context, logger, transaction, or query value. The Compiler can be copied and reused concurrently.

Validation walks the normalized tree in stable preorder depth-first order and returns the first structured, redacted error. It checks depth, node shape, capability, pure generated columns, exact value assignability, FieldSpec applicability, and Native/Expr preconditions before emission starts. Every failure returns a nil `Conditions` value.

## Native and Expr preconditions

A Native payload is a root-level `Conditions` slice. The slice may be empty; every element must be non-nil, have a nil `CondError()`, and be a condition accepted by the locked GORM Gen DAO. The Compiler shallow-clones the top-level output slice but does not clone individual conditions or maintain a private concrete-condition whitelist.

`Expression` is an alias of `field.Expr`, which also admits ordering, aggregate, and assignment-oriented expressions. Expr values may be nested, but callers must supply a non-nil expression with a nil `CondError()` that is a valid Boolean `WHERE` condition for the selected profile. The module does not add a Boolean wrapper, deep-copy the expression, or attempt to prove arbitrary expression semantics.

## Template and field safety

The internal GORM Gen raw-field constructor is used only with package-owned templates. Columns are supplied as `clause.Column` variables, and ordinary query values are supplied as separate GORM variables. No public API accepts an SQL template, and ordinary query values are not formatted into SQL. `In` placeholder count is derived only from the validated element count.

Literal text escapes `!`, `%`, and `_` as `!!`, `!%`, and `!_`, then adds only the wildcard positions required by the selected operation. User text is never interpreted as a wildcard pattern.

Registered-fields-only mode can be used when an application needs a closed standard-field allowlist. Native conditions and Expr remain explicit escape hatches outside the standard-field safety boundary.

## Known semantic boundaries

SQL rows do not represent Weave's `missing` state separately from explicit `NULL`. The shared real-backend harness therefore materializes missing fixture values as SQL `NULL` and declares `DistinguishesMissing=false`. The canonical `compilertest` cases select their missing-collapsed expected ID sets for direct `IsNull` and nullable `In`; only the case that identifies missing itself is skipped.

Text equality, ordering, and LIKE behavior inherit the deployed database schema's collation and Unicode rules. The verified fixture controls those inputs with `utf8mb4_bin` on MySQL and `COLLATE "C"` on PostgreSQL; the Compiler does not promise to erase collation, normalization, case, or accent differences between arbitrary deployments.

The selected profile must match the database used to execute the conditions. Database connections, transactions, statement lifetimes, schema types, and migration policy remain application-owned. Native and Expr values retain the additional caller obligations described above.

## Verification

The checked-in fixture is generated from `internal/fixture/model.User` and `internal/fixture/model.SemanticRecord`. Regenerate it and compile every package with:

```sh
go generate ./...
go test ./...
go test -race ./...
go vet ./...
```

The fixture's usage package calls the real generated `Where(conditions...).Find()` signature; it does not substitute handwritten field or DAO types. Structural SQL/Vars checks cover both locked drivers in DryRun mode.

Run the semantic suite against temporary real databases with:

```sh
./scripts/test-integration.sh
```

The runner creates exactly one MySQL and one PostgreSQL container with tmpfs data directories, executes the generated-DAO `compilertest` harness, and removes both containers on exit. The harness compares final record-ID sets, checks bound-value SQL construction, and covers SQL NULL materialization in addition to the shared cases.

Compile and sample the no-threshold benchmark baseline, including generated-field reflection metadata lookup, with:

```sh
go test -run '^$' -bench . -benchtime=1x
```

The Compiler currently performs metadata reflection on each lookup and has no metadata cache, predicate cache, or query-value cache.

## License

This module is licensed under the repository's [Apache License 2.0](../LICENSE). The locked GORM projects are distributed under their upstream MIT licenses; their attribution and license text are retained in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). The dependency names identify compatibility only and do not imply upstream sponsorship or endorsement.
