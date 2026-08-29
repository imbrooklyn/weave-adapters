# Weave Adapters

Weave Adapters contains backend compilers for [Weave](https://github.com/imbrooklyn/weave). Each Adapter is an independent Go module with its own dependency and release boundary. Adapter modules do not import one another.

## Status

This repository is in pre-release development. It currently contains the `memory` reference Compiler; the `gormgen`, native `gorm`, and `goqu` SQL Compilers; the MongoDB BSON Compiler; and the LDAP RFC 4515 filter Compiler described below.

The module files require the published Weave core prerelease `v0.1.0-alpha.1` and are independently resolvable with `GOWORK=off`. Development CI tests all six modules together against an exact current public core revision and also tests every declared module boundary independently with `GOWORK=off`. Public module files contain no local `replace` directives.

## Modules

| Module | Current behavior |
| --- | --- |
| [`ldap`](ldap/) | Compiles the exact LDAP-applicable standard-operator subset into deterministic RFC 4515 filters, with immutable typed Schema descriptors, numeric attribute OIDs, explicit presence totalization, no portable IsNull or approximate strict ordering, root Schema-bound Native filters, validated nestable string Expr filters, redacted two-pass compilation, and real OpenLDAP 2.6.10 match-set validation. |
| [`mongo`](mongo/) | Compiles every standard operator and Boolean group into deterministic ordered BSON filters for the immutable MongoDB 6.0+ profile, with typed safe field paths, explicit existence/non-null totalization, exact null/missing shapes, quoted literal-text PCRE patterns, root Native and nestable opaque Expr support, stable two-pass validation/emission, redacted BSON preflight failures, shallow escape-hatch cloning, and deterministic concurrent compilation. |
| [`goqu`](goqu/) | Compiles every standard operator and Boolean group into native goqu expressions, with canonical typed fields, immutable MySQL/PostgreSQL profiles, SQL NULL totalization, fixed parameterized literal-text lowering, root Native and nestable Expr support, stable two-pass validation/emission, deterministic concurrent compilation, prepared SQL/argument safety checks, and real MySQL/PostgreSQL shared semantic coverage. |
| [`gorm`](gorm/) | Compiles every standard operator and Boolean group into one native `clause.Expression`, with typed non-raw fields, SQL NULL totalization, fixed parameterized literal-text lowering, root Native and nestable Expr support, stable two-pass validation/emission, traditional/generic GORM execution, DryRun SQL/Vars checks, and real MySQL/PostgreSQL shared semantic coverage. |
| [`gormgen`](gormgen/) | Compiles every standard operator and Boolean group into parameterized GORM Gen conditions, with pure-column metadata, immutable FieldSpec/registry configuration, Native/Expr support, generated-DAO coverage, and real MySQL/PostgreSQL semantic tests. |
| [`memory`](memory/) | Compiles every standard operator, Boolean group, constant, Native condition, and Expr into a record-level condition while preserving value, explicit-null, and missing semantics. |

The root `go.work` contains only modules that actually exist. It is a repository development convenience, not a published dependency boundary.

## Requirements

- Go 1.27 or newer.
- A compatible Weave core dependency.

## Repository checks

`scripts/check-repository.sh` verifies the complete Apache-2.0 license text, independent module paths, Go versions, absence of public `replace` directives, and exact agreement between `go.work` and the modules present in the repository.

With all module dependencies resolvable in the active Go environment, run:

```sh
./scripts/verify.sh
```

The script checks formatting and runs tests and `go vet` for every existing Adapter module. It discovers modules from the repository instead of naming modules that have not been created. With workspace resolution disabled, CI additionally runs module verification, tidy checks, tests, race tests, vet, and benchmark smoke tests for all six modules. Every Adapter suite has a fuzz-smoke entry; LDAP runs separate grammar/canonicalization and literal escaping/redaction targets.

The GORM Gen and native GORM modules provide `gormgen/scripts/test-integration.sh` and `gorm/scripts/test-integration.sh`. Each runs its shared semantic fixture against temporary real MySQL and PostgreSQL containers and removes the containers and their tmpfs data on exit.

The integration testbed runs the goqu module's prepared queries against the
same real MySQL and PostgreSQL fixture and requires its record-ID match sets to
agree with memory, GORM Gen, and GORM.

After a tagged module version is resolvable, verify the released artifact's inherited license with:

```sh
./scripts/check-module-zip.sh github.com/imbrooklyn/weave-adapters/memory@v0.1.0-alpha.1
```

The check downloads the actual module zip and requires its root `LICENSE` to be the complete Apache-2.0 text.

## Versioning

Each Adapter module is versioned independently. A module release uses a subdirectory tag such as `memory/v0.1.0-alpha.1`; a repository-root tag does not imply that every Adapter has the same version.

## Security

Standard Adapter paths must preserve Weave's two-valued match-set semantics, keep compilers request-stateless, and avoid exposing field values, query values, native payloads, expression payloads, or credentials in errors. See [SECURITY.md](SECURITY.md) for private reporting and the current security boundaries.

## License

Repository-owned code and documentation are licensed under the [Apache License 2.0](LICENSE).
