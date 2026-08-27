package goqu

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

var numberedPlaceholder = regexp.MustCompile(`\$[0-9]+`)

func TestEveryStandardOperatorKeepsPreparedValuesBound(t *testing.T) {
	const queryText = "x' OR 1=1 -- %_! 世界\\end"
	number := mustField(t, sqlbuilder.T("records").Col("number_value"), int64(0))
	text := mustField(t, sqlbuilder.T("records").Col("text_value"), "")

	tests := []struct {
		name         string
		build        func(*weave.Builder[Expressions, Expression])
		wantFragment string
		wantArgs     []any
		wantGuard    bool
	}{
		{
			name: "eq",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.EQ(number, int64(7))
			},
			wantFragment: " = ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "neq",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.NEQ(number, int64(7))
			},
			wantFragment: " != ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "lt",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.LT(number, int64(7))
			},
			wantFragment: " < ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "lte",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.LTE(number, int64(7))
			},
			wantFragment: " <= ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "gt",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.GT(number, int64(7))
			},
			wantFragment: " > ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "gte",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.GTE(number, int64(7))
			},
			wantFragment: " >= ",
			wantArgs:     []any{int64(7)},
			wantGuard:    true,
		},
		{
			name: "in",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.In(number, []int64{7, 11})
			},
			wantFragment: " IN (",
			wantArgs:     []any{int64(7), int64(11)},
			wantGuard:    true,
		},
		{
			name: "not in",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.NotIn(number, []int64{7, 11})
			},
			wantFragment: " NOT IN (",
			wantArgs:     []any{int64(7), int64(11)},
			wantGuard:    true,
		},
		{
			name: "between",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Between(number, int64(7), int64(11))
			},
			wantFragment: " >= ",
			wantArgs:     []any{int64(7), int64(11)},
			wantGuard:    true,
		},
		{
			name: "is null",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.IsNull(text)
			},
			wantFragment: " IS NULL",
			wantArgs:     []any{},
		},
		{
			name: "not null",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.NotNull(text)
			},
			wantFragment: " IS NOT NULL",
			wantArgs:     []any{},
		},
		{
			name: "contains",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Contains(text, queryText)
			},
			wantFragment: " LIKE ",
			wantArgs:     []any{"%x' OR 1=1 -- !%!_!! 世界\\end%"},
			wantGuard:    true,
		},
		{
			name: "has prefix",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.HasPrefix(text, queryText)
			},
			wantFragment: " LIKE ",
			wantArgs:     []any{"x' OR 1=1 -- !%!_!! 世界\\end%"},
			wantGuard:    true,
		},
		{
			name: "has suffix",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.HasSuffix(text, queryText)
			},
			wantFragment: " LIKE ",
			wantArgs:     []any{"%x' OR 1=1 -- !%!_!! 世界\\end"},
			wantGuard:    true,
		},
	}

	for _, profile := range []Profile{MySQL, PostgreSQL} {
		t.Run(profile.String(), func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					expressions, sqlText, arguments := buildPrepared(
						t,
						profile,
						test.build,
					)
					if len(expressions) != 1 {
						t.Fatalf("compiled expression count = %d, want 1", len(expressions))
					}
					if !strings.Contains(sqlText, test.wantFragment) {
						t.Fatalf("SQL = %q, want fragment %q", sqlText, test.wantFragment)
					}
					if test.wantGuard && !strings.Contains(sqlText, " IS NOT NULL") {
						t.Fatalf("ordinary leaf lacks a non-NULL guard: %q", sqlText)
					}
					if strings.Contains(sqlText, queryText) {
						t.Fatalf("prepared SQL contains query text: %q", sqlText)
					}
					assertArguments(t, arguments, test.wantArgs)
					assertPreparedShape(t, profile, sqlText, arguments)
					assertNoExpressionArguments(t, arguments)
				})
			}
		})
	}
}

func TestPreparedBooleanAndByteSliceValuesStayBound(t *testing.T) {
	active := mustField(t, sqlbuilder.T("records").Col("active"), false)
	payload := mustField(t, sqlbuilder.T("records").Col("payload"), []byte(nil))
	bytesValue := []byte{0x00, 0x01, 0xff}

	for _, profile := range []Profile{MySQL, PostgreSQL} {
		t.Run(profile.String(), func(t *testing.T) {
			_, sqlText, arguments := buildPrepared(
				t,
				profile,
				func(builder *weave.Builder[Expressions, Expression]) {
					builder.EQ(active, true)
					builder.In(payload, [][]byte{bytesValue})
				},
			)
			if strings.Contains(sqlText, " IS TRUE") ||
				strings.Contains(sqlText, " IS NOT TRUE") {
				t.Fatalf("Boolean comparison was rewritten as a SQL literal: %q", sqlText)
			}
			assertArguments(t, arguments, []any{true, bytesValue})
			assertPreparedShape(t, profile, sqlText, arguments)
		})
	}
}

func TestPreparedStringPayloadNeverEntersSQL(t *testing.T) {
	const payload = "secret' OR 1=1 --"
	field := mustField(t, sqlbuilder.T("records").Col("text_value"), "")
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		t.Run(profile.String(), func(t *testing.T) {
			_, sqlText, arguments := buildPrepared(
				t,
				profile,
				func(builder *weave.Builder[Expressions, Expression]) {
					builder.EQ(field, payload)
					builder.In(field, []string{payload, "second-secret"})
				},
			)
			for _, secret := range []string{payload, "second-secret"} {
				if strings.Contains(sqlText, secret) {
					t.Fatalf("prepared SQL contains query payload %q: %q", secret, sqlText)
				}
			}
			assertArguments(t, arguments, []any{payload, payload, "second-secret"})
			assertPreparedShape(t, profile, sqlText, arguments)
		})
	}
}

func TestFourLogicFormsConstantsAndRootExpressionOrder(t *testing.T) {
	number := mustField(t, sqlbuilder.T("records").Col("number_value"), int64(0))
	factory, err := NewFactory(PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	expressions, err := factory.New().
		AllOf(func(group *Group) {
			group.EQ(number, int64(1))
			group.GT(number, int64(0))
		}).
		AnyOf(func(group *Group) {
			group.EQ(number, int64(2))
			group.NotIn(number, []int64{})
		}).
		NoneOf(func(group *Group) {
			group.EQ(number, int64(3))
		}).
		NotAllOf(func(group *Group) {
			group.EQ(number, int64(4))
			group.In(number, []int64{})
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(expressions) != 4 {
		t.Fatalf("root expression count = %d, want 4", len(expressions))
	}

	sqlText, arguments, err := sqlbuilder.
		Dialect(PostgreSQL.dialectName()).
		From("records").
		Where(expressions...).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		" AND ",
		" OR ",
		"NOT (",
		trueTemplate,
		falseTemplate,
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("logic SQL = %q, want fragment %q", sqlText, fragment)
		}
	}
	assertArguments(t, arguments, []any{int64(1), int64(0), int64(2), int64(3), int64(4)})
	assertPreparedShape(t, PostgreSQL, sqlText, arguments)

	empty, err := factory.New().Build()
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty root Build() = (%#v, %v), want owned empty Expressions", empty, err)
	}
}

func TestNativeAndExprPreserveRootOrderWithoutBooleanWrapper(t *testing.T) {
	nativeInput := ExpressionsOf(
		sqlbuilder.L("native_one = ?", "one"),
		sqlbuilder.L("native_two = ?", "two"),
	)
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatal(err)
	}
	builder := factory.New().
		Native(nativeInput).
		Expr(sqlbuilder.L("expr_one = ?", "three")).
		AnyOf(func(group *Group) {
			group.Expr(sqlbuilder.L("expr_two = ?", "four"))
		})
	nativeInput[0] = sqlbuilder.L("mutated = 1")

	expressions, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(expressions) != 4 {
		t.Fatalf("root expression count = %d, want 4", len(expressions))
	}
	sqlText, arguments, err := sqlbuilder.
		Dialect(MySQL.dialectName()).
		From("records").
		Where(expressions...).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"native_one = ?",
		"native_two = ?",
		"expr_one = ?",
		"expr_two = ?",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("Native/Expr SQL = %q, want fragment %q", sqlText, fragment)
		}
	}
	if strings.Contains(sqlText, "mutated") {
		t.Fatalf("Native retained caller top-level backing storage: %q", sqlText)
	}
	assertArguments(t, arguments, []any{"one", "two", "three", "four"})
}

func buildPrepared(
	t testing.TB,
	profile Profile,
	build func(*weave.Builder[Expressions, Expression]),
) (Expressions, string, []any) {
	t.Helper()
	factory, err := NewFactory(profile)
	if err != nil {
		t.Fatalf("NewFactory(%s) error = %v", profile, err)
	}
	builder := factory.New()
	build(builder)
	expressions, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if expressions == nil {
		t.Fatal("Build() returned nil Expressions on success")
	}
	sqlText, arguments, err := sqlbuilder.
		Dialect(profile.dialectName()).
		From("records").
		Where(expressions...).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatalf("prepared ToSQL() error = %v", err)
	}
	return expressions, sqlText, arguments
}

func assertPreparedShape(
	t testing.TB,
	profile Profile,
	sqlText string,
	arguments []any,
) {
	t.Helper()
	placeholderCount := strings.Count(sqlText, "?")
	if profile == PostgreSQL {
		placeholderCount = len(numberedPlaceholder.FindAllString(sqlText, -1))
		if strings.Contains(sqlText, "?") {
			t.Fatalf("PostgreSQL prepared SQL retained an unnumbered placeholder: %q", sqlText)
		}
	}
	if placeholderCount != len(arguments) {
		t.Fatalf(
			"prepared SQL has %d placeholders and %d arguments: %q / %#v",
			placeholderCount,
			len(arguments),
			sqlText,
			arguments,
		)
	}
}

func assertArguments(t testing.TB, got, want []any) {
	t.Helper()
	if len(got) != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared arguments = %#v, want %#v", got, want)
	}
}

func assertNoExpressionArguments(t testing.TB, arguments []any) {
	t.Helper()
	for _, argument := range arguments {
		if _, ok := argument.(exp.Expression); ok {
			t.Fatalf("prepared arguments retain an expression: %#v", arguments)
		}
		if _, ok := argument.(exp.IdentifierExpression); ok {
			t.Fatalf("prepared arguments retain an identifier: %#v", arguments)
		}
	}
}
