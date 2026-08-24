package gormgen

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
)

func TestCapabilitiesForGeneratedFields(t *testing.T) {
	fixture := newFixtureQuery(t)
	compiler, err := NewCompiler(PostgreSQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	tests := []struct {
		name      string
		field     any
		wantCount int
		want      []weave.Operator
		reject    []weave.Operator
	}{
		{
			name:      "number",
			field:     fixture.User.ID,
			wantCount: 11,
			want: []weave.Operator{
				weave.OperatorEQ,
				weave.OperatorGTE,
				weave.OperatorBetween,
				weave.OperatorIsNull,
			},
			reject: []weave.Operator{weave.OperatorContains},
		},
		{
			name:      "string",
			field:     fixture.User.Name,
			wantCount: 13,
			want: []weave.Operator{
				weave.OperatorEQ,
				weave.OperatorLT,
				weave.OperatorContains,
				weave.OperatorHasPrefix,
			},
			reject: []weave.Operator{weave.OperatorBetween},
		},
		{
			name:      "boolean",
			field:     fixture.User.Active,
			wantCount: 6,
			want: []weave.Operator{
				weave.OperatorEQ,
				weave.OperatorIn,
				weave.OperatorNotNull,
			},
			reject: []weave.Operator{
				weave.OperatorLT,
				weave.OperatorContains,
			},
		},
		{
			name:      "time",
			field:     fixture.User.CreatedAt,
			wantCount: 10,
			want: []weave.Operator{
				weave.OperatorEQ,
				weave.OperatorLTE,
				weave.OperatorIsNull,
			},
			reject: []weave.Operator{
				weave.OperatorBetween,
				weave.OperatorContains,
			},
		},
		{
			name:      "bytes",
			field:     fixture.User.Payload,
			wantCount: 6,
			want: []weave.Operator{
				weave.OperatorEQ,
				weave.OperatorNotIn,
				weave.OperatorIsNull,
			},
			reject: []weave.Operator{
				weave.OperatorLT,
				weave.OperatorContains,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilities, err := compiler.CapabilitiesFor(test.field)
			if err != nil {
				t.Fatalf("CapabilitiesFor() error = %v", err)
			}
			if capabilities.Operators.Count() != test.wantCount {
				t.Fatalf(
					"operator count = %d, want %d",
					capabilities.Operators.Count(),
					test.wantCount,
				)
			}
			for _, operator := range test.want {
				if !capabilities.Operators.Has(operator) {
					t.Errorf("capabilities do not contain %s", operator)
				}
			}
			for _, operator := range test.reject {
				if capabilities.Operators.Has(operator) {
					t.Errorf("capabilities unexpectedly contain %s", operator)
				}
			}
		})
	}

	for name, invalid := range map[string]any{
		"non field": "private-field-value",
		"derived":   fixture.User.Name.Desc(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := compiler.CapabilitiesFor(invalid); !errors.Is(err, weave.ErrInvalidField) {
				t.Fatalf("CapabilitiesFor(invalid) error = %v, want ErrInvalidField", err)
			}
		})
	}

	if _, err := (Compiler{}).CapabilitiesFor(fixture.User.ID); !errors.Is(err, weave.ErrInvalidState) {
		t.Fatalf("zero Compiler CapabilitiesFor() error = %v, want ErrInvalidState", err)
	}
}

func TestCapabilitiesForAppliesFieldSpecsAndRegistry(t *testing.T) {
	fixture := newFixtureQuery(t)
	spec, err := NewFieldSpec[string](
		fixture.User.Name,
		weave.OperatorEQ,
		weave.OperatorIsNull,
	)
	if err != nil {
		t.Fatalf("NewFieldSpec() error = %v", err)
	}
	compiler, err := NewCompiler(
		MySQL,
		WithFieldSpecs(spec),
		WithRegisteredFieldsOnly(),
	)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	capabilities, err := compiler.CapabilitiesFor(fixture.User.Name)
	if err != nil {
		t.Fatalf("CapabilitiesFor(registered) error = %v", err)
	}
	if capabilities.Operators.Count() != 2 ||
		!capabilities.Operators.Has(weave.OperatorEQ) ||
		!capabilities.Operators.Has(weave.OperatorIsNull) {
		t.Fatal("CapabilitiesFor(registered) did not apply FieldSpec replacement set")
	}
	if _, err := compiler.CapabilitiesFor(fixture.User.ID); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("CapabilitiesFor(unregistered) error = %v, want ErrInvalidField", err)
	}
}

func TestBooleanGroupsUseUpstreamClauseComposition(t *testing.T) {
	fixture := newFixtureQuery(t)
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	conditions, err := factory.New().
		AllOf(func(group *Group) {
			group.EQ(fixture.User.ID, int64(1))
			group.GT(fixture.User.ID, int64(0))
		}).
		AnyOf(func(group *Group) {
			group.EQ(fixture.User.Name, "alpha")
			group.EQ(fixture.User.Name, "beta")
		}).
		NoneOf(func(group *Group) {
			group.EQ(fixture.User.ID, int64(2))
			group.EQ(fixture.User.ID, int64(3))
		}).
		NotAllOf(func(group *Group) {
			group.EQ(fixture.User.ID, int64(4))
			group.EQ(fixture.User.ID, int64(5))
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(conditions) != 4 {
		t.Fatalf("condition count = %d, want 4", len(conditions))
	}

	wantTypes := []any{
		clause.AndConditions{},
		clause.OrConditions{},
		clause.NotConditions{},
		clause.NotConditions{},
	}
	for index, condition := range conditions {
		expression, ok := condition.(field.Expr)
		if !ok {
			t.Fatalf("condition %d type = %T, want field.Expr", index, condition)
		}
		raw := expression.RawExpr()
		if reflect.TypeOf(raw) != reflect.TypeOf(wantTypes[index]) {
			t.Fatalf(
				"condition %d RawExpr type = %T, want %T",
				index,
				raw,
				wantTypes[index],
			)
		}
		leaves := collectRawClauses(raw)
		if len(leaves) != 2 {
			t.Fatalf("condition %d leaf count = %d, want 2", index, len(leaves))
		}
		for _, leaf := range leaves {
			if !strings.HasPrefix(leaf.SQL, "(? IS NOT NULL AND ? ") {
				t.Fatalf("ordinary leaf is not NULL-totalized: %q", leaf.SQL)
			}
		}
	}
}

func TestNativeRootOrderExprNestingAndOutputOwnership(t *testing.T) {
	fixture := newFixtureQuery(t)
	compiler, err := NewCompiler(PostgreSQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)
	firstNative := field.NewUnsafeFieldRaw("(native_one = ?)", 1)
	secondNative := field.NewUnsafeFieldRaw("(native_two = ?)", 2)
	nativeInput := Conditions{firstNative, secondNative}
	nestedExpr := fixture.User.Active.Eq(true)

	builder := factory.New().
		EQ(fixture.User.ID, int64(7)).
		Native(nativeInput).
		AnyOf(func(group *Group) {
			group.EQ(fixture.User.Name, "alice")
			group.Expr(nestedExpr)
		})
	nativeInput[0] = secondNative
	predicate, err := builder.Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	conditions, err := factory.Compile(predicate)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(conditions) != 4 {
		t.Fatalf("condition count = %d, want 4", len(conditions))
	}
	if !reflect.DeepEqual(conditions[1], firstNative) ||
		!reflect.DeepEqual(conditions[2], secondNative) {
		t.Fatal("Native conditions did not preserve root order or top-level clone")
	}
	groupExpression, ok := conditions[3].(field.Expr)
	if !ok {
		t.Fatalf("nested Expr condition type = %T, want field.Expr", conditions[3])
	}
	if _, ok := any(groupExpression.RawExpr()).(clause.OrConditions); !ok {
		t.Fatalf("nested Expr group RawExpr type = %T, want clause.OrConditions", groupExpression.RawExpr())
	}

	conditions[1] = secondNative
	again, err := factory.Compile(predicate)
	if err != nil {
		t.Fatalf("second Compile() error = %v", err)
	}
	if !reflect.DeepEqual(again[1], firstNative) {
		t.Fatal("compiled Conditions share top-level backing storage")
	}

	emptyNative, err := factory.New().Native(nil).Build()
	if err != nil {
		t.Fatalf("Build(nil Native) error = %v", err)
	}
	if emptyNative == nil || len(emptyNative) != 0 {
		t.Fatalf("Build(nil Native) = %#v, want non-nil empty Conditions", emptyNative)
	}
}

func TestCompilerConcurrentReuse(t *testing.T) {
	fixture := newFixtureQuery(t)
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)
	predicate, err := factory.New().
		EQ(fixture.User.ID, int64(9)).
		Contains(fixture.User.Name, "shared").
		Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}

	const workers = 32
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			conditions, err := compiler.Compile(predicate)
			if err != nil {
				errorsFound <- err
				return
			}
			if len(conditions) != 2 {
				errorsFound <- errors.New("unexpected concurrent condition count")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Compile() error = %v", err)
	}
}

func collectRawClauses(expression any) []clause.Expr {
	switch typed := expression.(type) {
	case clause.Expr:
		return []clause.Expr{typed}
	case clause.AndConditions:
		return collectClauseExpressionSlice(typed.Exprs)
	case clause.OrConditions:
		return collectClauseExpressionSlice(typed.Exprs)
	case clause.NotConditions:
		return collectClauseExpressionSlice(typed.Exprs)
	default:
		return nil
	}
}

func collectClauseExpressionSlice(expressions []clause.Expression) []clause.Expr {
	var result []clause.Expr
	for _, expression := range expressions {
		result = append(result, collectRawClauses(expression)...)
	}
	return result
}

var _ gen.Condition = field.NewUnsafeFieldRaw(trueTemplate)
