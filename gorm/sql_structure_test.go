package gorm_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	weavegorm "github.com/imbrooklyn/weave-adapters/gorm"
	"github.com/imbrooklyn/weave-adapters/gorm/internal/fixture/usage"
	"gorm.io/gorm/clause"
)

func TestEveryStandardOperatorDryRunKeepsValuesBound(t *testing.T) {
	queryText := "x' OR 1=1 -- %_! \u4e16\u754c\\end"
	pattern := "%x' OR 1=1 -- !%!_!! \u4e16\u754c\\end%"
	tests := []struct {
		name         string
		build        func(*weave.Builder[weavegorm.Condition, weavegorm.Expression])
		wantFragment string
		wantVars     []any
	}{
		{name: "eq", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.EQ(scoreField(), int64(7))
		}, wantFragment: " = ", wantVars: []any{int64(7)}},
		{name: "neq", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.NEQ(scoreField(), int64(7))
		}, wantFragment: " <> ", wantVars: []any{int64(7)}},
		{name: "lt", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.LT(scoreField(), int64(7))
		}, wantFragment: " < ", wantVars: []any{int64(7)}},
		{name: "lte", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.LTE(scoreField(), int64(7))
		}, wantFragment: " <= ", wantVars: []any{int64(7)}},
		{name: "gt", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.GT(scoreField(), int64(7))
		}, wantFragment: " > ", wantVars: []any{int64(7)}},
		{name: "gte", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.GTE(scoreField(), int64(7))
		}, wantFragment: " >= ", wantVars: []any{int64(7)}},
		{name: "in", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.In(scoreField(), []int64{7, 11})
		}, wantFragment: " IN (", wantVars: []any{int64(7), int64(11)}},
		{name: "not in", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.NotIn(scoreField(), []int64{7, 11})
		}, wantFragment: " NOT IN (", wantVars: []any{int64(7), int64(11)}},
		{name: "between", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.Between(scoreField(), int64(7), int64(11))
		}, wantFragment: " >= ", wantVars: []any{int64(7), int64(11)}},
		{name: "is null", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) { builder.IsNull(nameField()) }, wantFragment: " IS NULL", wantVars: []any{}},
		{name: "not null", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) { builder.NotNull(nameField()) }, wantFragment: " IS NOT NULL", wantVars: []any{}},
		{name: "contains", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.Contains(nameField(), queryText)
		}, wantFragment: " ESCAPE '!'", wantVars: []any{pattern}},
		{name: "prefix", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.HasPrefix(nameField(), queryText)
		}, wantFragment: " ESCAPE '!'", wantVars: []any{strings.TrimPrefix(pattern, "%")}},
		{name: "suffix", build: func(builder *weave.Builder[weavegorm.Condition, weavegorm.Expression]) {
			builder.HasSuffix(nameField(), queryText)
		}, wantFragment: " ESCAPE '!'", wantVars: []any{strings.TrimSuffix(pattern, "%")}},
	}

	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					factory, err := weavegorm.NewFactory(profileForFixture(fixture))
					if err != nil {
						t.Fatalf("NewFactory() error = %v", err)
					}
					builder := factory.New()
					test.build(builder)
					condition, err := builder.Build()
					if err != nil {
						t.Fatalf("Build() error = %v", err)
					}
					statement := usage.Traditional(fixture.database, condition).Statement
					if statement.Error != nil {
						t.Fatalf("DryRun build error = %v", statement.Error)
					}
					sqlText := statement.SQL.String()
					if !strings.Contains(sqlText, test.wantFragment) {
						t.Fatalf("SQL = %q, want fragment %q", sqlText, test.wantFragment)
					}
					if !reflect.DeepEqual(statement.Vars, test.wantVars) {
						t.Fatalf("Vars = %#v, want %#v", statement.Vars, test.wantVars)
					}
					if strings.Contains(sqlText, queryText) {
						t.Fatalf("SQL contains query text: %q", sqlText)
					}
					assertNoColumnVars(t, statement.Vars)
				})
			}
		})
	}
}

func TestCompiledLiteralTemplateQuotesColumnAndBindsOnlyPattern(t *testing.T) {
	const value = "50%_!done"
	const wantPattern = "%50!%!_!!done%"
	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			factory, err := weavegorm.NewFactory(profileForFixture(fixture))
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			condition, err := factory.New().Contains(nameField(), value).Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			statement := usage.Traditional(fixture.database, condition).Statement
			if statement.Error != nil {
				t.Fatalf("DryRun error = %v", statement.Error)
			}
			sqlText := statement.SQL.String()
			assertQuotedColumn(t, fixture, sqlText)
			if !strings.Contains(sqlText, " LIKE "+fixture.placeholder+" ESCAPE '!'") {
				t.Fatalf("literal SQL = %q", sqlText)
			}
			if strings.Contains(sqlText, value) ||
				!reflect.DeepEqual(statement.Vars, []any{wantPattern}) {
				t.Fatalf("literal SQL/Vars = %q / %#v", sqlText, statement.Vars)
			}
			t.Logf("compiled literal SQL/Vars: %s | %#v", sqlText, statement.Vars)
		})
	}
}

func TestGroupPrecedenceAndNullTotalizationDryRun(t *testing.T) {
	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			factory, err := weavegorm.NewFactory(profileForFixture(fixture))
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			condition, err := factory.New().
				AnyOf(func(group *weavegorm.Group) {
					group.EQ(nameField(), "alpha")
					group.AllOf(func(nested *weavegorm.Group) {
						nested.GT(scoreField(), int64(0))
						nested.LT(scoreField(), int64(10))
					})
				}).
				NoneOf(func(group *weavegorm.Group) {
					group.EQ(scoreField(), int64(7))
				}).
				Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			statement := usage.Traditional(fixture.database, condition).Statement
			if statement.Error != nil {
				t.Fatalf("DryRun error = %v", statement.Error)
			}
			sqlText := statement.SQL.String()
			for _, fragment := range []string{" OR ", " AND ", "NOT (", " IS NOT NULL", " = "} {
				if !strings.Contains(sqlText, fragment) {
					t.Fatalf("group SQL = %q, want fragment %q", sqlText, fragment)
				}
			}
			if strings.Contains(sqlText, " IS NULL") || strings.Contains(sqlText, " <> ") {
				t.Fatalf("whole NOT was distributed into guarded leaf: %q", sqlText)
			}
			if !reflect.DeepEqual(statement.Vars, []any{"alpha", int64(0), int64(10), int64(7)}) {
				t.Fatalf("group Vars = %#v", statement.Vars)
			}
			t.Logf("group SQL/Vars: %s | %#v", sqlText, statement.Vars)
		})
	}
}

func TestNullableInAndTraditionalGenericsEntrypoints(t *testing.T) {
	value := int64(7)
	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			factory, err := weavegorm.NewFactory(profileForFixture(fixture))
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			condition, err := factory.New().In(
				scoreField(),
				[]*int64{&value, nil},
			).Build()
			if err != nil {
				t.Fatalf("Build(nullable In) error = %v", err)
			}
			statement := usage.Traditional(fixture.database, condition).Statement
			if statement.Error != nil {
				t.Fatalf("traditional DryRun error = %v", statement.Error)
			}
			if !strings.Contains(statement.SQL.String(), " OR ") ||
				!strings.Contains(statement.SQL.String(), " IS NULL") ||
				!reflect.DeepEqual(statement.Vars, []any{value}) {
				t.Fatalf("nullable In SQL/Vars = %q / %#v", statement.SQL.String(), statement.Vars)
			}
			if _, err := usage.Generics(context.Background(), fixture.database, condition); err != nil {
				t.Fatalf("generic DryRun error = %v", err)
			}
		})
	}
}

func profileForFixture(fixture dryRunFixture) weavegorm.Profile {
	if fixture.name == "postgresql" {
		return weavegorm.PostgreSQL
	}
	return weavegorm.MySQL
}

func scoreField() weavegorm.Field[int64] {
	return weavegorm.MustQualifiedField[int64]("weave_gorm_records", "id")
}

func nameField() weavegorm.Field[string] {
	return weavegorm.MustQualifiedField[string]("weave_gorm_records", "name")
}

func assertNoColumnVars(t testing.TB, values []any) {
	t.Helper()
	for _, value := range values {
		if _, ok := value.(clause.Column); ok {
			t.Fatalf("final Vars retain clause.Column: %#v", values)
		}
	}
}
