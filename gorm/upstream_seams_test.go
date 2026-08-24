package gorm_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	weavegorm "github.com/imbrooklyn/weave-adapters/gorm"
	"github.com/imbrooklyn/weave-adapters/gorm/internal/fixture/usage"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	upstream "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	_ weavegorm.Condition  = clause.Eq{}
	_ weavegorm.Expression = clause.Expr{}
)

func TestClauseCompositionTypesAndWhereEntrypoints(t *testing.T) {
	column := clause.Column{Table: "weave_gorm_records", Name: "name"}
	leaves := []clause.Expression{
		clause.Eq{Column: column, Value: "alice"},
		clause.Neq{Column: column, Value: "bob"},
		clause.Lt{Column: column, Value: "m"},
	}
	andExpression := clause.And(leaves[0], leaves[1])
	orExpression := clause.Or(andExpression, leaves[2])
	notExpression := clause.Not(orExpression)

	if got := reflect.TypeOf(andExpression); got != reflect.TypeFor[clause.AndConditions]() {
		t.Fatalf("clause.And type = %v, want clause.AndConditions", got)
	}
	if got := reflect.TypeOf(orExpression); got != reflect.TypeFor[clause.OrConditions]() {
		t.Fatalf("clause.Or type = %v, want clause.OrConditions", got)
	}
	if got := reflect.TypeOf(notExpression); got != reflect.TypeFor[clause.NotConditions]() {
		t.Fatalf("clause.Not type = %v, want clause.NotConditions", got)
	}
	if got := clause.And(leaves[0]); reflect.TypeOf(got) != reflect.TypeFor[clause.Eq]() {
		t.Fatalf("unary clause.And type = %T, want clause.Eq passthrough", got)
	}
	if got := clause.Or(leaves[0]); reflect.TypeOf(got) != reflect.TypeFor[clause.OrConditions]() {
		t.Fatalf("unary clause.Or type = %T, want clause.OrConditions", got)
	}
	if clause.And() != nil || clause.Or() != nil || clause.Not() != nil {
		t.Fatal("zero-argument clause composition did not return nil")
	}

	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			traditional := usage.Traditional(fixture.database, notExpression)
			if traditional.Error != nil {
				t.Fatalf("traditional DB.Where build error = %v", traditional.Error)
			}
			if _, err := usage.Generics(
				context.Background(),
				fixture.database,
				notExpression,
			); err != nil {
				t.Fatalf("generic Where build error = %v", err)
			}
			profile := weavegorm.MySQL
			if fixture.name == "postgresql" {
				profile = weavegorm.PostgreSQL
			}
			if result, err := usage.CompiledTraditional(
				fixture.database,
				profile,
			); err != nil || result.Error != nil {
				t.Fatalf(
					"compiled traditional Where build = (%v, %v)",
					err,
					result.Error,
				)
			}
			if _, err := usage.CompiledGenerics(
				context.Background(),
				fixture.database,
				profile,
			); err != nil {
				t.Fatalf("compiled generic Where build error = %v", err)
			}
		})
	}
}

func TestWholeExpressionNegationRequiresIdentityWrapper(t *testing.T) {
	column := clause.Column{Table: "weave_gorm_records", Name: "name"}
	guarded := clause.And(
		clause.Neq{Column: column, Value: nil},
		clause.Eq{Column: column, Value: "alice"},
	)

	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			direct := usage.Traditional(
				fixture.database,
				clause.Not(guarded),
			).Statement
			safe := usage.Traditional(
				fixture.database,
				clause.Not(clause.Or(guarded)),
			).Statement
			if direct.Error != nil || safe.Error != nil {
				t.Fatalf("NOT build errors = (%v, %v)", direct.Error, safe.Error)
			}
			if strings.Contains(direct.SQL.String(), "NOT (") ||
				!strings.Contains(direct.SQL.String(), " IS NULL") ||
				!strings.Contains(direct.SQL.String(), " <> ") {
				t.Fatalf("direct Not(And(...)) SQL = %q", direct.SQL.String())
			}
			if !strings.Contains(safe.SQL.String(), "NOT (") ||
				!strings.Contains(safe.SQL.String(), " IS NOT NULL") ||
				!strings.Contains(safe.SQL.String(), " = ") {
				t.Fatalf("identity-wrapped Not(And(...)) SQL = %q", safe.SQL.String())
			}
			if !reflect.DeepEqual(direct.Vars, []any{"alice"}) ||
				!reflect.DeepEqual(safe.Vars, []any{"alice"}) {
				t.Fatalf("NOT Vars = (%#v, %#v)", direct.Vars, safe.Vars)
			}
			t.Logf("direct Not(And(...)) SQL/Vars: %s | %#v", direct.SQL.String(), direct.Vars)
			t.Logf("safe Not(Or(And(...))) SQL/Vars: %s | %#v", safe.SQL.String(), safe.Vars)
		})
	}
}

func TestColumnAndFixedLikeTemplateDryRun(t *testing.T) {
	column := clause.Column{Table: "weave_gorm_records", Name: "name"}
	pattern := "%50!%!_done%"
	like := clause.Expr{
		SQL:  "? LIKE ? ESCAPE '!'",
		Vars: []any{column, pattern},
	}

	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			quoted := usage.Traditional(
				fixture.database,
				clause.Eq{Column: column, Value: "needle"},
			).Statement
			assertQuotedColumn(t, fixture, quoted.SQL.String())
			if !reflect.DeepEqual(quoted.Vars, []any{"needle"}) {
				t.Fatalf("quoted column Vars = %#v", quoted.Vars)
			}

			statement := usage.Traditional(fixture.database, like).Statement
			if statement.Error != nil {
				t.Fatalf("LIKE build error = %v", statement.Error)
			}
			assertQuotedColumn(t, fixture, statement.SQL.String())
			if !strings.Contains(statement.SQL.String(), " LIKE "+fixture.placeholder+" ESCAPE '!'") {
				t.Fatalf("LIKE SQL = %q", statement.SQL.String())
			}
			if strings.Contains(statement.SQL.String(), pattern) ||
				!reflect.DeepEqual(statement.Vars, []any{pattern}) {
				t.Fatalf("LIKE SQL/Vars = %q / %#v", statement.SQL.String(), statement.Vars)
			}
			t.Logf("LIKE SQL/Vars: %s | %#v", statement.SQL.String(), statement.Vars)
		})
	}
}

func TestPublicComparisonMembershipAndNullExpressions(t *testing.T) {
	column := clause.Column{Table: "weave_gorm_records", Name: "name"}
	tests := []struct {
		name       string
		expression clause.Expression
		fragment   string
		wantVars   []any
	}{
		{name: "eq", expression: clause.Eq{Column: column, Value: "a"}, fragment: " = ", wantVars: []any{"a"}},
		{name: "neq", expression: clause.Neq{Column: column, Value: "a"}, fragment: " <> ", wantVars: []any{"a"}},
		{name: "lt", expression: clause.Lt{Column: column, Value: "a"}, fragment: " < ", wantVars: []any{"a"}},
		{name: "lte", expression: clause.Lte{Column: column, Value: "a"}, fragment: " <= ", wantVars: []any{"a"}},
		{name: "gt", expression: clause.Gt{Column: column, Value: "a"}, fragment: " > ", wantVars: []any{"a"}},
		{name: "gte", expression: clause.Gte{Column: column, Value: "a"}, fragment: " >= ", wantVars: []any{"a"}},
		{name: "in", expression: clause.IN{Column: column, Values: []any{"a", "b"}}, fragment: " IN (", wantVars: []any{"a", "b"}},
		{name: "is null", expression: clause.Eq{Column: column, Value: nil}, fragment: " IS NULL", wantVars: []any{}},
		{name: "not null", expression: clause.Neq{Column: column, Value: nil}, fragment: " IS NOT NULL", wantVars: []any{}},
	}

	for _, fixture := range dryRunFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					statement := usage.Traditional(fixture.database, test.expression).Statement
					if statement.Error != nil {
						t.Fatalf("DryRun build error = %v", statement.Error)
					}
					if !strings.Contains(statement.SQL.String(), test.fragment) {
						t.Fatalf("SQL = %q, want fragment %q", statement.SQL.String(), test.fragment)
					}
					if !reflect.DeepEqual(statement.Vars, test.wantVars) {
						t.Fatalf("Vars = %#v, want %#v", statement.Vars, test.wantVars)
					}
				})
			}
		})
	}
}

type dryRunFixture struct {
	name        string
	database    *upstream.DB
	quoted      string
	placeholder string
}

func dryRunFixtures(t testing.TB) []dryRunFixture {
	t.Helper()
	definitions := []struct {
		name        string
		dialector   upstream.Dialector
		quoted      string
		placeholder string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
				SkipInitializeWithVersion: true,
			}),
			quoted:      "`weave_gorm_records`.`name`",
			placeholder: "?",
		},
		{
			name: "postgresql",
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=127.0.0.1 port=9911 user=gorm dbname=gorm sslmode=disable",
				PreferSimpleProtocol: true,
			}),
			quoted:      `"weave_gorm_records"."name"`,
			placeholder: "$1",
		},
	}

	fixtures := make([]dryRunFixture, 0, len(definitions))
	for _, definition := range definitions {
		database, err := upstream.Open(definition.dialector, &upstream.Config{
			DisableAutomaticPing: true,
			DryRun:               true,
		})
		if err != nil {
			t.Fatalf("gorm.Open(%s) error = %v", definition.name, err)
		}
		fixtures = append(fixtures, dryRunFixture{
			name:        definition.name,
			database:    database,
			quoted:      definition.quoted,
			placeholder: definition.placeholder,
		})
	}
	return fixtures
}

func assertQuotedColumn(t testing.TB, fixture dryRunFixture, sql string) {
	t.Helper()
	if !strings.Contains(sql, fixture.quoted) {
		t.Fatalf("SQL = %q, want quoted column %q", sql, fixture.quoted)
	}
}
