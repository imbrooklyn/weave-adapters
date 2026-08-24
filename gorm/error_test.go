package gorm

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

func TestValidationErrorsAreStableStructuredRedactedAndZero(t *testing.T) {
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	secretField := "secret-field-payload"
	secretValue := "secret-query-payload"
	builder := factory.New().
		EQ(secretField, secretValue).
		EQ(MustField[int64]("id"), secretValue)

	for iteration := range 3 {
		condition, err := builder.Build()
		if condition != nil {
			t.Fatalf("Build(%d) condition = %#v, want nil", iteration, condition)
		}
		assertStructuredCompileError(
			t,
			err,
			weave.ErrInvalidField,
			weave.CodeInvalidField,
			weave.OperatorEQ,
			0,
		)
		var detail *weave.Error
		if !errors.As(err, &detail) {
			t.Fatalf("Build() error type = %T, want *weave.Error", err)
		}
		if detail.Origin.Sequence != 1 ||
			detail.FieldType != reflect.TypeFor[string]() ||
			detail.ValueType != reflect.TypeFor[string]() {
			t.Fatalf("stable first DFS error metadata = %#v", detail)
		}
		for _, secret := range []string{secretField, secretValue} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaks secret %q: %q", secret, err.Error())
			}
		}
	}
}

func TestValidationRejectsForeignFieldsValuesAndApplicability(t *testing.T) {
	type score int64
	tests := []struct {
		name         string
		build        func(*weave.Builder[Condition, Expression])
		wantSentinel error
		wantCode     weave.ErrorCode
		wantOperator weave.Operator
	}{
		{
			name: "plain string field",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.EQ("private-field", "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "raw clause column field",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.EQ(clause.Column{Name: "private"}, "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "zero typed field",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.EQ(Field[int64]{}, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "typed field pointer",
			build: func(builder *weave.Builder[Condition, Expression]) {
				field := MustField[int64]("score")
				builder.EQ(&field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "embedded typed field wrapper",
			build: func(builder *weave.Builder[Condition, Expression]) {
				field := struct{ Field[int64] }{
					Field: MustField[int64]("score"),
				}
				builder.EQ(field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "convertible comparison value",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.EQ(MustField[int64]("score"), score(7))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "membership element type",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.In(MustField[int64]("score"), []int{7})
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorIn,
		},
		{
			name: "range bound type",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.Between(MustField[int64]("score"), int(1), int(2))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorBetween,
		},
		{
			name: "default applicability",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.LT(MustField[string]("name"), "private-value")
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorLT,
		},
		{
			name: "exact explicit operator set",
			build: func(builder *weave.Builder[Condition, Expression]) {
				field := MustField[string](
					"name",
					WithOperators(weave.OperatorEQ),
				)
				builder.NEQ(field, "private-value")
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorNEQ,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := NewFactory(MySQL)
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			builder := factory.New()
			test.build(builder)
			condition, err := builder.Build()
			if condition != nil {
				t.Fatalf("Build() condition = %#v, want nil", condition)
			}
			assertStructuredCompileError(
				t,
				err,
				test.wantSentinel,
				test.wantCode,
				test.wantOperator,
				0,
			)
			if strings.Contains(err.Error(), "private") {
				t.Fatalf("error leaks field or query payload: %q", err.Error())
			}
		})
	}

	definedField := MustField[score](
		"score",
		WithOperators(weave.OperatorEQ),
	)
	condition, err := mustFactory(t).New().EQ(definedField, score(7)).Build()
	if err != nil || condition == nil {
		t.Fatalf("assignable defined value Build() = (%#v, %v)", condition, err)
	}
}

func TestValidationRejectsTypedNilNativeExprAndValues(t *testing.T) {
	factory := mustFactory(t)

	for _, test := range []struct {
		name    string
		build   func(*weave.Builder[Condition, Expression])
		feature weave.Feature
	}{
		{
			name: "Native",
			build: func(builder *weave.Builder[Condition, Expression]) {
				var condition *borrowedExpression
				builder.Native(condition)
			},
			feature: weave.FeatureNativeCondition,
		},
		{
			name: "Expr",
			build: func(builder *weave.Builder[Condition, Expression]) {
				var expression *borrowedExpression
				builder.Expr(expression)
			},
			feature: weave.FeatureNativeExpression,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := factory.New()
			test.build(builder)
			condition, err := builder.Build()
			if condition != nil {
				t.Fatalf("Build() condition = %#v, want nil", condition)
			}
			assertStructuredCompileError(
				t,
				err,
				weave.ErrInvalidValue,
				weave.CodeInvalidValue,
				0,
				test.feature,
			)
		})
	}

	var nilField *Field[int64]
	condition, err := factory.New().EQ(nilField, int64(7)).Build()
	if condition != nil {
		t.Fatalf("Build(typed nil field) condition = %#v, want nil", condition)
	}
	assertConstructionError(
		t,
		err,
		weave.ErrInvalidField,
		weave.CodeInvalidField,
		weave.OperatorEQ,
	)

	type record struct{ ID int64 }
	pointerField := MustField[*record](
		"record",
		WithOperators(weave.OperatorEQ),
	)
	var value *record
	condition, err = factory.New().EQ(pointerField, value).Build()
	if condition != nil {
		t.Fatalf("Build(typed nil value) condition = %#v, want nil", condition)
	}
	assertConstructionError(
		t,
		err,
		weave.ErrInvalidValue,
		weave.CodeInvalidValue,
		weave.OperatorEQ,
	)
}

func TestDirectCompileDefendsStatePredicateAndEmission(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	factory := weave.NewFactory[Condition, Expression](compiler)
	predicate, err := factory.New().
		EQ(MustField[int64]("id"), int64(1)).
		Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}

	condition, err := (Compiler{}).Compile(predicate)
	if condition != nil {
		t.Fatalf("zero Compiler condition = %#v, want nil", condition)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidState,
		weave.CodeInvalidState,
		0,
		0,
	)

	condition, err = compiler.Compile(weave.Predicate[Condition, Expression]{})
	if condition != nil {
		t.Fatalf("invalid Predicate condition = %#v, want nil", condition)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidPredicate,
		weave.CodeInvalidPredicate,
		0,
		0,
	)

	condition, err = emitPredicate(validatedNode{
		kind:  weave.KindGroup,
		logic: weave.LogicAllOf,
		children: []validatedNode{{
			kind:     weave.KindComparison,
			operator: weave.OperatorEQ,
		}},
	})
	if condition != nil {
		t.Fatalf("invalid emit plan condition = %#v, want nil", condition)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) ||
		detail.Code != weave.CodeCompileFailure ||
		detail.Phase != weave.PhaseEmit ||
		!errors.Is(err, errUnexpectedEmitPlan) {
		t.Fatalf("invalid emit plan error = %#v", err)
	}
}

func assertStructuredCompileError(
	t *testing.T,
	err error,
	wantSentinel error,
	wantCode weave.ErrorCode,
	wantOperator weave.Operator,
	wantFeature weave.Feature,
) {
	t.Helper()
	if !errors.Is(err, wantSentinel) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("error = %v, want %v and ErrCompile", err, wantSentinel)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("error type = %T, want *weave.Error", err)
	}
	if detail.Code != wantCode ||
		detail.Phase != weave.PhaseValidate ||
		detail.Operator != wantOperator ||
		detail.Feature != wantFeature {
		t.Fatalf(
			"error detail = (code=%s phase=%s operator=%s feature=%s)",
			detail.Code,
			detail.Phase,
			detail.Operator,
			detail.Feature,
		)
	}
}

func assertConstructionError(
	t *testing.T,
	err error,
	wantSentinel error,
	wantCode weave.ErrorCode,
	wantOperator weave.Operator,
) {
	t.Helper()
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("error = %v, want %v", err, wantSentinel)
	}
	var detail *weave.Error
	if !errors.As(err, &detail) {
		t.Fatalf("error type = %T, want *weave.Error", err)
	}
	if detail.Code != wantCode ||
		detail.Phase != weave.PhaseConstruct ||
		detail.Operator != wantOperator {
		t.Fatalf(
			"construction error = (code=%s phase=%s operator=%s)",
			detail.Code,
			detail.Phase,
			detail.Operator,
		)
	}
}

func mustFactory(t testing.TB) *Factory {
	t.Helper()
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	return factory
}
