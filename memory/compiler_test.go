package memory

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
)

type numberRecord struct {
	value int64
	state State
}

type namedInt64 int64

func TestCompilerRequiresAssignableValues(t *testing.T) {
	field, err := NewField(
		"secret-field-label",
		func(record numberRecord) (int64, State) {
			return record.value, StateValue
		},
		OrderedSemantics[int64](),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	condition, err := NewFactory[numberRecord]().New().
		EQ(field, namedInt64(42)).
		Build()
	if condition != nil {
		t.Fatal("Build() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrInvalidValue) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("Build() error = %v, want ErrInvalidValue and ErrCompile", err)
	}

	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("Build() error type = %T, want *weave.Error", err)
	}
	if detail.Phase != weave.PhaseValidate || detail.Operator != weave.OperatorEQ {
		t.Fatalf(
			"error detail = (phase=%s, operator=%s), want (validate, eq)",
			detail.Phase,
			detail.Operator,
		)
	}
	if detail.FieldType != reflect.TypeOf(field) ||
		detail.ValueType != reflect.TypeFor[namedInt64]() {
		t.Fatalf(
			"error types = (%v, %v), want (%v, %v)",
			detail.FieldType,
			detail.ValueType,
			reflect.TypeOf(field),
			reflect.TypeFor[namedInt64](),
		)
	}
	if strings.Contains(err.Error(), "secret-field-label") ||
		strings.Contains(err.Error(), "42") {
		t.Fatalf("compile error disclosed a field label or query value: %q", err)
	}
}

type label interface {
	Label() string
}

type labelValue string

func (value labelValue) Label() string {
	return string(value)
}

type labelRecord struct {
	value label
}

func TestCompilerAcceptsValuesAssignableToInterfaceField(t *testing.T) {
	field, err := NewField(
		"label",
		func(record labelRecord) (label, State) {
			return record.value, StateValue
		},
		NewSemantics[label](
			func(left, right label) bool { return left.Label() == right.Label() },
			nil,
			nil,
		),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	condition, err := NewFactory[labelRecord]().New().
		EQ(field, labelValue("expected")).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	matched, err := condition.Match(labelRecord{value: labelValue("expected")})
	if err != nil || !matched {
		t.Fatalf("Match() = (%v, %v), want (true, nil)", matched, err)
	}
}

func TestFieldConfigurationIsAValueSnapshot(t *testing.T) {
	accessor := Accessor[numberRecord, int64](func(record numberRecord) (int64, State) {
		return record.value, StateValue
	})
	semantics := OrderedSemantics[int64]()
	field, err := NewField("number", accessor, semantics)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	accessor = func(numberRecord) (int64, State) { return 0, StateMissing }
	semantics = NewSemantics[int64](nil, nil, nil)
	_ = accessor
	_ = semantics

	if !field.Capabilities().Operators.Has(weave.OperatorBetween) {
		t.Fatal("Field capabilities changed after source variables were reassigned")
	}
	condition, err := NewFactory[numberRecord]().New().EQ(field, int64(7)).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	matched, err := condition.Match(numberRecord{value: 7})
	if err != nil || !matched {
		t.Fatalf("Match() = (%v, %v), want (true, nil)", matched, err)
	}
}

func TestCompilerReportsOperatorNotApplicable(t *testing.T) {
	field, err := NewField(
		"value",
		func(record testRecord) (string, State) {
			return record.value, StateValue
		},
		ComparableSemantics[string](),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	condition, err := NewFactory[testRecord]().New().Contains(field, "value").Build()
	if condition != nil {
		t.Fatal("Build() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrOperatorNotApplicable) ||
		!errors.Is(err, weave.ErrCompile) {
		t.Fatalf(
			"Build() error = %v, want ErrOperatorNotApplicable and ErrCompile",
			err,
		)
	}
}

func TestCompilerReturnsStableFirstValidationError(t *testing.T) {
	stringField, err := NewField(
		"first-secret-field",
		func(record testRecord) (string, State) {
			return record.value, StateValue
		},
		StringSemantics(),
	)
	if err != nil {
		t.Fatalf("NewField(string) error = %v", err)
	}
	equalityOnly, err := NewField(
		"second-secret-field",
		func(record testRecord) (string, State) {
			return record.value, StateValue
		},
		ComparableSemantics[string](),
	)
	if err != nil {
		t.Fatalf("NewField(equality) error = %v", err)
	}

	builder := NewFactory[testRecord]().New().
		EQ(stringField, labelValue("secret-query-value")).
		Contains(equalityOnly, "second-query-value")
	predicate, err := builder.Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	root, ok := predicate.Root().AsGroup()
	if !ok {
		t.Fatal("Predicate root is not a group")
	}
	first, ok := root.Child(0)
	if !ok {
		t.Fatal("Predicate root has no first child")
	}

	condition, err := NewCompiler[testRecord]().Compile(predicate)
	if condition != nil {
		t.Fatal("Compile() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("Compile() error = %v, want first-node ErrInvalidValue", err)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("Compile() error type = %T, want *weave.Error", err)
	}
	if detail.Path.String() != first.Path().String() || detail.Origin != first.Origin() {
		t.Fatalf(
			"error location = (%s, %+v), want (%s, %+v)",
			detail.Path,
			detail.Origin,
			first.Path(),
			first.Origin(),
		)
	}
	for _, secret := range []string{
		"first-secret-field",
		"second-secret-field",
		"secret-query-value",
		"second-query-value",
	} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Compile() error disclosed %q: %q", secret, err)
		}
	}
}

func TestCompilerRejectsZeroField(t *testing.T) {
	var field Field[testRecord, string]
	condition, err := NewFactory[testRecord]().New().EQ(field, "value").Build()
	if condition != nil {
		t.Fatal("Build() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("Build() error = %v, want ErrInvalidField", err)
	}
	if _, err := NewCompiler[testRecord]().CapabilitiesFor(field); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("CapabilitiesFor(zero Field) error = %v, want ErrInvalidField", err)
	}

	var pointer *Field[testRecord, string]
	if _, err := NewCompiler[testRecord]().CapabilitiesFor(pointer); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("CapabilitiesFor(nil *Field) error = %v, want ErrInvalidField", err)
	}
}

func TestCompilerRejectsZeroPredicate(t *testing.T) {
	var predicate weave.Predicate[Condition[testRecord], Expression[testRecord]]
	condition, err := NewCompiler[testRecord]().Compile(predicate)
	if condition != nil {
		t.Fatal("Compile() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrInvalidPredicate) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf(
			"Compile() error = %v, want ErrInvalidPredicate and ErrCompile",
			err,
		)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) || detail.Phase != weave.PhaseValidate {
		t.Fatalf("Compile() detail = %#v, want PhaseValidate", detail)
	}
}

func TestEmissionFailureReturnsZeroCondition(t *testing.T) {
	condition, err := emitPredicate(validatedNode[testRecord]{
		kind:     weave.KindComparison,
		operator: weave.OperatorEQ,
	})
	if condition != nil {
		t.Fatal("emitPredicate() returned a non-nil Condition on failure")
	}
	if !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("emitPredicate() error = %v, want ErrCompile", err)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("emitPredicate() error type = %T, want *weave.Error", err)
	}
	if detail.Code != weave.CodeCompileFailure ||
		detail.Phase != weave.PhaseEmit ||
		detail.Operator != weave.OperatorEQ ||
		!errors.Is(detail.Cause, errUnexpectedEmitPlan) {
		t.Fatalf("emitPredicate() detail = %#v, want a structured emit failure", detail)
	}
	if strings.Contains(err.Error(), errUnexpectedEmitPlan.Error()) {
		t.Fatalf("emitPredicate() exposed its cause text: %q", err)
	}
}

func TestCompilerRejectsNilNativePayloads(t *testing.T) {
	t.Run("condition", func(t *testing.T) {
		condition, err := NewFactory[testRecord]().New().
			Native(Condition[testRecord](nil)).
			Build()
		if condition != nil {
			t.Fatal("Build() returned a non-nil Condition on failure")
		}
		assertValidationFeatureError(
			t,
			err,
			weave.FeatureNativeCondition,
		)
	})

	t.Run("expression", func(t *testing.T) {
		condition, err := NewFactory[testRecord]().New().
			Expr(Expression[testRecord](nil)).
			Build()
		if condition != nil {
			t.Fatal("Build() returned a non-nil Condition on failure")
		}
		assertValidationFeatureError(
			t,
			err,
			weave.FeatureNativeExpression,
		)
	})
}

func assertValidationFeatureError(
	t *testing.T,
	err error,
	feature weave.Feature,
) {
	t.Helper()
	if !errors.Is(err, weave.ErrInvalidValue) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("error = %v, want ErrInvalidValue and ErrCompile", err)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("error type = %T, want *weave.Error", err)
	}
	if detail.Phase != weave.PhaseValidate || detail.Feature != feature {
		t.Fatalf(
			"error detail = (phase=%s, feature=%s), want (validate, %s)",
			detail.Phase,
			detail.Feature,
			feature,
		)
	}
}

func TestNativeAndExpressionPropagateExecutionErrors(t *testing.T) {
	t.Run("native", func(t *testing.T) {
		wantErr := errors.New("native execution failed")
		condition, err := NewFactory[testRecord]().New().Native(
			Condition[testRecord](func(testRecord) (bool, error) {
				return false, wantErr
			}),
		).Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if _, err := condition.Match(testRecord{}); !errors.Is(err, wantErr) {
			t.Fatalf("Match() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("expression", func(t *testing.T) {
		wantErr := errors.New("expression execution failed")
		calls := 0
		expression := Expression[testRecord](func(testRecord) (bool, error) {
			calls++
			return false, wantErr
		})
		condition, err := NewFactory[testRecord]().New().Expr(expression).Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if _, err := condition.Match(testRecord{}); !errors.Is(err, wantErr) {
			t.Fatalf("Match() error = %v, want %v", err, wantErr)
		}
		if calls != 1 {
			t.Fatalf("Expression calls = %d, want 1", calls)
		}
	})
}

func TestNativeAndExpressionCapturedStateRemainBorrowed(t *testing.T) {
	tests := []struct {
		name  string
		build func(*weave.Builder[Condition[testRecord], Expression[testRecord]], func(testRecord) (bool, error))
	}{
		{
			name: "native",
			build: func(
				builder *weave.Builder[Condition[testRecord], Expression[testRecord]],
				evaluate func(testRecord) (bool, error),
			) {
				builder.Native(Condition[testRecord](evaluate))
			},
		},
		{
			name: "expression",
			build: func(
				builder *weave.Builder[Condition[testRecord], Expression[testRecord]],
				evaluate func(testRecord) (bool, error),
			) {
				builder.Expr(Expression[testRecord](evaluate))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchedState := false
			builder := NewFactory[testRecord]().New()
			test.build(builder, func(testRecord) (bool, error) {
				return matchedState, nil
			})
			condition, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			matched, err := condition.Match(testRecord{})
			if err != nil || matched {
				t.Fatalf("first Match() = (%v, %v), want (false, nil)", matched, err)
			}
			matchedState = true
			matched, err = condition.Match(testRecord{})
			if err != nil || !matched {
				t.Fatalf("second Match() = (%v, %v), want (true, nil)", matched, err)
			}
		})
	}
}

func TestCompilerDoesNotReadRecordsDuringCompile(t *testing.T) {
	calls := 0
	field, err := NewField(
		"value",
		func(record numberRecord) (int64, State) {
			calls++
			return record.value, record.state
		},
		OrderedSemantics[int64](),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}
	condition, err := NewFactory[numberRecord]().New().
		Between(field, int64(1), int64(3)).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("accessor calls during compile = %d, want 0", calls)
	}
	matched, err := condition.Match(numberRecord{value: 2, state: StateValue})
	if err != nil || !matched {
		t.Fatalf("Match() = (%v, %v), want (true, nil)", matched, err)
	}
	if calls != 1 {
		t.Fatalf("accessor calls after one leaf evaluation = %d, want 1", calls)
	}
}

func TestInvalidAccessorStateAndOrderingPropagate(t *testing.T) {
	t.Run("state", func(t *testing.T) {
		field, err := NewField(
			"value",
			func(numberRecord) (int64, State) {
				return 0, State(99)
			},
			OrderedSemantics[int64](),
		)
		if err != nil {
			t.Fatalf("NewField() error = %v", err)
		}
		condition, err := NewFactory[numberRecord]().New().EQ(field, int64(0)).Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if _, err := condition.Match(numberRecord{}); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("Match() error = %v, want ErrInvalidState", err)
		}
	})

	t.Run("ordering", func(t *testing.T) {
		field, err := NewField(
			"value",
			func(record numberRecord) (int64, State) {
				return record.value, StateValue
			},
			NewSemantics[int64](
				func(left, right int64) bool { return left == right },
				func(int64, int64) Ordering { return Ordering(99) },
				nil,
			),
		)
		if err != nil {
			t.Fatalf("NewField() error = %v", err)
		}
		condition, err := NewFactory[numberRecord]().New().LT(field, int64(1)).Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if _, err := condition.Match(numberRecord{}); !errors.Is(err, ErrInvalidOrdering) {
			t.Fatalf("Match() error = %v, want ErrInvalidOrdering", err)
		}
	})
}

func TestUnorderedValuesDoNotMatchOrderingOperators(t *testing.T) {
	type floatRecord struct {
		value float64
	}
	field, err := NewField(
		"value",
		func(record floatRecord) (float64, State) {
			return record.value, StateValue
		},
		OrderedSemantics[float64](),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	tests := []struct {
		name  string
		build func(*weave.Builder[Condition[floatRecord], Expression[floatRecord]])
	}{
		{
			name: "less than",
			build: func(builder *weave.Builder[Condition[floatRecord], Expression[floatRecord]]) {
				builder.LT(field, float64(1))
			},
		},
		{
			name: "less than or equal",
			build: func(builder *weave.Builder[Condition[floatRecord], Expression[floatRecord]]) {
				builder.LTE(field, float64(1))
			},
		},
		{
			name: "greater than",
			build: func(builder *weave.Builder[Condition[floatRecord], Expression[floatRecord]]) {
				builder.GT(field, float64(1))
			},
		},
		{
			name: "greater than or equal",
			build: func(builder *weave.Builder[Condition[floatRecord], Expression[floatRecord]]) {
				builder.GTE(field, float64(1))
			},
		},
		{
			name: "between",
			build: func(builder *weave.Builder[Condition[floatRecord], Expression[floatRecord]]) {
				builder.Between(field, float64(0), float64(2))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := NewFactory[floatRecord]().New()
			test.build(builder)
			condition, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			matched, err := condition.Match(floatRecord{value: math.NaN()})
			if err != nil || matched {
				t.Fatalf("Match(NaN) = (%v, %v), want (false, nil)", matched, err)
			}
		})
	}
}

func TestGroupEvaluationShortCircuitsLeftToRight(t *testing.T) {
	tests := []struct {
		name  string
		build func(*weave.Builder[Condition[testRecord], Expression[testRecord]], Expression[testRecord], Expression[testRecord])
	}{
		{
			name: "all of false",
			build: func(builder *weave.Builder[Condition[testRecord], Expression[testRecord]], first, second Expression[testRecord]) {
				builder.AllOf(func(group *weave.Group[Expression[testRecord]]) {
					group.Expr(first).Expr(second)
				})
			},
		},
		{
			name: "any of true",
			build: func(builder *weave.Builder[Condition[testRecord], Expression[testRecord]], first, second Expression[testRecord]) {
				builder.AnyOf(func(group *weave.Group[Expression[testRecord]]) {
					group.Expr(first).Expr(second)
				})
			},
		},
		{
			name: "none of true",
			build: func(builder *weave.Builder[Condition[testRecord], Expression[testRecord]], first, second Expression[testRecord]) {
				builder.NoneOf(func(group *weave.Group[Expression[testRecord]]) {
					group.Expr(first).Expr(second)
				})
			},
		},
		{
			name: "not all of false",
			build: func(builder *weave.Builder[Condition[testRecord], Expression[testRecord]], first, second Expression[testRecord]) {
				builder.NotAllOf(func(group *weave.Group[Expression[testRecord]]) {
					group.Expr(first).Expr(second)
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstResult := strings.Contains(test.name, "true")
			firstCalls := 0
			secondCalls := 0
			first := Expression[testRecord](func(testRecord) (bool, error) {
				firstCalls++
				return firstResult, nil
			})
			second := Expression[testRecord](func(testRecord) (bool, error) {
				secondCalls++
				return false, errors.New("second expression must not run")
			})
			builder := NewFactory[testRecord]().New()
			test.build(builder, first, second)
			condition, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if _, err := condition.Match(testRecord{}); err != nil {
				t.Fatalf("Match() error = %v", err)
			}
			if firstCalls != 1 || secondCalls != 0 {
				t.Fatalf(
					"expression calls = (%d, %d), want (1, 0)",
					firstCalls,
					secondCalls,
				)
			}
		})
	}
}

func TestCompilerEmitsMaximumPredicateDepth(t *testing.T) {
	field, err := NewField(
		"value",
		func(record numberRecord) (int64, State) {
			return record.value, StateValue
		},
		OrderedSemantics[int64](),
	)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}

	builder := NewFactory[numberRecord]().New()
	builder.AllOf(maximumDepthScope(field, weave.MaxPredicateDepth-2))
	condition, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	matched, err := condition.Match(numberRecord{value: 7})
	if err != nil || !matched {
		t.Fatalf("Match() = (%v, %v), want (true, nil)", matched, err)
	}
}

func maximumDepthScope(
	field Field[numberRecord, int64],
	groupsBelow int,
) Scope[numberRecord] {
	return func(group *Group[numberRecord]) {
		if groupsBelow == 0 {
			group.EQ(field, int64(7))
			return
		}
		group.AllOf(maximumDepthScope(field, groupsBelow-1))
	}
}
