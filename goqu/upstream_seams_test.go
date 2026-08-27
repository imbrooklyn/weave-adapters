package goqu

import (
	"reflect"
	"strings"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

var (
	_ exp.Expression           = sqlbuilder.L("1 = 1")
	_ exp.ExpressionList       = sqlbuilder.And(sqlbuilder.C("a").Eq(1))
	_ exp.IdentifierExpression = sqlbuilder.T("records").Col("name")
)

func TestUpstreamExpressionAndIdentifierSeams(t *testing.T) {
	identifier := sqlbuilder.T("records").Col("name")
	if identifier.IsEmpty() || identifier.GetSchema() != "" ||
		identifier.GetTable() != "records" || identifier.GetCol() != "name" {
		t.Fatalf(
			"identifier accessors = (schema=%q table=%q column=%#v empty=%v)",
			identifier.GetSchema(),
			identifier.GetTable(),
			identifier.GetCol(),
			identifier.IsEmpty(),
		)
	}

	ordinary := sqlbuilder.And(
		identifier.IsNotNull(),
		identifier.Eq(sqlbuilder.V("secret")),
	)
	if ordinary.Type() != exp.AndType || ordinary.IsEmpty() ||
		len(ordinary.Expressions()) != 2 {
		t.Fatalf(
			"ExpressionList = (type=%v empty=%v length=%d)",
			ordinary.Type(),
			ordinary.IsEmpty(),
			len(ordinary.Expressions()),
		)
	}
	appended := ordinary.Append(identifier.Neq(sqlbuilder.V("other")))
	if appended.Type() != exp.AndType || len(appended.Expressions()) != 3 ||
		len(ordinary.Expressions()) != 2 {
		t.Fatalf("ExpressionList.Append() did not return the expected list")
	}
	if ordinary.Expression() == nil || ordinary.Clone() == nil {
		t.Fatal("Expression interface methods returned nil")
	}
}

func TestUpstreamPreparedDialectSeams(t *testing.T) {
	identifier := sqlbuilder.T("records").Col("name")
	ordinary := sqlbuilder.And(
		identifier.IsNotNull(),
		identifier.Eq(sqlbuilder.V("secret")),
	)
	negated := sqlbuilder.L(
		"NOT (?)",
		sqlbuilder.Or(ordinary, identifier.Eq(sqlbuilder.V("other"))),
	)
	text := sqlbuilder.L(
		"(? IS NOT NULL AND ? LIKE ? ESCAPE '!')",
		identifier,
		identifier,
		"%50!%!!done!_%",
	)
	expressions := []exp.Expression{negated, text}

	tests := []struct {
		profile Profile
		wantSQL string
	}{
		{
			profile: MySQL,
			wantSQL: "SELECT * FROM `records` WHERE " +
				"(NOT ((((`records`.`name` IS NOT NULL) AND " +
				"(`records`.`name` = ?)) OR (`records`.`name` = ?))) AND " +
				"(`records`.`name` IS NOT NULL AND " +
				"`records`.`name` LIKE ? ESCAPE '!'))",
		},
		{
			profile: PostgreSQL,
			wantSQL: `SELECT * FROM "records" WHERE ` +
				`(NOT ((((` + `"records"."name" IS NOT NULL) AND ` +
				`("records"."name" = $1)) OR ("records"."name" = $2))) AND ` +
				`("records"."name" IS NOT NULL AND ` +
				`"records"."name" LIKE $3 ESCAPE '!'))`,
		},
	}

	for _, test := range tests {
		t.Run(test.profile.String(), func(t *testing.T) {
			sqlText, arguments, err := sqlbuilder.
				Dialect(test.profile.dialectName()).
				From("records").
				Where(expressions...).
				Prepared(true).
				ToSQL()
			if err != nil {
				t.Fatalf("prepared ToSQL() error = %v", err)
			}
			if sqlText != test.wantSQL {
				t.Fatalf("prepared SQL\n got: %s\nwant: %s", sqlText, test.wantSQL)
			}
			wantArguments := []any{"secret", "other", "%50!%!!done!_%"}
			if !reflect.DeepEqual(arguments, wantArguments) {
				t.Fatalf("prepared arguments = %#v, want %#v", arguments, wantArguments)
			}
		})
	}
}

func TestUpstreamValueWrapperAndValsPreserveBoundValueShape(t *testing.T) {
	active := sqlbuilder.C("active")
	payload := sqlbuilder.C("payload")
	rawBooleanSQL, rawBooleanArguments, err := sqlbuilder.
		Dialect("mysql").
		From("records").
		Where(active.Eq(true)).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawBooleanSQL, " IS TRUE") ||
		len(rawBooleanArguments) != 0 {
		t.Fatalf(
			"unwrapped upstream Boolean SQL/arguments = %q / %#v",
			rawBooleanSQL,
			rawBooleanArguments,
		)
	}

	expression := sqlbuilder.And(
		active.IsNotNull(),
		active.Eq(sqlbuilder.V(true)),
		payload.IsNotNull(),
		payload.In(exp.Vals{[]byte{0x01, 0x02}}),
	)
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		t.Run(profile.String(), func(t *testing.T) {
			sqlText, arguments, err := sqlbuilder.
				Dialect(profile.dialectName()).
				From("records").
				Where(expression).
				Prepared(true).
				ToSQL()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(sqlText, " IS TRUE") ||
				!reflect.DeepEqual(arguments, []any{true, []byte{0x01, 0x02}}) {
				t.Fatalf("bound SQL/arguments = %q / %#v", sqlText, arguments)
			}
		})
	}
}
