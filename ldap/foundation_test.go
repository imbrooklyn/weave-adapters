package ldap

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
)

type ldapFixture struct {
	schema   Schema
	cn       Attribute[string]
	uid      Attribute[int64]
	memberOf Attribute[string]
}

func newLDAPFixture(t testing.TB) ldapFixture {
	t.Helper()
	caseIgnore, err := NewMatchingRules("2.5.13.2", "2.5.13.3", "2.5.13.4")
	if err != nil {
		t.Fatal(err)
	}
	integer, err := NewMatchingRules("2.5.13.14", "2.5.13.15", "")
	if err != nil {
		t.Fatal(err)
	}
	cn, err := NewAttribute[string](AttributeSpec{
		Description:  "CN",
		OID:          "2.5.4.3",
		SingleValued: true,
		Syntax:       SyntaxDirectoryString,
		Matching:     caseIgnore,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorLTE,
			weave.OperatorGTE,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorNotNull,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	uid, err := NewAttribute[int64](AttributeSpec{
		Description:  "uidNumber",
		OID:          "1.3.6.1.1.1.1.0",
		SingleValued: true,
		Syntax:       SyntaxInteger,
		Matching:     integer,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorLTE,
			weave.OperatorGTE,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorBetween,
			weave.OperatorNotNull,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	memberOf, err := NewAttribute[string](AttributeSpec{
		Description:  "memberOf",
		OID:          "1.2.840.113556.1.2.102",
		SingleValued: false,
		Syntax:       SyntaxDirectoryString,
		Matching:     caseIgnore,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := NewSchema(cn, uid, memberOf)
	if err != nil {
		t.Fatal(err)
	}
	return ldapFixture{schema: schema, cn: cn, uid: uid, memberOf: memberOf}
}

func TestProfileCompilerAndCapabilities(t *testing.T) {
	fixture := newLDAPFixture(t)
	if RFC4515.String() != "rfc4515" || Profile(99).String() != "profile(99)" {
		t.Fatal("Profile.String() is not stable")
	}
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := compiler.Capabilities()
	if capabilities.Operators.Count() != 11 || capabilities.Features.Count() != 2 {
		t.Fatalf("capabilities = %#v, want 11 operators and 2 features", capabilities)
	}
	for _, unsupported := range []weave.Operator{
		weave.OperatorLT,
		weave.OperatorGT,
		weave.OperatorIsNull,
	} {
		if capabilities.Operators.Has(unsupported) {
			t.Fatalf("unexpected capability %s", unsupported)
		}
	}
	cnCapabilities, err := compiler.CapabilitiesFor(fixture.cn)
	if err != nil || cnCapabilities.Operators.Count() != 10 {
		t.Fatalf("CapabilitiesFor(cn) = (%#v, %v)", cnCapabilities, err)
	}
	multiCapabilities, err := compiler.CapabilitiesFor(fixture.memberOf)
	if err != nil || multiCapabilities.Operators.Count() != 0 {
		t.Fatalf("CapabilitiesFor(memberOf) = (%#v, %v)", multiCapabilities, err)
	}
	if got := (Compiler{}).Capabilities(); got != (weave.Capabilities{}) {
		t.Fatalf("zero Compiler capabilities = %#v", got)
	}
	if _, err := NewCompiler(0, fixture.schema); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("NewCompiler(invalid profile) error = %v", err)
	}
	if _, err := NewCompiler(RFC4515, Schema{}); !errors.Is(err, weave.ErrInvalidState) {
		t.Fatalf("NewCompiler(invalid schema) error = %v", err)
	}
}

func TestCompilerStateRetainsOnlyProfileAndSchema(t *testing.T) {
	stateType := reflect.TypeFor[compilerState]()
	if stateType.NumField() != 2 || stateType.Field(0).Name != "profile" ||
		stateType.Field(1).Name != "schema" {
		t.Fatalf("compilerState shape = %v", stateType)
	}
	for _, banned := range []string{
		"conn", "connection", "request", "context", "credential", "bind", "logger",
	} {
		for index := range stateType.NumField() {
			if strings.Contains(strings.ToLower(stateType.Field(index).Name), banned) {
				t.Fatalf("compilerState unexpectedly retains %q", banned)
			}
		}
	}
}

func TestAttributeAndSchemaValidation(t *testing.T) {
	fixture := newLDAPFixture(t)
	if fixture.cn.Description() != "cn" || fixture.cn.OID() != "2.5.4.3" ||
		!fixture.cn.SingleValued() || fixture.cn.Syntax() != SyntaxDirectoryString {
		t.Fatalf("cn descriptor = %#v", fixture.cn)
	}
	if fixture.schema.AttributeCount() != 3 {
		t.Fatalf("AttributeCount() = %d", fixture.schema.AttributeCount())
	}

	rules, err := NewMatchingRules("2.5.13.2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, operators := range []weave.OperatorSet{
		weave.NewOperatorSet(weave.OperatorLT),
		weave.NewOperatorSet(weave.OperatorGT),
		weave.NewOperatorSet(weave.OperatorIsNull),
	} {
		if _, err := NewAttribute[string](AttributeSpec{
			Description:  "testValue",
			OID:          "1.3.6.1.4.1.55555.1",
			SingleValued: true,
			Syntax:       SyntaxDirectoryString,
			Matching:     rules,
			Operators:    operators,
		}); !errors.Is(err, weave.ErrOperatorNotApplicable) {
			t.Fatalf("unsupported descriptor operator error = %v", err)
		}
	}
	if _, err := NewAttribute[string](AttributeSpec{
		Description:  "member",
		OID:          "2.5.4.31",
		SingleValued: false,
		Syntax:       SyntaxDirectoryString,
		Matching:     rules,
		Operators:    weave.NewOperatorSet(weave.OperatorEQ),
	}); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("multi-valued operator error = %v", err)
	}
	if _, err := NewAttribute[string](AttributeSpec{
		Description:  "bad;binary",
		OID:          "1.3.6.1.4.1.55555.2",
		SingleValued: true,
		Syntax:       SyntaxDirectoryString,
		Matching:     rules,
	}); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("attribute option error = %v", err)
	}
	if _, err := NewSchema(fixture.cn, fixture.cn); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("duplicate schema error = %v", err)
	}
	orderingOnly, err := NewMatchingRules("", "2.5.13.3", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAttribute[string](AttributeSpec{
		Description:  "orderedValue",
		OID:          "1.3.6.1.4.1.55555.3",
		SingleValued: true,
		Syntax:       SyntaxDirectoryString,
		Matching:     orderingOnly,
		Operators:    weave.NewOperatorSet(weave.OperatorLTE),
	}); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("LTE without equality rule error = %v", err)
	}
	substringOnly, err := NewMatchingRules("", "", "2.5.13.4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAttribute[string](AttributeSpec{
		Description:  "substringValue",
		OID:          "1.3.6.1.4.1.55555.4",
		SingleValued: true,
		Syntax:       SyntaxDirectoryString,
		Matching:     substringOnly,
		Operators:    weave.NewOperatorSet(weave.OperatorContains),
	}); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("substring without equality rule error = %v", err)
	}

	foreign, err := NewAttribute[string](AttributeSpec{
		Description:  "displayName",
		OID:          "2.16.840.1.113730.3.1.241",
		SingleValued: true,
		Syntax:       SyntaxDirectoryString,
		Matching:     rules,
		Operators:    weave.NewOperatorSet(weave.OperatorEQ),
	})
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.CapabilitiesFor(foreign); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("foreign Attribute error = %v", err)
	}
}

func TestSyntaxEncodingsAreDeterministic(t *testing.T) {
	type namedInteger int32
	type namedBoolean bool
	type namedBytes []byte

	tests := []struct {
		name   string
		syntax Syntax
		value  any
		want   string
	}{
		{name: "directory", syntax: SyntaxDirectoryString, value: "Grüße", want: "Grüße"},
		{name: "ia5", syntax: SyntaxIA5String, value: "mail@example.org", want: "mail@example.org"},
		{name: "integer", syntax: SyntaxInteger, value: namedInteger(-42), want: "-42"},
		{name: "boolean", syntax: SyntaxBoolean, value: namedBoolean(true), want: "TRUE"},
		{
			name:   "generalized time",
			syntax: SyntaxGeneralizedTime,
			value: time.Date(
				2026, time.August, 29, 12, 34, 56, 123400000,
				time.FixedZone("offset", 8*60*60),
			),
			want: "20260829043456.1234Z",
		},
		{name: "octets", syntax: SyntaxOctetString, value: namedBytes{0, '*'}, want: "\x00*"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attribute := &attributeState{
				syntax:    test.syntax,
				valueType: reflect.TypeOf(test.value),
			}
			got, err := encodeAttributeValue(attribute, test.value)
			if err != nil || got != test.want {
				t.Fatalf("encodeAttributeValue() = (%q, %v), want %q", got, err, test.want)
			}
		})
	}

	attribute := &attributeState{
		syntax:    SyntaxDirectoryString,
		valueType: reflect.TypeFor[string](),
	}
	if _, err := encodeAttributeValue(attribute, ""); !errors.Is(err, errInvalidAssertionValue) {
		t.Fatalf("empty Directory String error = %v", err)
	}
	attribute.syntax = SyntaxIA5String
	if _, err := encodeAttributeValue(attribute, "é"); !errors.Is(err, errInvalidAssertionValue) {
		t.Fatalf("non-ASCII IA5 String error = %v", err)
	}
}

func TestStrictFilterValidationAndCanonicalization(t *testing.T) {
	fixture := newLDAPFixture(t)
	valid := map[string]string{
		"(cn=Alice)":                  "(cn=Alice)",
		"(CN=Alice)":                  "(CN=Alice)",
		"(&(cn=Alice)(uidNumber>=2))": "(&(cn=Alice)(uidNumber>=2))",
		"(cn=\\2A\\28\\29\\5C\\00)":   "(cn=\\2a\\28\\29\\5c\\00)",
		"(cn=中文)":                     "(cn=\\e4\\b8\\ad\\e6\\96\\87)",
		"(memberOf=admins)":           "(memberOf=admins)",
		"(cn:2.5.13.2:=alice)":        "(cn:2.5.13.2:=alice)",
	}
	for source, want := range valid {
		filter, err := ParseFilter(fixture.schema, source)
		if err != nil {
			t.Errorf("ParseFilter(valid) error = %v", err)
			continue
		}
		if filter.String() != want {
			t.Errorf("ParseFilter() = %q, want %q", filter.String(), want)
		}
		again, err := ParseFilter(fixture.schema, filter.String())
		if err != nil || again.String() != filter.String() {
			t.Errorf("canonical filter is not idempotent: (%q, %v)", again.String(), err)
		}
	}

	invalid := []string{
		"(&)",
		"(|)",
		"(cn=raw\x00nul)",
		"(\\2a=value)",
		"(unknown=top-secret)",
		"(cn;binary=value)",
		"(cn~=value)",
		"(:2.5.13.2:=value)",
		"(cn:1.2.3:=value)",
		"(cn=**)",
		"(cn=\\zz)",
		"(!(cn=a)(cn=b))",
		strings.Repeat("(!", weave.MaxPredicateDepth+2) + "(cn=a)" +
			strings.Repeat(")", weave.MaxPredicateDepth+2),
		"(cn=" + strings.Repeat("x", maxFilterTextBytes) + ")",
	}
	for _, source := range invalid {
		filter, err := ParseFilter(fixture.schema, source)
		if filter.Valid() || !errors.Is(err, weave.ErrInvalidValue) {
			t.Errorf("ParseFilter(invalid) = (%q, %v)", filter.String(), err)
		}
		if err != nil && containsSensitiveFilterText(err.Error(), source, "top-secret", "unknown") {
			t.Errorf("ParseFilter error disclosed input: %q", err)
		}
	}
}

func TestExprGrammarHonorsMatchingRulePrerequisites(t *testing.T) {
	orderingOnly, err := NewMatchingRules("", "2.5.13.3", "")
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := NewAttribute[string](AttributeSpec{
		Description:  "orderedValue",
		OID:          "1.3.6.1.4.1.55555.10",
		SingleValued: false,
		Syntax:       SyntaxDirectoryString,
		Matching:     orderingOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	substringOnly, err := NewMatchingRules("", "", "2.5.13.4")
	if err != nil {
		t.Fatal(err)
	}
	substring, err := NewAttribute[string](AttributeSpec{
		Description:  "substringValue",
		OID:          "1.3.6.1.4.1.55555.11",
		SingleValued: false,
		Syntax:       SyntaxDirectoryString,
		Matching:     substringOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := NewSchema(ordered, substring)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFilter(schema, `(orderedValue>=x)`); err != nil {
		t.Fatalf("GTE with ordering rule error = %v", err)
	}
	for _, source := range []string{
		`(orderedValue<=x)`,
		`(substringValue=*x*)`,
	} {
		if _, err := ParseFilter(schema, source); !errors.Is(err, weave.ErrInvalidValue) {
			t.Fatalf("ParseFilter(%q) error = %v", source, err)
		}
	}
}

func TestCompileRejectsInvalidPredicate(t *testing.T) {
	fixture := newLDAPFixture(t)
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := compiler.Compile(weave.Predicate[Filter, Expression]{})
	if filter.Valid() || !errors.Is(err, weave.ErrInvalidPredicate) ||
		!errors.Is(err, weave.ErrCompile) {
		t.Fatalf("Compile(zero) = (%q, %v)", filter.String(), err)
	}
	filter, err = (Compiler{}).Compile(weave.Predicate[Filter, Expression]{})
	if filter.Valid() || !errors.Is(err, weave.ErrInvalidState) {
		t.Fatalf("zero Compiler.Compile() = (%q, %v)", filter.String(), err)
	}
}

func containsSensitiveFilterText(errorText string, values ...string) bool {
	for _, value := range values {
		if value != "" && strings.Contains(errorText, value) {
			return true
		}
	}
	return false
}
