# Changelog

This file records notable user-visible changes to Weave Adapters. It follows the structure of [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and uses [Semantic Versioning](https://semver.org/) for independently tagged Adapter module releases.

## [Unreleased]

### Changed

- GORM and GORM Gen profiles now provide stable English diagnostic strings; Profile, memory State, and memory Ordering integer representations are explicitly non-protocol implementation details.

### Fixed

- Module-zip verification no longer reports a present LICENSE as missing when the archive contains later entries under `pipefail`.

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
