package memory

import (
	"errors"
	"math"
	"testing"

	"github.com/imbrooklyn/weave"
)

type testRecord struct {
	value string
}

func TestStateAndOrderingDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name  string
		value State
		want  string
	}{
		{name: "value", value: StateValue, want: "value"},
		{name: "null", value: StateNull, want: "null"},
		{name: "missing", value: StateMissing, want: "missing"},
		{name: "unknown", value: State(99), want: "state(99)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.value.String(); got != test.want {
				t.Fatalf("State.String() = %q, want %q", got, test.want)
			}
		})
	}

	if !OrderUnordered.Valid() {
		t.Fatal("OrderUnordered.Valid() = false, want true")
	}
	if got := Ordering(99).String(); got != "ordering(99)" {
		t.Fatalf("Ordering(99).String() = %q, want %q", got, "ordering(99)")
	}
}

func TestSemanticsAndFieldCapabilities(t *testing.T) {
	field, err := NewField(
		"value",
		func(record testRecord) (string, State) {
			return record.value, StateValue
		},
		StringSemantics(),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}
	if got := field.Name(); got != "value" {
		t.Fatalf("Field.Name() = %q, want %q", got, "value")
	}

	operators := field.Capabilities().Operators
	for _, operator := range []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorBetween,
		weave.OperatorIsNull,
		weave.OperatorContains,
	} {
		if !operators.Has(operator) {
			t.Errorf("Field capabilities do not contain %s", operator)
		}
	}

	resolved, err := NewCompiler[testRecord]().CapabilitiesFor(field)
	if err != nil {
		t.Fatalf("CapabilitiesFor() error = %v", err)
	}
	if !resolved.Operators.ContainsAll(operators) ||
		resolved.Operators.Count() != operators.Count() {
		t.Fatal("CapabilitiesFor() does not match Field.Capabilities()")
	}

	other, err := NewField(
		"value",
		func(record struct{ value string }) (string, State) {
			return record.value, StateValue
		},
		StringSemantics(),
	)
	if err != nil {
		t.Fatalf("NewField(other record) error = %v", err)
	}
	if _, err := NewCompiler[testRecord]().CapabilitiesFor(other); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("CapabilitiesFor(other record) error = %v, want ErrInvalidField", err)
	}
}

func TestNewFieldRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewField[testRecord, string](
		"  ",
		func(testRecord) (string, State) { return "", StateValue },
		StringSemantics(),
	); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewField(blank name) error = %v, want ErrInvalidField", err)
	}

	if _, err := NewField[testRecord, string](
		"value",
		nil,
		StringSemantics(),
	); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewField(nil accessor) error = %v, want ErrInvalidField", err)
	}
}

func TestOrderedSemanticsMarksNaNUnordered(t *testing.T) {
	semantics := OrderedSemantics[float64]()
	if got := semantics.compare(math.NaN(), 1); got != OrderUnordered {
		t.Fatalf("compare(NaN, 1) = %s, want unordered", got)
	}
}

func TestConditionAndExpressionExecution(t *testing.T) {
	condition := Condition[int](func(record int) (bool, error) {
		return record > 0, nil
	})
	matched, err := condition.Match(1)
	if err != nil || !matched {
		t.Fatalf("Condition.Match(1) = (%v, %v), want (true, nil)", matched, err)
	}

	expression := Expression[int](func(record int) (bool, error) {
		return record%2 == 0, nil
	})
	matched, err = expression.Evaluate(2)
	if err != nil || !matched {
		t.Fatalf("Expression.Evaluate(2) = (%v, %v), want (true, nil)", matched, err)
	}

	var nilCondition Condition[int]
	if _, err := nilCondition.Match(0); !errors.Is(err, ErrNilCondition) {
		t.Fatalf("nil Condition.Match() error = %v, want ErrNilCondition", err)
	}
	var nilExpression Expression[int]
	if _, err := nilExpression.Evaluate(0); !errors.Is(err, ErrNilExpression) {
		t.Fatalf("nil Expression.Evaluate() error = %v, want ErrNilExpression", err)
	}
}

func TestCompilerFoundation(t *testing.T) {
	factory := NewFactory[testRecord]()

	condition, err := factory.New().Build()
	if err != nil {
		t.Fatalf("Build(empty root) error = %v", err)
	}
	matched, err := condition.Match(testRecord{})
	if err != nil || !matched {
		t.Fatalf("empty root match = (%v, %v), want (true, nil)", matched, err)
	}

	condition, err = factory.New().AnyOf(func(*Group[testRecord]) {}).Build()
	if err != nil {
		t.Fatalf("Build(empty AnyOf) error = %v", err)
	}
	matched, err = condition.Match(testRecord{})
	if err != nil || matched {
		t.Fatalf("empty AnyOf match = (%v, %v), want (false, nil)", matched, err)
	}
}

func TestCompilerCapabilitiesCoverStandardOperations(t *testing.T) {
	capabilities := NewCompiler[testRecord]().Capabilities()
	if got := capabilities.Operators.Count(); got != 14 {
		t.Fatalf("operator count = %d, want 14", got)
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
			t.Errorf("Capabilities() does not contain %s", operator)
		}
	}
	if !capabilities.Features.Has(weave.FeatureNativeCondition) ||
		!capabilities.Features.Has(weave.FeatureNativeExpression) {
		t.Fatal("Capabilities() does not contain both native features")
	}
}
