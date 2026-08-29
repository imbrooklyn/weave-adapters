# Weave LDAP Adapter

This independent Go module compiles [Weave](https://github.com/imbrooklyn/weave) predicates into deterministic RFC 4515 LDAP search filters. A successful `Filter.String()` can be passed directly to `github.com/go-ldap/ldap/v3.NewSearchRequest`.

The module is in pre-release development. Its public module file contains no local `replace` directive. The Compiler emits filter text only; it does not connect to, inspect, bind to, or negotiate with an LDAP server.

## Version matrix

| Component | Version or status |
| --- | --- |
| Weave LDAP Adapter | Unreleased pre-release source |
| Go | 1.27 or newer |
| Weave | `v0.1.0-alpha.1` |
| go-ldap | `v3.4.14` |
| Filter profile | `RFC4515` |
| Real-server validation | OpenLDAP `2.6.10` |

## Type boundary

```go
type Filter struct { /* immutable, unexported state */ }
type Expression = string
```

`Filter` is an immutable canonical filter value bound to the `Schema` that validated it. The zero value is invalid. `ParseFilter` and successful compilation are the only public construction paths, and `String` returns the text used by a go-ldap search request.

`Expression` deliberately remains the open string carrier used by Weave Expr nodes. The Adapter does not impose a Boolean wrapper. Expr remains nestable, while Native remains root-only and accepts only a valid `Filter` created for the same Schema.

## Example

```go
package example

import (
    ldapv3 "github.com/go-ldap/ldap/v3"
    "github.com/imbrooklyn/weave"
    weaveldap "github.com/imbrooklyn/weave-adapters/ldap"
)

func userSearch() (*ldapv3.SearchRequest, error) {
    rules, err := weaveldap.NewMatchingRules(
        "2.5.13.2", // caseIgnoreMatch
        "2.5.13.3", // caseIgnoreOrderingMatch
        "2.5.13.4", // caseIgnoreSubstringsMatch
    )
    if err != nil {
        return nil, err
    }
    commonName, err := weaveldap.NewAttribute[string](weaveldap.AttributeSpec{
        Description:  "cn",
        OID:          "2.5.4.3",
        SingleValued: true,
        Syntax:       weaveldap.SyntaxDirectoryString,
        Matching:     rules,
        Operators: weave.NewOperatorSet(
            weave.OperatorEQ,
            weave.OperatorNEQ,
            weave.OperatorIn,
            weave.OperatorNotIn,
            weave.OperatorNotNull,
            weave.OperatorContains,
            weave.OperatorHasPrefix,
            weave.OperatorHasSuffix,
        ),
    })
    if err != nil {
        return nil, err
    }
    schema, err := weaveldap.NewSchema(commonName)
    if err != nil {
        return nil, err
    }
    factory, err := weaveldap.NewFactory(weaveldap.RFC4515, schema)
    if err != nil {
        return nil, err
    }
    filter, err := factory.New().
        HasPrefix(commonName, "A*(").
        NotNull(commonName).
        Build()
    if err != nil {
        return nil, err
    }

    return ldapv3.NewSearchRequest(
        "ou=people,dc=example,dc=org",
        ldapv3.ScopeWholeSubtree,
        ldapv3.NeverDerefAliases,
        100,
        10,
        false,
        filter.String(),
        []string{"dn", "cn"},
        nil,
    ), nil
}
```

The literal text `A*(` is escaped as an assertion value. It cannot add a substring delimiter or filter group.

## Typed attributes and Schema

`Attribute[T]` is an immutable typed declaration. `AttributeSpec` fixes all of these properties:

- one option-free RFC 4512 attribute description;
- one numeric attribute type OID;
- an explicit single-valued or multi-valued declaration;
- one supported LDAP syntax and compatible Go query type;
- numeric equality, ordering, and substring matching-rule OIDs;
- the exact immutable `weave.OperatorSet` applicable to the attribute.

Attribute descriptions are ASCII case-insensitive and canonicalized to lowercase. Standard lowering always emits the numeric attribute OID, so generated filters do not depend on an alias spelling. Attribute options such as `;binary` are not accepted.

The built-in syntax declarations are:

| Syntax | Compatible Go query type | LDAP syntax OID |
| --- | --- | --- |
| `SyntaxDirectoryString` | string or defined string | `1.3.6.1.4.1.1466.115.121.1.15` |
| `SyntaxIA5String` | ASCII string or defined string | `1.3.6.1.4.1.1466.115.121.1.26` |
| `SyntaxInteger` | signed/unsigned integer or defined integer | `1.3.6.1.4.1.1466.115.121.1.27` |
| `SyntaxBoolean` | bool or defined bool | `1.3.6.1.4.1.1466.115.121.1.7` |
| `SyntaxGeneralizedTime` | `time.Time` | `1.3.6.1.4.1.1466.115.121.1.24` |
| `SyntaxOctetString` | byte slice or defined byte slice | `1.3.6.1.4.1.1466.115.121.1.40` |

Directory String values must be non-empty valid UTF-8. IA5 String values may be empty but every byte must be ASCII. NUL is a valid syntax byte where applicable and is always emitted as the RFC 4515 `\00` escape. Generalized Time values are converted to UTC while preserving fractional seconds; integer, Boolean, and octet encodings are deterministic.

`NewAttribute` checks every declared operator against cardinality, syntax, Go type, and matching rules. Equality and membership require an equality rule. LTE requires both equality and ordering rules; GTE requires ordering. Between requires both rules and `SyntaxInteger`. Literal substring operators require a string syntax plus equality and substring rules. NotNull needs only a single-valued declaration.

The descriptor is an application assertion about the deployed schema, not schema discovery. Every declared description/OID alias, `SINGLE-VALUE` property, syntax, and matching rule must agree with the live attribute type. The server must implement the declared matching rules and make them applicable to the attribute. A successful local compilation does not prove those deployment prerequisites.

`NewSchema` accepts differently typed Attribute values, rejects zero, forged, duplicate-description, and duplicate-OID entries, and creates immutable description/OID indexes. Attribute declarations must come from application-controlled schema metadata. Map an external field name through predeclared descriptors; never construct schema metadata directly from an untrusted request.

## Operator and feature matrix

Let `a` be the descriptor's numeric attribute OID and let every `v` be syntax-encoded and escaped with `ldap.EscapeFilter` from go-ldap v3.4.14.

| Operation or feature | `RFC4515` | Exact standard lowering or boundary |
| --- | --- | --- |
| EQ | Yes | `(&(a=*)(a=v))` |
| NEQ | Yes | `(&(a=*)(!(a=v)))` |
| LT | No | LDAP has no strict less-than assertion |
| LTE | Yes | `(&(a=*)(a<=v))` |
| GT | No | LDAP has no strict greater-than assertion |
| GTE | Yes | `(&(a=*)(a>=v))` |
| In | Yes | `(&(a=*)(|(a=v1)...(a=vn)))` |
| NotIn | Yes | `(&(a=*)(!(|(a=v1)...(a=vn))))` |
| Between | Yes | Integer, inclusive: `(&(a=*)(a>=lo)(a<=hi))` |
| IsNull | No | LDAP has no portable explicit NULL value |
| NotNull | Yes | Attribute presence: `(a=*)` |
| Contains | Yes | Literal substring: `(&(a=*)(a=*v*))` |
| HasPrefix | Yes | Literal prefix: `(&(a=*)(a=v*))` |
| HasSuffix | Yes | Literal suffix: `(&(a=*)(a=*v))` |
| AllOf, AnyOf | Yes | Non-empty `&` and `|` filters |
| NoneOf, NotAllOf | Yes | NOT around `|` and `&`, respectively |
| Normalized constants | Yes | True `(|(2.5.4.0=*)(!(2.5.4.0=*)))`; false `(&(2.5.4.0=*)(!(2.5.4.0=*)))` |
| Native condition | Yes | Root-only, valid same-Schema `Filter` |
| Expr expression | Yes | Nestable validated string filter |

The global set has 11 standard operators. `Compiler.CapabilitiesFor` returns the exact per-attribute subset. LT and GT are not approximated as negated LTE or GTE: LDAP filter evaluation can be Undefined for an absent attribute, unavailable matching rule, or incomparable value, so those negations are not exact strict-order operations.

## Absence, explicit NULL, and multi-valued attributes

LDAP represents an absent attribute with no values. It does not define one portable explicit NULL assertion value across attribute syntaxes and servers. Where the declared syntax permits them, an empty string or empty octet string is an ordinary value; zero integer and FALSE are also ordinary values, not NULL. Consequently:

- `NotNull` means attribute presence;
- `IsNull` is not advertised;
- ordinary positive and negative standard leaves include a presence guard;
- the inner NOT used by `NEQ` and `NotIn` therefore does not make absence match;
- outer `NoneOf` and `NotAllOf` Logic complements the already-guarded child
  match sets over the search universe, so an absent attribute can match that
  outer complement.

LDAP matching over a multi-valued attribute commonly means that at least one value satisfies an assertion. That is not the single-valued logical-field contract of Weave's standard operators. A descriptor with `SingleValued: false` must therefore have an empty standard OperatorSet. It may still be registered for validated Expr filters. Future LDAP-specific element or set APIs can model other semantics without changing the standard operator contract.

## Filter grammar, escaping, and canonicalization

The Adapter has its own strict validation layer before the locked go-ldap codec. It accepts this fixed subset:

- non-empty AND and OR, and NOT with exactly one child;
- equality, greater-or-equal, less-or-equal, presence, and substring assertions;
- attribute-bound extensible matches;
- valid UTF-8 and RFC 4515 `\HH` assertion-value escapes;
- option-free allowlisted attribute descriptions or numeric OIDs;
- numeric extensible matching-rule OIDs assigned to that attribute;
- `:dn` only on an attribute-bound extensible match.

Raw ParseFilter and Expr text is limited to 64 KiB and recursive depth `weave.MaxPredicateDepth`. Both limits are enforced before invoking the upstream recursive parser and are checked again after canonicalization. Final generated text has a separate bounded depth allowance for the presence guards, negative Logic wrappers, and already-validated nested Expr text introduced while lowering any legal depth-128 Predicate; this allowance does not expand the accepted raw grammar or core Predicate depth.

It rejects empty AND/OR, approximate match, attribute options, attribute-less extensible match, unknown attributes or matching rules, consecutive substring delimiters, raw NUL/parentheses/backslash in assertion values, malformed escapes, and invalid UTF-8.

After strict validation, the Adapter calls these real go-ldap v3.4.14 APIs:

```go
func CompileFilter(filter string) (*ber.Packet, error)
func DecompileFilter(packet *ber.Packet) (string, error)
func EscapeFilter(filter string) string
```

Generated and Expr filters both pass compile/decompile, strict revalidation, and a second compile/decompile idempotence check. Canonical output preserves child order, uses go-ldap's lowercase hexadecimal escapes, and escapes non-ASCII assertion bytes. Stability is promised within the locked Adapter and go-ldap version, not across a future dependency upgrade. The Adapter never treats a successful `CompileFilter` call alone as full RFC or Schema validation. For raw Expr, syntax validation is lexical and structural; the caller remains responsible for supplying an assertion value valid for its selected attribute syntax or explicit matching rule.

## Native and Expr responsibility

`ParseFilter` creates a Schema-bound Filter for reviewed Native use. A Native value from a different Schema, a zero Filter, or a Native node below another group is rejected.

Expr keeps the native string representation open and can express reviewed filters for a registered multi-valued attribute or an allowlisted extensible matching rule. The Adapter still enforces grammar, attribute and rule allowlists, and canonicalization. The caller remains responsible for business meaning, directory ACLs, matching-rule intent, server-specific cost, and never concatenating untrusted input into Expr. Assertion values in standard operators are the safer path and are always escaped by the locked upstream function.

## Errors, concurrency, and ownership

Compilation uses a complete stable preorder validation pass followed by a separate deterministic emission pass. Failures return a zero Filter and a location-aware `*weave.Error`. Error text and retained causes do not contain attribute identifiers, query values, raw filters, Native or Expr payloads, DNs, bind identities, credentials, or logger state.

A configured Compiler retains only its immutable Profile and Schema and is safe for concurrent use. It does not retain a connection, search request, context, bind credential, logger, session, or per-request value. Filter and Attribute values contain only immutable package-owned state; Expression strings follow normal Go string immutability.

Successful `Filter.String()` and raw Expression values can contain query data. Applications own their logging and transport redaction policy; the Adapter's no-payload guarantee applies to returned compilation errors, not to successful query values explicitly requested by the caller.

## Verification

From this directory, with dependencies available in the active Go environment:

```sh
gofmt -w *.go
GOWORK=off go mod verify
GOWORK=off go mod tidy -diff
GOWORK=off go test ./...
GOWORK=off go test -run '^$' -fuzz '^FuzzParseFilterCanonicalization$' -fuzztime=10s
GOWORK=off go test -run '^$' -fuzz '^FuzzCompileLiteralEscapingAndRedaction$' -fuzztime=10s
GOWORK=off go test -run '^$' -bench '^Benchmark(FilterEmit|Compile)' -benchmem
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

The module tests cover descriptor invariants, exact capabilities and field applicability, strict filter acceptance and rejection, canonicalization idempotence, all 11 standard lowerings, constants, Boolean groups, Schema-bound Native, validated Expr, stable first errors, zero-result failures, redacted errors, and concurrent deterministic compilation. Fuzz targets exercise grammar/canonicalization and literal escaping/redaction; benchmarks isolate emission, 1,024-value In, depth-128 Logic, and repeated/concurrent Compile.

The integration testbed additionally executes the shared Compiler contract and dedicated escaping, absence, empty-value, multi-value, Native/Expr, deep-logic, and concurrency checks against OpenLDAP 2.6.10. This is an exact tested-server statement, not a compatibility claim for other OpenLDAP releases or LDAP products. Applications must validate their configured schema and matching rules against every deployed directory profile.

## License

Repository-owned code and documentation are licensed under the [Apache License 2.0](../LICENSE). The locked `github.com/go-ldap/ldap/v3` dependency is distributed under the MIT License; exact-version attribution and the upstream license text are retained in [Third-Party Notices](THIRD_PARTY_NOTICES.md). go-ldap and LDAP server projects are independent from Weave; their names identify dependencies and compatibility boundaries only.
