# Changelog

This file records notable user-visible changes to Weave Adapters. It follows the structure of [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and uses [Semantic Versioning](https://semver.org/) for independently tagged Adapter module releases.

## [Unreleased]

### Added

- The `ldap` Compiler as an independent Go 1.27 module, with an immutable RFC 4515 profile, locked go-ldap v3.4.14 filter codec, Schema-bound immutable Filter values, open string Expr values, typed attribute description/OID/cardinality/syntax/matching-rule descriptors, exact field capabilities, 11 precisely lowerable standard operators, presence totalization, safe literal substring escaping, fixed grammar and allowlists, redacted two-pass compilation, deterministic canonicalization, stable-first-error, concurrency, fuzz and benchmark coverage, a real OpenLDAP 2.6.10 match-set contract, and retained upstream MIT attribution.
- The `mongo` Compiler as an independent Go 1.27 module, with an immutable MongoDB 6.0+ profile, locked MongoDB Go Driver v2.8.2 BSON boundary, typed safe field paths, all standard operators and Boolean groups, explicit existence/non-null totalization, exact null/missing shapes, quoted literal-text PCRE patterns with absolute anchors, root Native and nestable opaque Expr support, stable two-pass validation/emission, deterministic ordered BSON, redacted default-registry preflight failures, shallow escape-hatch cloning, concurrency, fuzz, and benchmark coverage, real MongoDB 6.0.28/8.3.8 final-match-set validation, and retained upstream license attribution.
- The `goqu` Compiler with a locked `[]exp.Expression`/`exp.Expression` boundary, canonical typed fields, immutable MySQL/PostgreSQL profiles, exact field applicability, all standard operators and Boolean groups, SQL NULL totalization, fixed parameterized literal-text lowering, root Native and nestable Expr support, stable two-pass validation/emission, deterministic concurrent compilation, prepared SQL/argument safety coverage, fuzz and benchmark baselines, retained goqu MIT attribution, and real MySQL/PostgreSQL shared semantic validation.
- Generated-field and generated-DAO literal-text fuzz coverage for GORM Gen on both SQL profiles.

### Changed

- GORM and GORM Gen profiles now provide stable English diagnostic strings; Profile, memory State, and memory Ordering integer representations are explicitly non-protocol implementation details.
- Development CI now includes all six modules in the current-core workspace and runs independent `GOWORK=off` verify, tidy, test, race, vet, benchmark, and per-Adapter fuzz-smoke checks.

### Fixed

- Module-zip verification no longer reports a present LICENSE as missing when the archive contains later entries under `pipefail`.
- LDAP final-filter validation now accounts for the bounded extra nesting introduced by presence guards, negative Logic, and already-validated deep Expr text, so legal depth-128 Predicates within the filter-size bound can complete deterministic canonicalization without expanding the raw filter depth limit.

## [memory/v0.1.0-alpha.1], [gormgen/v0.1.0-alpha.1], and [gorm/v0.1.0-alpha.1] - 2026-08-26

### Added

- Apache-2.0 repository licensing and independent-module workspace metadata.
- Repository checks, module-zip license verification, and development CI that operate only on existing Adapter modules.
- The `memory` reference Compiler with typed fields, immutable semantic configuration, all standard operators and Boolean groups, value/null/missing match-set semantics, Native and Expr support, field capability discovery, differential fuzzing, runnable examples, benchmark baselines, and shared `compilertest` coverage.
- The `gormgen` Compiler with locked GORM Gen/GORM compatibility, all standard operators and Boolean groups, SQL NULL totalization, literal text escaping, Native/Expr support, pure-column and value-type discovery, immutable FieldSpec/registry configuration, fixed parameterized templates, real generated-DAO usage, benchmark baselines, and real MySQL/PostgreSQL shared semantic coverage.
- The native `gorm` Compiler with a locked `clause.Expression` C/E boundary, immutable MySQL/PostgreSQL profiles, typed non-raw fields, exact field applicability, all standard operators and Boolean groups, SQL NULL totalization, fixed parameterized literal-text lowering, root Native and nestable Expr support, stable two-pass validation/emission, deterministic concurrent compilation, traditional/generic GORM execution, benchmark and fuzz baselines, DryRun SQL/Vars checks, and real MySQL/PostgreSQL shared semantic coverage.

[Unreleased]: https://github.com/imbrooklyn/weave-adapters/compare/gorm/v0.1.0-alpha.1...HEAD
[memory/v0.1.0-alpha.1]: https://github.com/imbrooklyn/weave-adapters/releases/tag/memory/v0.1.0-alpha.1
[gormgen/v0.1.0-alpha.1]: https://github.com/imbrooklyn/weave-adapters/releases/tag/gormgen/v0.1.0-alpha.1
[gorm/v0.1.0-alpha.1]: https://github.com/imbrooklyn/weave-adapters/releases/tag/gorm/v0.1.0-alpha.1
