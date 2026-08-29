package ldap

import (
	"errors"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
)

func FuzzParseFilterCanonicalization(f *testing.F) {
	fixture := newLDAPFixture(f)
	for _, source := range []string{
		"(cn=Alice)",
		"(&(cn=*)(uidNumber>=2))",
		"(cn=\\2a\\28\\29\\5c\\00)",
		"(cn=世界)",
		"(memberOf=shared)",
		"(cn:2.5.13.2:=alice)",
		"(&)",
		"(unknown=secret)",
		"(cn=raw\x00nul)",
		string([]byte{'(', 'c', 'n', '=', 0xff, ')'}),
	} {
		f.Add(source)
	}

	f.Fuzz(func(t *testing.T, source string) {
		source = boundedFuzzString(source)
		filter, err := ParseFilter(fixture.schema, source)
		if err != nil {
			if filter.Valid() || filter.String() != "" {
				t.Fatal("ParseFilter failure returned a nonzero Filter")
			}
		} else {
			if !filter.Valid() || filter.String() == "" {
				t.Fatal("ParseFilter success returned an invalid Filter")
			}
			again, parseErr := ParseFilter(fixture.schema, filter.String())
			if parseErr != nil || again.String() != filter.String() {
				t.Fatal("canonical filter is not idempotent")
			}
		}

		sensitive := "LDAP-FUZZ-FILTER-SECRET-" + source
		redacted, redactionErr := ParseFilter(
			fixture.schema,
			"(unknown="+sensitive+")",
		)
		if redacted.Valid() || !errors.Is(redactionErr, weave.ErrInvalidValue) {
			t.Fatal("unknown attribute did not return a zero Filter and invalid-value error")
		}
		if strings.Contains(redactionErr.Error(), sensitive) {
			t.Fatal("ParseFilter error disclosed filter input")
		}
	})
}

func FuzzCompileLiteralEscapingAndRedaction(f *testing.F) {
	fixture := newLDAPFixture(f)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []struct {
		value    string
		contains bool
	}{
		{value: "Alice"},
		{value: "*()\\\x00世界"},
		{value: "*)(|(cn=*))"},
		{value: "", contains: true},
		{value: string([]byte{0xff}), contains: true},
	} {
		f.Add(seed.value, seed.contains)
	}

	f.Fuzz(func(t *testing.T, value string, contains bool) {
		value = boundedFuzzString(value)
		builder := factory.New()
		if contains {
			builder.Contains(fixture.cn, value)
		} else {
			builder.EQ(fixture.cn, value)
		}
		predicate, predicateErr := builder.Predicate()
		if predicateErr != nil {
			t.Fatal("literal predicate construction failed")
		}
		filter, compileErr := factory.Compile(predicate)
		if compileErr != nil {
			if filter.Valid() || filter.String() != "" ||
				!errors.Is(compileErr, weave.ErrCompile) {
				t.Fatal("Compile failure was not structured or returned a nonzero Filter")
			}
		} else {
			if !filter.Valid() || strings.ContainsRune(filter.String(), '\x00') {
				t.Fatal("Compile success returned an invalid or raw-NUL Filter")
			}
			again, repeatedErr := factory.Compile(predicate)
			if repeatedErr != nil || again.String() != filter.String() {
				t.Fatal("repeated Compile changed canonical output")
			}
		}

		sensitive := "LDAP-FUZZ-VALUE-SECRET-" + value
		redacted, redactionErr := factory.New().EQ(fixture.uid, sensitive).Build()
		if redacted.Valid() || !errors.Is(redactionErr, weave.ErrInvalidValue) ||
			!errors.Is(redactionErr, weave.ErrCompile) {
			t.Fatal("wrong typed value did not return a structured zero result")
		}
		if strings.Contains(redactionErr.Error(), sensitive) {
			t.Fatal("Compile error disclosed query value")
		}
	})
}

func boundedFuzzString(value string) string {
	const maximum = 4 * 1024
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}
