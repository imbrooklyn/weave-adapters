package gorm

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

func TestCompilerCapabilitiesAndNormalizedConstants(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	capabilities := compiler.Capabilities()
	if capabilities.Operators.Count() != 14 || capabilities.Features.Count() != 2 {
		t.Fatalf(
			"capabilities = (%d operators, %d features), want (14, 2)",
			capabilities.Operators.Count(),
			capabilities.Features.Count(),
		)
	}
	for _, operator := range []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorLT,
		weave.OperatorLTE,
		weave.OperatorGT,
		weave.OperatorGTE,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorBetween,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	} {
		if !capabilities.Operators.Has(operator) {
			t.Errorf("capabilities do not contain %s", operator)
		}
	}
	for _, feature := range []weave.Feature{
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	} {
		if !capabilities.Features.Has(feature) {
			t.Errorf("capabilities do not contain %s", feature)
		}
	}
	if got := (Compiler{}).Capabilities(); got != (weave.Capabilities{}) {
		t.Fatalf("zero Compiler capabilities = %#v, want zero", got)
	}

	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	condition, err := factory.New().Build()
	if err != nil {
		t.Fatalf("Build(empty root) error = %v", err)
	}
	constant, ok := condition.(clause.Expr)
	if !ok || constant.SQL != trueTemplate || len(constant.Vars) != 0 {
		t.Fatalf("Build(empty root) = %#v, want explicit true clause.Expr", condition)
	}

	id := MustField[int64]("id")
	condition, err = factory.New().In(id, []int64{}).Build()
	if err != nil {
		t.Fatalf("Build(empty In) error = %v", err)
	}
	constant, ok = condition.(clause.Expr)
	if !ok || constant.SQL != falseTemplate || len(constant.Vars) != 0 {
		t.Fatalf("Build(empty In) = %#v, want explicit false clause.Expr", condition)
	}
}

func TestNativeRootExprNestingAndBorrowedIdentity(t *testing.T) {
	factory, err := NewFactory(PostgreSQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	id := MustQualifiedField[int64]("users", "id")
	name := MustQualifiedField[string]("users", "name")
	native := &borrowedExpression{label: "native"}
	nested := &borrowedExpression{label: "nested"}

	condition, err := factory.New().
		EQ(id, int64(7)).
		Native(native).
		AnyOf(func(group *Group) {
			group.EQ(name, "alice")
			group.Expr(nested)
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	root, ok := condition.(clause.AndConditions)
	if !ok || len(root.Exprs) != 3 {
		t.Fatalf("root = %#v, want three-child clause.AndConditions", condition)
	}
	if root.Exprs[1] != native {
		t.Fatal("Native expression identity was not preserved")
	}
	group, ok := root.Exprs[2].(clause.OrConditions)
	if !ok || len(group.Exprs) != 2 || group.Exprs[1] != nested {
		t.Fatalf("nested Expr group = %#v", root.Exprs[2])
	}
}

func TestCompilerConcurrentReuseIsDeterministic(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	factory := weave.NewFactory[Condition, Expression](compiler)
	id := MustQualifiedField[int64]("users", "id")
	name := MustQualifiedField[string]("users", "name")
	predicate, err := factory.New().
		EQ(id, int64(9)).
		Contains(name, "shared%_!").
		Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	const workers = 64
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := compiler.Compile(predicate)
			if err != nil {
				errorsFound <- err
				return
			}
			if !reflect.DeepEqual(got, want) {
				errorsFound <- errors.New("concurrent Compile returned a different clause tree")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Compile() error = %v", err)
	}
}

type borrowedExpression struct {
	label string
}

func (expression *borrowedExpression) Build(builder clause.Builder) {
	builder.WriteQuoted(clause.Column{Name: expression.label})
}
