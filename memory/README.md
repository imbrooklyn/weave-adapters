# Weave Memory Adapter

The `memory` module is the typed, in-process reference Compiler for [Weave](https://github.com/imbrooklyn/weave). It compiles predicates into record-level functions without owning a record collection or other request state.

## Current behavior

The module provides:

- `StateValue`, `StateNull`, and `StateMissing` as distinct Accessor results.
- Immutable `Field[R,V]` descriptors composed from a typed `Accessor[R,V]` and `Semantics[V]` value.
- Built-in semantics for comparable, ordered, string, `time.Time`, and `[]byte` values.
- Every standard comparison, membership, range, null, and literal-text operator.
- `AllOf`, `AnyOf`, `NoneOf`, and `NotAllOf`, normalized constants, and implicit-root conjunction.
- Root Native conditions and directly accepted, nestable Expr expressions.
- Global and field-level capability discovery.
- A validate-then-emit Compiler with stable preorder depth-first error selection.
- Differential fuzz coverage against an independent match-set evaluator.
- Runnable examples and benchmark baselines for compilation and record matching.

## Field states and match sets

An Accessor returns a value and exactly one state:

| State | Ordinary standard leaf | `IsNull` | `NotNull` |
| --- | ---: | ---: | ---: |
| `StateValue` | According to the operator | `false` | `true` |
| `StateNull` | `false` | `true` | `false` |
| `StateMissing` | `false` | `false` | `false` |

The returned value is used only for `StateValue`. Zero and unrecognized State values produce `ErrInvalidState` during Condition execution. An Accessor is called once for each evaluated leaf, and no value is cached across nodes.

`NEQ(field, value)` and `NotIn(field, values)` remain ordinary leaves, so they exclude null and missing fields. Group negation complements a complete match set: `NoneOf(EQ(field, value))` therefore includes records whose field is null or missing.

## Typed fields and semantics

```go
type Record struct {
    Name string
}

name, err := memory.NewField(
    "name",
    func(record Record) (string, memory.State) {
        return record.Name, memory.StateValue
    },
    memory.StringSemantics(),
)
if err != nil {
    return err
}
```

`NewField` rejects blank names and nil accessors. A Field is an immutable value snapshot; its Accessor and semantic functions are borrowed, so callers must keep captured state deterministic and safe for their intended concurrency. The developer-facing name is not included in default compile diagnostics.

Query values must be assignable to the Field's value type. The Compiler does not apply reflection conversion, numeric narrowing, or string coercion. A concrete value that implements an interface-valued Field is assignable and accepted.

Semantic functions control field applicability:

| Semantic function | Applicable operator family |
| --- | --- |
| equality | `EQ`, `NEQ`, `In`, `NotIn` |
| ordering | `LT`, `LTE`, `GT`, `GTE`, `Between` |
| text projection | `Contains`, `HasPrefix`, `HasSuffix` |

`IsNull` and `NotNull` depend only on Accessor state and are available for every valid Field. A missing semantic function returns `weave.ErrOperatorNotApplicable` during validation. `OrderUnordered` makes every ordering comparison false; an unrecognized Ordering produces `ErrInvalidOrdering` during execution.

Text operations use `strings.Contains`, `strings.HasPrefix`, and `strings.HasSuffix`. Their operands are literal strings: wildcard, regular-expression, escape, Unicode, and newline characters receive no special interpretation.

## Compilation and execution

`Compiler[R]` first validates the entire normalized tree and then emits a Condition from a typed validation plan. Validation returns the first error in stable preorder depth-first order, with the core node Path and Origin, `PhaseValidate`, and redacted type metadata. Emission failures use `PhaseEmit`. Every compilation failure returns a nil Condition.

Conditions and expressions share the signature:

```go
func(Record) (bool, error)
```

Groups evaluate children from left to right and short-circuit. Evaluated errors propagate unchanged. `Expr` accepts a legal `Expression[R]` directly; the Compiler does not add another Boolean wrapper or attempt to prove the expression's meaning. Native and Expr functions, their captured state, and nested references remain borrowed. Nil function values are rejected during validation, while direct calls to nil `Condition.Match` and `Expression.Evaluate` return module errors instead of panicking.

Accessor and semantic-function panics are not recovered. Callers remain responsible for legal Native and Expr behavior and for concurrency-safe borrowed state.

## Shared semantic fixture

The module consumes `github.com/imbrooklyn/weave/compilertest`. That shared suite compares stable record-ID match sets across every standard operator, all four Boolean logics, scalar values, explicit null, missing fields, literal special characters, nullable membership, three-level nesting, Native, and Expr. It also checks global and field capabilities, nil-like native payload rejection, structured redacted validation errors, zero Conditions on failure, and repeated and concurrent compilation. The suite does not interpret an Adapter's generated text or internal condition representation.

## Differential fuzzing

`FuzzMemoryMatchesOracle` generates bounded predicate trees and compares the compiled Condition with an independent evaluator over the same specification. Its corpus and generated inputs cover value/null/missing states, nullable `In`, floating-point NaN and unordered comparisons, literal Unicode and special characters, and nested groups.

## Examples and benchmarks

The package examples demonstrate typed field construction, compilation, matching, and the difference between explicit null and missing:

```sh
go test -run '^Example' ./...
```

Benchmarks cover one-record matching, a 1,024-record batch, building and compiling a representative nested AST, compiling 100- and 1,000-element `In` predicates, and repeatedly compiling one immutable Predicate:

```sh
go test -run '^$' -bench . ./...
```

Run the differential fuzzer with a duration appropriate for the change:

```sh
go test -run '^$' -fuzz '^FuzzMemoryMatchesOracle$' -fuzztime=10s
```

## Compiler lifecycle

`Compiler[R]` contains no records, request state, database handle, collection, session, context, logger, or transaction. It can be reused concurrently when all borrowed functions and captured state follow their concurrency contracts. Compiled Conditions receive records only when evaluated.

The compatible Weave core line has not been tagged. Consequently, the module cannot yet pass an independent `GOWORK=off` dependency test even though its `go.mod` contains no local `replace` directive.

## Requirements

- Go 1.27 or newer.
- A compatible Weave core dependency.

## License

This module is licensed under the repository's [Apache License 2.0](../LICENSE).
