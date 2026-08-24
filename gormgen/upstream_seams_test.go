package gormgen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestUpstreamConditionCompositionSeam(t *testing.T) {
	fixture := newFixtureQuery(t)
	column := fixture.User.Name
	leaf := newGuardedEquality(mustColumn(t, column), "alice")

	for name, expression := range map[string]field.Expr{
		"and": field.And(leaf, column.Eq("alice")),
		"or":  field.Or(leaf, column.Eq("bob")),
		"not": field.Not(leaf),
	} {
		t.Run(name, func(t *testing.T) {
			if expression == nil {
				t.Fatal("composed expression is nil")
			}
			if err := expression.CondError(); err != nil {
				t.Fatalf("CondError() = %v, want nil", err)
			}
			var _ gen.Condition = expression
		})
	}
}

func TestGeneratedFieldMetadataSeam(t *testing.T) {
	fixture := newFixtureQuery(t)
	tests := []struct {
		name      string
		native    field.Expr
		wantTable string
		wantName  string
		wantType  reflect.Type
	}{
		{
			name:      "int64",
			native:    fixture.User.ID,
			wantTable: "weave_users",
			wantName:  "id",
			wantType:  reflect.TypeFor[int64](),
		},
		{
			name:      "string",
			native:    fixture.User.Name,
			wantTable: "weave_users",
			wantName:  "name",
			wantType:  reflect.TypeFor[string](),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			column := mustColumn(t, test.native)
			if column.Raw || column.Alias != "" ||
				column.Table != test.wantTable || column.Name != test.wantName {
				t.Fatalf("RawExpr column = %#v", column)
			}

			method, ok := reflect.TypeOf(test.native).MethodByName("Eq")
			if !ok {
				t.Fatal("generated field has no public Eq method")
			}
			if method.Type.NumIn() != 2 || method.Type.NumOut() != 1 {
				t.Fatalf("Eq signature = %v", method.Type)
			}
			if got := method.Type.In(1); got != test.wantType {
				t.Fatalf("Eq value type = %v, want %v", got, test.wantType)
			}
			if got := method.Type.Out(0); got != reflect.TypeFor[field.Expr]() {
				t.Fatalf("Eq result type = %v, want field.Expr", got)
			}
		})
	}

	if _, ok := any(fixture.User.Name.Desc().RawExpr()).(clause.Column); ok {
		t.Fatal("derived expression unexpectedly exposes a pure column RawExpr")
	}
}

func TestFixedTemplatesKeepValuesOutOfSQL(t *testing.T) {
	fixture := newFixtureQuery(t)
	column := mustColumn(t, fixture.User.Name)
	value := "alice' OR 1=1 -- 50%_!"
	tests := []struct {
		name       string
		template   string
		expression field.Expr
		boundValue string
	}{
		{
			name:       "equality",
			template:   guardedEqualityTemplate,
			expression: newGuardedEquality(column, value),
			boundValue: value,
		},
		{
			name:       "literal text",
			template:   literalLikeTemplate,
			expression: newLiteralLike(column, "%alice!%"),
			boundValue: "%alice!%",
		},
	}

	for _, test := range tests {
		t.Run(test.name+" raw expression", func(t *testing.T) {
			raw, ok := any(test.expression.RawExpr()).(clause.Expr)
			if !ok {
				t.Fatalf("RawExpr() type = %T, want clause.Expr", test.expression.RawExpr())
			}
			if raw.SQL != test.template {
				t.Fatalf("RawExpr SQL = %q, want fixed template %q", raw.SQL, test.template)
			}
			if strings.Contains(raw.SQL, test.boundValue) ||
				strings.Contains(raw.SQL, column.Table) ||
				strings.Contains(raw.SQL, column.Name) {
				t.Fatalf("fixed template contains field or query value: %q", raw.SQL)
			}
			if len(raw.Vars) != 3 {
				t.Fatalf("Vars length = %d, want 3", len(raw.Vars))
			}
			for _, index := range []int{0, 1} {
				if got, ok := raw.Vars[index].(clause.Column); !ok || got != column {
					t.Fatalf("Vars[%d] = %#v, want column %#v", index, raw.Vars[index], column)
				}
			}
			if got := raw.Vars[2]; got != test.boundValue {
				t.Fatalf("Vars[2] = %#v, want bound value", got)
			}
		})
	}

	for name, dialector := range map[string]gorm.Dialector{
		"mysql": mysql.New(mysql.Config{
			DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
			SkipInitializeWithVersion: true,
		}),
		"postgresql": postgres.New(postgres.Config{
			DSN:                  "host=127.0.0.1 port=9911 user=gorm dbname=gorm sslmode=disable",
			PreferSimpleProtocol: true,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			database, err := gorm.Open(dialector, &gorm.Config{
				DisableAutomaticPing: true,
				DryRun:               true,
			})
			if err != nil {
				t.Fatalf("gorm.Open() error = %v", err)
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var users []model.User
					statement := database.Where(test.expression).Find(&users).Statement
					if statement.Error != nil {
						t.Fatalf("DryRun build error = %v", statement.Error)
					}
					if strings.Contains(statement.SQL.String(), test.boundValue) {
						t.Fatalf("DryRun SQL contains query value: %q", statement.SQL.String())
					}
					if len(statement.Vars) != 1 || statement.Vars[0] != test.boundValue {
						t.Fatalf("DryRun Vars = %#v, want bound query value", statement.Vars)
					}
				})
			}
		})
	}
}

func newFixtureQuery(t testing.TB) *query.Query {
	t.Helper()
	database, err := gorm.Open(
		mysql.New(mysql.Config{
			DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
			SkipInitializeWithVersion: true,
		}),
		&gorm.Config{
			DisableAutomaticPing: true,
			DryRun:               true,
		},
	)
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return query.Use(database)
}

func mustColumn(t testing.TB, expression field.Expr) clause.Column {
	t.Helper()
	column, ok := any(expression.RawExpr()).(clause.Column)
	if !ok {
		t.Fatalf("RawExpr() type = %T, want clause.Column", expression.RawExpr())
	}
	return column
}
