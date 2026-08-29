package gormgen

import (
	"reflect"
	"testing"

	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gen/field"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func FuzzGeneratedDAOBindsLiteralText(f *testing.F) {
	type configuration struct {
		factory     *Factory
		database    *gorm.DB
		baselineSQL string
	}
	configurations := make([]configuration, 0, 2)
	for _, setup := range []struct {
		profile   Profile
		dialector gorm.Dialector
	}{
		{
			profile: MySQL,
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
				SkipInitializeWithVersion: true,
			}),
		},
		{
			profile: PostgreSQL,
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=127.0.0.1 port=9911 user=gorm dbname=gorm sslmode=disable",
				PreferSimpleProtocol: true,
			}),
		},
	} {
		database, err := gorm.Open(setup.dialector, &gorm.Config{
			DisableAutomaticPing: true,
			DryRun:               true,
		})
		if err != nil {
			f.Fatal(err)
		}
		factory, err := NewFactory(setup.profile)
		if err != nil {
			f.Fatal(err)
		}
		queries := query.Use(database)
		conditions, err := factory.New().
			Contains(queries.SemanticRecord.Text, "").
			Build()
		if err != nil {
			f.Fatal(err)
		}
		statement := fuzzGeneratedDAOStatement(queries, conditions)
		if statement.Error != nil {
			f.Fatal(statement.Error)
		}
		configurations = append(configurations, configuration{
			factory:     factory,
			database:    database,
			baselineSQL: statement.SQL.String(),
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
		queries := query.Use(configuration.database)
		conditions, err := configuration.factory.New().
			Contains(queries.SemanticRecord.Text, value).
			Build()
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
		column := mustColumn(t, queries.SemanticRecord.Text)
		wantPattern := "%" + literalLikeEscaper.Replace(value) + "%"
		if raw.SQL != literalLikeTemplate {
			t.Fatalf("SQL template = %q, want %q", raw.SQL, literalLikeTemplate)
		}
		if want := []any{column, column, wantPattern}; !reflect.DeepEqual(raw.Vars, want) {
			t.Fatalf("raw Vars = %#v, want %#v", raw.Vars, want)
		}

		statement := fuzzGeneratedDAOStatement(queries, conditions)
		if statement.Error != nil {
			t.Fatalf("generated DAO DryRun error = %v", statement.Error)
		}
		if statement.SQL.String() != configuration.baselineSQL {
			t.Fatalf("generated DAO SQL changed with the bound value: %q", statement.SQL.String())
		}
		if want := []any{wantPattern}; !reflect.DeepEqual(statement.Vars, want) {
			t.Fatalf("generated DAO Vars = %#v, want %#v", statement.Vars, want)
		}
	})
}

func fuzzGeneratedDAOStatement(
	queries *query.Query,
	conditions Conditions,
) *gorm.Statement {
	rows := make([]model.SemanticRecord, 0)
	return queries.SemanticRecord.
		Where(conditions...).
		UnderlyingDB().
		Find(&rows).
		Statement
}
