package goqu

import (
	"reflect"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
)

func FuzzCompilePreparedLiteralText(f *testing.F) {
	field, err := NewField[string](sqlbuilder.T("records").Col("text_value"))
	if err != nil {
		f.Fatal(err)
	}
	type configuration struct {
		profile  Profile
		factory  *Factory
		baseline string
	}
	configurations := make([]configuration, 0, 2)
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		factory, err := NewFactory(profile)
		if err != nil {
			f.Fatal(err)
		}
		expressions, err := factory.New().Contains(field, "").Build()
		if err != nil {
			f.Fatal(err)
		}
		query, _, err := renderFuzzPrepared(profile, expressions)
		if err != nil {
			f.Fatal(err)
		}
		configurations = append(configurations, configuration{
			profile:  profile,
			factory:  factory,
			baseline: query,
		})
	}

	for _, seed := range []struct {
		profile uint8
		value   string
	}{
		{profile: 0, value: ""},
		{profile: 1, value: "plain"},
		{profile: 0, value: "%_!"},
		{profile: 1, value: "\u4e16\u754c\nend"},
		{profile: 0, value: "x' OR 1=1 -- %_!"},
	} {
		f.Add(seed.profile, seed.value)
	}

	f.Fuzz(func(t *testing.T, selector uint8, value string) {
		if len(value) > 4096 {
			t.Skip()
		}
		configuration := configurations[int(selector)%len(configurations)]
		expressions, err := configuration.factory.New().
			Contains(field, value).
			Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		query, arguments, err := renderFuzzPrepared(
			configuration.profile,
			expressions,
		)
		if err != nil {
			t.Fatalf("ToSQL() error = %v", err)
		}
		if query != configuration.baseline {
			t.Fatalf("prepared SQL changed with the bound value: %q", query)
		}
		wantArguments := []any{"%" + escapeLiteralText(value) + "%"}
		if !reflect.DeepEqual(arguments, wantArguments) {
			t.Fatalf("arguments = %#v, want %#v", arguments, wantArguments)
		}
	})
}

func renderFuzzPrepared(
	profile Profile,
	expressions Expressions,
) (string, []any, error) {
	return sqlbuilder.
		Dialect(profile.dialectName()).
		From(sqlbuilder.T("records")).
		Where(expressions...).
		Prepared(true).
		ToSQL()
}
