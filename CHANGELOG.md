# Changelog

This file records notable user-visible changes to Weave Adapters. It follows the structure of [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and uses [Semantic Versioning](https://semver.org/) for independently tagged Adapter module releases.

## [Unreleased]

### Added

- Apache-2.0 repository licensing and independent-module workspace metadata.
- Repository checks, module-zip license verification, and development CI that operate only on existing Adapter modules.
- The `memory` reference Compiler with typed fields, immutable semantic configuration, all standard operators and Boolean groups, value/null/missing match-set semantics, Native and Expr support, field capability discovery, differential fuzzing, runnable examples, benchmark baselines, and shared `compilertest` coverage.
- The `gormgen` Compiler with locked GORM Gen/GORM compatibility, all standard operators and Boolean groups, SQL NULL totalization, literal text escaping, Native/Expr support, pure-column and value-type discovery, immutable FieldSpec/registry configuration, fixed parameterized templates, real generated-DAO usage, benchmark baselines, and real MySQL/PostgreSQL shared semantic coverage.
- The native `gorm` Compiler with a locked `clause.Expression` C/E boundary, immutable MySQL/PostgreSQL profiles, typed non-raw fields, exact field applicability, all standard operators and Boolean groups, SQL NULL totalization, fixed parameterized literal-text lowering, root Native and nestable Expr support, stable two-pass validation/emission, deterministic concurrent compilation, traditional/generic GORM execution, benchmark and fuzz baselines, DryRun SQL/Vars checks, and real MySQL/PostgreSQL shared semantic coverage.

[Unreleased]: https://github.com/imbrooklyn/weave-adapters/commits/main
