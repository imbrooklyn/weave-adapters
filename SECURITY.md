# Security Policy

## Supported versions

The repository does not currently declare a tagged support line. Security fixes are prepared on the default branch.

## Report a vulnerability

Please report suspected vulnerabilities privately through [GitHub Security Advisories](https://github.com/imbrooklyn/weave-adapters/security/advisories/new). Do not open a public issue for an uncoordinated vulnerability.

Include enough information to reproduce and assess the issue without including production credentials or unrelated personal data:

- Affected Adapter module, API, and revision or version.
- Security impact and realistic attack conditions.
- Minimal, redacted reproduction or test case.
- Whether a standard operator, `Native`, or `Expr` is involved.
- Any known mitigation.

Maintainers will use the private advisory to confirm scope, coordinate a fix, and prepare disclosure. Public details should be released only after affected users have a reasonable remediation path.

## Security boundaries

Adapter Compilers translate predicates; they do not execute queries or own backend clients. A Compiler must not retain records, database handles, collections, sessions, contexts, loggers, transactions, credentials, or per-request query state.

Standard field and value paths must use typed descriptors and the target backend's safe parameter or expression mechanisms. Errors must not expose field values, query values, `Native` payloads, `Expr` payloads, or credentials.

`Native(C)` and `Expr(E)` are explicit escape hatches. Callers are responsible for their backend validity, Boolean meaning, parameterization, escaping, immutability, and concurrency safety. Passing untrusted raw query material through either path is outside the guarantees of standard operators.

The memory module borrows Accessor and Semantics functions. Callers must keep captured state deterministic and concurrency-safe while Fields, Predicates, compiled Conditions, or Expressions may use it.
