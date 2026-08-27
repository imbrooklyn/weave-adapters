package goqu

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

func TestCompilerCapabilitiesAreExact(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := compiler.Capabilities()
	for _, operator := range standardOperators() {
		if !capabilities.Operators.Has(operator) {
			t.Errorf("capabilities do not contain %s", operator)
		}
	}
	if capabilities.Operators.Count() != len(standardOperators()) {
		t.Fatalf(
			"operator count = %d, want %d",
			capabilities.Operators.Count(),
			len(standardOperators()),
		)
	}
	for _, feature := range []weave.Feature{
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	} {
		if !capabilities.Features.Has(feature) {
			t.Errorf("capabilities do not contain %s", feature)
		}
	}
	if capabilities.Features.Count() != 2 {
		t.Fatalf("feature count = %d, want 2", capabilities.Features.Count())
	}
	if got := (Compiler{}).Capabilities(); got != (weave.Capabilities{}) {
		t.Fatalf("zero Compiler capabilities = %#v, want zero", got)
	}
}

type borrowedExpression struct {
	label string
}

func (expression *borrowedExpression) Clone() exp.Expression {
	if expression == nil {
		return (*borrowedExpression)(nil)
	}
	cloned := *expression
	return &cloned
}

func (expression *borrowedExpression) Expression() exp.Expression {
	return expression
}

func TestNativeRootAndNestedExprPreserveBorrowedIdentity(t *testing.T) {
	factory, err := NewFactory(PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	number := mustField(t, sqlbuilder.T("users").Col("id"), int64(0))
	nativeOne := &borrowedExpression{label: "native-one"}
	nativeTwo := &borrowedExpression{label: "native-two"}
	nested := &borrowedExpression{label: "nested"}
	native := Expressions{nativeOne, nativeTwo}

	expressions, err := factory.New().
		EQ(number, int64(7)).
		Native(native).
		AnyOf(func(group *Group) {
			group.Expr(nested)
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(expressions) != 4 || expressions[1] != nativeOne ||
		expressions[2] != nativeTwo {
		t.Fatalf("root Native identity/order = %#v", expressions)
	}
	list, ok := expressions[3].(exp.ExpressionList)
	if !ok || len(list.Expressions()) != 1 || list.Expressions()[0] != nested {
		t.Fatalf("nested Expr identity = %#v", expressions[3])
	}
}

func TestCompilerConcurrentReuseIsDeterministicAndOwnsOutputSlice(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatal(err)
	}
	factory := weave.NewFactory[Expressions, Expression](compiler)
	number := mustField(t, sqlbuilder.T("records").Col("number_value"), int64(0))
	text := mustField(t, sqlbuilder.T("records").Col("text_value"), "")
	predicate, err := factory.New().
		EQ(number, int64(9)).
		Contains(text, "shared%_!").
		Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	mutated, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	mutated[0] = sqlbuilder.L("caller_mutation = 1")
	again, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("Compile() reused caller-visible top-level backing storage")
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
				errorsFound <- errors.New("concurrent Compile returned a different expression tree")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Compile() error = %v", err)
	}
}
