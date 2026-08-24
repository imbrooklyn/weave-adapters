package gormgen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gen/field"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestEveryStandardOperatorUsesFixedTemplateAndBoundVars(t *testing.T) {
	fixture := newFixtureQuery(t)
	idColumn := mustColumn(t, fixture.User.ID)
	nameColumn := mustColumn(t, fixture.User.Name)
	text := "literal '%_!' OR 1=1 -- \u4e16\u754c\\end"

	tests := []struct {
		name         string
		build        func(*weave.Builder[Conditions, Expression])
		wantTemplate string
		wantVars     []any
	}{
		{
			name: "eq",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.EQ(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedEqualityTemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "neq",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.NEQ(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedInequalityTemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "lt",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.LT(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedLTTemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "lte",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.LTE(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedLTETemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "gt",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.GT(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedGTTemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "gte",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.GTE(fixture.User.ID, int64(7))
			},
			wantTemplate: guardedGTETemplate,
			wantVars:     []any{idColumn, idColumn, int64(7)},
		},
		{
			name: "in",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.In(fixture.User.ID, []int64{7, 11})
			},
			wantTemplate: "(? IS NOT NULL AND ? IN (?, ?))",
			wantVars:     []any{idColumn, idColumn, int64(7), int64(11)},
		},
		{
			name: "not in",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.NotIn(fixture.User.ID, []int64{7, 11})
			},
			wantTemplate: "(? IS NOT NULL AND ? NOT IN (?, ?))",
			wantVars:     []any{idColumn, idColumn, int64(7), int64(11)},
		},
		{
			name: "between",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Between(fixture.User.ID, int64(7), int64(11))
			},
			wantTemplate: guardedBetweenTemplate,
			wantVars:     []any{idColumn, idColumn, int64(7), int64(11)},
		},
		{
			name: "is null",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.IsNull(fixture.User.Name)
			},
			wantTemplate: isNullTemplate,
			wantVars:     []any{nameColumn},
		},
		{
			name: "not null",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.NotNull(fixture.User.Name)
			},
			wantTemplate: notNullTemplate,
			wantVars:     []any{nameColumn},
		},
		{
			name: "contains",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Contains(fixture.User.Name, text)
			},
			wantTemplate: literalLikeTemplate,
			wantVars:     []any{nameColumn, nameColumn, "%literal '!%!_!!' OR 1=1 -- \u4e16\u754c\\end%"},
		},
		{
			name: "prefix",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.HasPrefix(fixture.User.Name, text)
			},
			wantTemplate: literalLikeTemplate,
			wantVars:     []any{nameColumn, nameColumn, "literal '!%!_!!' OR 1=1 -- \u4e16\u754c\\end%"},
		},
		{
			name: "suffix",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.HasSuffix(fixture.User.Name, text)
			},
			wantTemplate: literalLikeTemplate,
			wantVars:     []any{nameColumn, nameColumn, "%literal '!%!_!!' OR 1=1 -- \u4e16\u754c\\end"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := NewFactory(MySQL)
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			builder := factory.New()
			test.build(builder)
			conditions, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if len(conditions) != 1 {
				t.Fatalf("condition count = %d, want 1", len(conditions))
			}
			expression, ok := conditions[0].(field.Expr)
			if !ok {
				t.Fatalf("condition type = %T, want field.Expr", conditions[0])
			}
			raw, ok := any(expression.RawExpr()).(clause.Expr)
			if !ok {
				t.Fatalf("RawExpr type = %T, want clause.Expr", expression.RawExpr())
			}
			if raw.SQL != test.wantTemplate {
				t.Fatalf("SQL template = %q, want %q", raw.SQL, test.wantTemplate)
			}
			if !reflect.DeepEqual(raw.Vars, test.wantVars) {
				t.Fatalf("Vars = %#v, want %#v", raw.Vars, test.wantVars)
			}
			if strings.Contains(raw.SQL, text) ||
				strings.Contains(raw.SQL, idColumn.Table) ||
				strings.Contains(raw.SQL, idColumn.Name) ||
				strings.Contains(raw.SQL, nameColumn.Name) {
				t.Fatalf("template contains field or query text: %q", raw.SQL)
			}
		})
	}
}

func TestGeneratedDAOWhereKeepsOrdinaryValuesBound(t *testing.T) {
	queryValue := "x' OR 1=1 -- %_! \u4e16\u754c"
	for _, test := range []struct {
		name      string
		profile   Profile
		dialector gorm.Dialector
	}{
		{
			name:    "mysql",
			profile: MySQL,
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
				SkipInitializeWithVersion: true,
			}),
		},
		{
			name:    "postgresql",
			profile: PostgreSQL,
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=127.0.0.1 port=9911 user=gorm dbname=gorm sslmode=disable",
				PreferSimpleProtocol: true,
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, err := gorm.Open(test.dialector, &gorm.Config{
				DisableAutomaticPing: true,
				DryRun:               true,
			})
			if err != nil {
				t.Fatalf("gorm.Open() error = %v", err)
			}
			queries := query.Use(database)
			factory, err := NewFactory(test.profile)
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			conditions, err := factory.New().
				EQ(queries.User.Name, queryValue).
				In(queries.User.ID, []int64{7, 11}).
				Contains(queries.User.Name, queryValue).
				Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			var users []*model.User
			statement := queries.User.
				Where(conditions...).
				UnderlyingDB().
				Find(&users).
				Statement
			if statement.Error != nil {
				t.Fatalf("generated DAO DryRun error = %v", statement.Error)
			}
			if strings.Contains(statement.SQL.String(), queryValue) {
				t.Fatalf("generated DAO SQL contains query value: %q", statement.SQL.String())
			}
			wantPattern := "%x' OR 1=1 -- !%!_!! \u4e16\u754c%"
			wantVars := []any{queryValue, int64(7), int64(11), wantPattern}
			if !reflect.DeepEqual(statement.Vars, wantVars) {
				t.Fatalf("generated DAO Vars = %#v, want %#v", statement.Vars, wantVars)
			}
			if !strings.Contains(statement.SQL.String(), "ESCAPE '!'") {
				t.Fatalf("generated DAO SQL has no fixed ESCAPE clause: %q", statement.SQL.String())
			}
		})
	}
}
