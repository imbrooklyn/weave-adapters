package goqu

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

func TestValidationErrorsAreStableDFSStructuredRedactedAndZero(t *testing.T) {
	factory := mustFactory(t)
	valid := mustField(t, sqlbuilder.C("valid_value"), "")
	secretField := "secret-field-payload"
	secretValue := "secret-query-payload"
	builder := factory.New().
		EQ(valid, "safe").
		AnyOf(func(group *Group) {
			group.EQ(secretField, secretValue)
			group.EQ(sqlbuilder.L("secret_fragment"), secretValue)
		}).
		EQ("later-secret-field", "later-secret-value")

	for iteration := range 3 {
		expressions, err := builder.Build()
		if expressions != nil {
			t.Fatalf("Build(%d) expressions = %#v, want nil", iteration, expressions)
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
		if detail.Origin.Sequence != 3 ||
			detail.Path.String() != "root.allOf[1].anyOf[0].eq" ||
			detail.FieldType != reflect.TypeFor[string]() ||
			detail.ValueType != reflect.TypeFor[string]() {
			t.Fatalf("stable first DFS error metadata = %#v", detail)
		}
		for _, secret := range []string{
			secretField,
			secretValue,
			"secret_fragment",
			"later-secret-field",
			"later-secret-value",
		} {
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
		build        func(*weave.Builder[Expressions, Expression])
		wantSentinel error
		wantCode     weave.ErrorCode
		wantOperator weave.Operator
	}{
		{
			name: "plain string field",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.EQ("private-field", "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "raw goqu identifier field",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.EQ(sqlbuilder.C("private_field"), "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "raw goqu literal field",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.EQ(sqlbuilder.L("private_fragment"), "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "zero typed field",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.EQ(Field[int64]{}, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "typed field pointer",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("score"), int64(0))
				builder.EQ(&field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "embedded typed field wrapper",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := struct{ Field[int64] }{
					Field: mustField(t, sqlbuilder.C("score"), int64(0)),
				}
				builder.EQ(field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "convertible comparison value",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("score"), int64(0))
				builder.EQ(field, score(7))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "expression-shaped standard value",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("value"), any(nil))
				builder.EQ(field, any(sqlbuilder.L("private_value_fragment")))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "membership element type",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("score"), int64(0))
				builder.In(field, []int{7})
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorIn,
		},
		{
			name: "range bound type",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("score"), int64(0))
				builder.Between(field, int(1), int(2))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorBetween,
		},
		{
			name: "default applicability",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field := mustField(t, sqlbuilder.C("active"), false)
				builder.LT(field, false)
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorLT,
		},
		{
			name: "exact explicit operator set",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				field, err := NewFieldWithOperators[string](
					sqlbuilder.C("name"),
					weave.OperatorEQ,
				)
				if err != nil {
					t.Fatal(err)
				}
				builder.NEQ(field, "private-value")
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorNEQ,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			expressions, err := builder.Build()
			if expressions != nil {
				t.Fatalf("Build() expressions = %#v, want nil", expressions)
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

	definedField := mustField(t, sqlbuilder.C("score"), score(0))
	expressions, err := mustFactory(t).New().EQ(definedField, score(7)).Build()
	if err != nil || len(expressions) != 1 {
		t.Fatalf("assignable defined value Build() = (%#v, %v)", expressions, err)
	}
}

type nilNativeExpression struct{}

func (*nilNativeExpression) Clone() exp.Expression {
	return (*nilNativeExpression)(nil)
}

func (expression *nilNativeExpression) Expression() exp.Expression {
	return expression
}

type scoredValue interface {
	score() int
}

type concreteScore int

func (value concreteScore) score() int {
	return int(value)
}

func TestAssignableInterfaceValuesAreAcceptedWithoutConversion(t *testing.T) {
	field := mustField(t, sqlbuilder.C("score"), scoredValue(nil))
	values := []any{concreteScore(7), concreteScore(11)}
	expressions, err := mustFactory(t).New().
		EQ(field, scoredValue(concreteScore(7))).
		In(field, values).
		Build()
	if err != nil || len(expressions) != 2 {
		t.Fatalf("assignable interface values Build() = (%#v, %v)", expressions, err)
	}
}

func TestValidationRejectsNilNativeAndExprPayloads(t *testing.T) {
	var typedNil *nilNativeExpression
	tests := []struct {
		name    string
		build   func(*weave.Builder[Expressions, Expression])
		feature weave.Feature
	}{
		{
			name: "nil Native slice",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Native(nil)
			},
			feature: weave.FeatureNativeCondition,
		},
		{
			name: "typed nil Native element",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Native(ExpressionsOf(typedNil))
			},
			feature: weave.FeatureNativeCondition,
		},
		{
			name: "nil Expr",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Expr(nil)
			},
			feature: weave.FeatureNativeExpression,
		},
		{
			name: "typed nil Expr",
			build: func(builder *weave.Builder[Expressions, Expression]) {
				builder.Expr(typedNil)
			},
			feature: weave.FeatureNativeExpression,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			expressions, err := builder.Build()
			if expressions != nil {
				t.Fatalf("Build() expressions = %#v, want nil", expressions)
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

	expressions, err := mustFactory(t).New().Native(Expressions{}).Build()
	if err != nil || expressions == nil || len(expressions) != 0 {
		t.Fatalf("non-nil empty Native Build() = (%#v, %v)", expressions, err)
	}
}

func TestDirectEmissionFailureIsStructuredAndZero(t *testing.T) {
	expressions, err := emitPredicate(validatedNode{
		kind:  weave.KindGroup,
		logic: weave.LogicAllOf,
		children: []validatedNode{{
			kind:     weave.KindComparison,
			operator: weave.OperatorEQ,
		}},
	})
	if expressions != nil {
		t.Fatalf("invalid emit expressions = %#v, want nil", expressions)
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
	t testing.TB,
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

func mustFactory(t testing.TB) *Factory {
	t.Helper()
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	return factory
}
