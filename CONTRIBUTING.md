# Contributing to Weave Adapters

Thank you for helping improve Weave Adapters. Contributions should keep each Adapter independently buildable, preserve Weave semantics exactly, and limit changes to modules that provide real behavior.

## Requirements

- Go 1.27 or newer.
- Git.
- Bash for repository scripts.
- Any service required by the Adapter being changed.

The current memory module depends on a Weave core line that has not been tagged. Its standalone `GOWORK=off` dependency check is therefore not yet available. Development CI uses the pinned public Weave revision shown in the workflow; it does not add a `replace` directive to the module.

## Make a focused change

- Keep public identifiers, GoDoc, comments, examples, tests, scripts, workflow text, and documentation in English.
- Keep every Adapter in an independent directory with its own `go.mod` and version lifecycle.
- Do not import another Adapter module.
- Do not add local `replace` directives to a committed `go.mod`.
- Add a module directory only when it contains an implementation that can be tested; do not add placeholder modules.
- Update the root `go.work` in the same change whenever an Adapter module is added or removed.
- Keep Compiler values request-stateless. They must not retain records, database handles, collections, sessions, contexts, loggers, transactions, or per-request values.
- Preserve Weave's exact Boolean, null/missing, literal-text, ownership, capability, and error contracts. Do not silently ignore or approximate unsupported behavior.
- Treat `Native` and `Expr` as explicit caller-owned escape hatches.

## Format and verify

First ensure the active Go environment can resolve the module dependencies. Then run:

```sh
./scripts/verify.sh
```

For concurrency-sensitive changes, also run the race detector in each affected module:

```sh
cd memory
go test -race ./...
```

The repository check can be run separately without downloading dependencies:

```sh
./scripts/check-repository.sh
```

Before an independent module release, `GOWORK=off go test ./...` and `GOWORK=off go vet ./...` must pass with published dependencies. After the candidate tag is resolvable, verify the actual archive with `./scripts/check-module-zip.sh module-path@version`.

## Tests and documentation

Add tests for every behavior change and regression fix. Tests must cover the exact semantic boundary being changed and must not expose query values, field values, native payloads, expression payloads, or credentials in failure text.

Every exported symbol needs accurate GoDoc. Update the relevant module README and this changelog when behavior visible to module users changes. Documentation must describe current behavior and current compatibility, not unimplemented Adapter modules.

## Pull requests

A pull request should:

- Explain the user-visible problem and resulting behavior.
- Keep unrelated refactoring separate.
- Identify every affected module and dependency boundary.
- Include tests that would fail without the change.
- List the exact verification commands and results.
- Call out semantic, compatibility, ownership, concurrency, security, and licensing effects.
- Avoid generated output, local workspaces, coverage data, profiles, credentials, and editor files.

By contributing, you agree that your contribution is licensed under the repository's [Apache License 2.0](LICENSE).
