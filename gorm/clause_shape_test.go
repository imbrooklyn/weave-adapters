package gorm

import (
	"reflect"
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

func TestComparisonClauseShapesAreGuarded(t *testing.T) {
	field := MustQualifiedField[int64]("users", "score")
	wantColumn := clause.Column{Table: "users", Name: "score"}
	tests := []struct {
		name      string
		build     func(*weave.Builder[Condition, Expression])
		operation any
	}{
		{name: "eq", build: func(builder *weave.Builder[Condition, Expression]) { builder.EQ(field, int64(7)) }, operation: clause.Eq{}},
		{name: "neq", build: func(builder *weave.Builder[Condition, Expression]) { builder.NEQ(field, int64(7)) }, operation: clause.Neq{}},
		{name: "lt", build: func(builder *weave.Builder[Condition, Expression]) { builder.LT(field, int64(7)) }, operation: clause.Lt{}},
		{name: "lte", build: func(builder *weave.Builder[Condition, Expression]) { builder.LTE(field, int64(7)) }, operation: clause.Lte{}},
		{name: "gt", build: func(builder *weave.Builder[Condition, Expression]) { builder.GT(field, int64(7)) }, operation: clause.Gt{}},
		{name: "gte", build: func(builder *weave.Builder[Condition, Expression]) { builder.GTE(field, int64(7)) }, operation: clause.Gte{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			condition := buildCondition(t, test.build)
			guarded := assertGuarded(t, condition, wantColumn)
			if reflect.TypeOf(guarded.Exprs[1]) != reflect.TypeOf(test.operation) {
				t.Fatalf("operation type = %T, want %T", guarded.Exprs[1], test.operation)
			}
		})
	}
}

func TestMembershipBetweenNullAndTextClauseShapes(t *testing.T) {
	id := MustQualifiedField[int64]("users", "id")
	name := MustQualifiedField[string]("users", "name")
	idColumn := clause.Column{Table: "users", Name: "id"}
	nameColumn := clause.Column{Table: "users", Name: "name"}

	in := assertGuarded(t, buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.In(id, []int64{7, 11})
	}), idColumn)
	inOperation, ok := in.Exprs[1].(clause.IN)
	if !ok || !reflect.DeepEqual(inOperation.Values, []any{int64(7), int64(11)}) {
		t.Fatalf("In operation = %#v", in.Exprs[1])
	}

	notIn := assertGuarded(t, buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.NotIn(id, []int64{7, 11})
	}), idColumn)
	negated, ok := notIn.Exprs[1].(clause.NotConditions)
	if !ok || len(negated.Exprs) != 1 {
		t.Fatalf("NotIn operation = %#v", notIn.Exprs[1])
	}
	if _, ok := negated.Exprs[0].(clause.IN); !ok {
		t.Fatalf("NotIn child type = %T, want clause.IN", negated.Exprs[0])
	}

	between := assertGuarded(t, buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.Between(id, int64(7), int64(11))
	}), idColumn)
	rangeExpression, ok := between.Exprs[1].(clause.AndConditions)
	if !ok || len(rangeExpression.Exprs) != 2 {
		t.Fatalf("Between operation = %#v", between.Exprs[1])
	}
	if _, ok := rangeExpression.Exprs[0].(clause.Gte); !ok {
		t.Fatalf("Between lower type = %T, want clause.Gte", rangeExpression.Exprs[0])
	}
	if _, ok := rangeExpression.Exprs[1].(clause.Lte); !ok {
		t.Fatalf("Between upper type = %T, want clause.Lte", rangeExpression.Exprs[1])
	}

	isNull := buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.IsNull(name)
	})
	isNullExpression, ok := isNull.(clause.Eq)
	if !ok || isNullExpression.Column != nameColumn || isNullExpression.Value != nil {
		t.Fatalf("IsNull expression = %#v", isNull)
	}
	notNull := buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.NotNull(name)
	})
	notNullExpression, ok := notNull.(clause.Neq)
	if !ok || notNullExpression.Column != nameColumn || notNullExpression.Value != nil {
		t.Fatalf("NotNull expression = %#v", notNull)
	}

	text := assertGuarded(t, buildCondition(t, func(builder *weave.Builder[Condition, Expression]) {
		builder.Contains(name, "50%_!")
	}), nameColumn)
	like, ok := text.Exprs[1].(clause.Expr)
	if !ok || like.SQL != literalLikeTemplate || !reflect.DeepEqual(
		like.Vars,
		[]any{nameColumn, "%50!%!_!!%"},
	) {
		t.Fatalf("literal text expression = %#v", text.Exprs[1])
	}
}

func TestGroupClauseShapesPreserveWholeExpressionNegation(t *testing.T) {
	id := MustQualifiedField[int64]("users", "id")
	tests := []struct {
		name  string
		build func(*weave.Builder[Condition, Expression])
		kind  any
	}{
		{
			name: "all of",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.AllOf(func(group *Group) {
					group.EQ(id, int64(1))
					group.EQ(id, int64(2))
				})
			},
			kind: clause.AndConditions{},
		},
		{
			name: "any of",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.AnyOf(func(group *Group) {
					group.EQ(id, int64(1))
					group.EQ(id, int64(2))
				})
			},
			kind: clause.AndConditions{},
		},
		{
			name: "none of",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.NoneOf(func(group *Group) {
					group.EQ(id, int64(1))
					group.EQ(id, int64(2))
				})
			},
			kind: clause.NotConditions{},
		},
		{
			name: "not all of",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.NotAllOf(func(group *Group) {
					group.EQ(id, int64(1))
					group.EQ(id, int64(2))
				})
			},
			kind: clause.NotConditions{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			condition := buildCondition(t, test.build)
			if reflect.TypeOf(condition) != reflect.TypeOf(test.kind) {
				t.Fatalf("group type = %T, want %T", condition, test.kind)
			}
			if test.name == "any of" {
				root := condition.(clause.AndConditions)
				if len(root.Exprs) != 1 {
					t.Fatalf("root conjunction child count = %d, want 1", len(root.Exprs))
				}
				if _, ok := root.Exprs[0].(clause.OrConditions); !ok {
					t.Fatalf("root conjunction child type = %T, want clause.OrConditions", root.Exprs[0])
				}
			}
			if negated, ok := condition.(clause.NotConditions); ok {
				if len(negated.Exprs) != 1 {
					t.Fatalf("NOT child count = %d, want 1", len(negated.Exprs))
				}
				identity, ok := negated.Exprs[0].(clause.OrConditions)
				if !ok || len(identity.Exprs) != 1 {
					t.Fatalf("whole-NOT identity wrapper = %#v", negated.Exprs[0])
				}
			}
		})
	}
}

func buildCondition(
	t testing.TB,
	build func(*weave.Builder[Condition, Expression]),
) Condition {
	t.Helper()
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	builder := factory.New()
	build(builder)
	condition, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if isNilLike(condition) {
		t.Fatal("Build() returned nil Condition")
	}
	return condition
}

func assertGuarded(
	t testing.TB,
	condition Condition,
	wantColumn clause.Column,
) clause.AndConditions {
	t.Helper()
	guarded, ok := condition.(clause.AndConditions)
	if !ok || len(guarded.Exprs) != 2 {
		t.Fatalf("guarded expression = %#v, want two-child clause.AndConditions", condition)
	}
	guard, ok := guarded.Exprs[0].(clause.Neq)
	if !ok || guard.Column != wantColumn || guard.Value != nil {
		t.Fatalf("non-NULL guard = %#v", guarded.Exprs[0])
	}
	return guarded
}
