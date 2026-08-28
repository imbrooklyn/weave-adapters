package mongo

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestValidationErrorsAreStableDFSStructuredRedactedAndZero(t *testing.T) {
	factory := mustFactory(t)
	valid := mustField[string](t, "valid_value")
	secretField := "secret-field-$where-payload"
	secretValue := "secret-query-payload"
	builder := factory.New().
		EQ(valid, "safe").
		AnyOf(func(group *Group) {
			group.EQ(secretField, secretValue)
			group.EQ(bson.D{{Key: "$where", Value: "secret-fragment"}}, secretValue)
		}).
		EQ("later-secret-field", "later-secret-value")

	for iteration := range 3 {
		filter, err := builder.Build()
		if filter != nil {
			t.Fatalf("Build(%d) filter = %#v, want nil", iteration, filter)
		}
		structured := assertStructuredCompileError(
			t,
			err,
			weave.ErrInvalidField,
			weave.CodeInvalidField,
			weave.OperatorEQ,
			0,
		)
		if structured.Origin.Sequence != 3 ||
			structured.Path.String() != "root.allOf[1].anyOf[0].eq" ||
			structured.FieldType != reflect.TypeFor[string]() ||
			structured.ValueType != reflect.TypeFor[string]() {
			t.Fatalf("stable first DFS error metadata = %#v", structured)
		}
		for _, secret := range []string{
			secretField,
			secretValue,
			"secret-fragment",
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
		build        func(*weave.Builder[Filter, Expression])
		wantSentinel error
		wantCode     weave.ErrorCode
		wantOperator weave.Operator
	}{
		{
			name: "plain string field",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.EQ("private.$where", "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "raw BSON operator field",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.EQ(bson.D{{Key: "$expr", Value: true}}, "private-value")
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "zero typed field",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.EQ(Field[int64]{}, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "typed field pointer",
			build: func(builder *weave.Builder[Filter, Expression]) {
				field := mustField[int64](t, "score")
				builder.EQ(&field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "embedded typed field wrapper",
			build: func(builder *weave.Builder[Filter, Expression]) {
				field := struct{ Field[int64] }{Field: mustField[int64](t, "score")}
				builder.EQ(field, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "convertible comparison value",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.EQ(mustField[int64](t, "score"), score(7))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name: "membership element type",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.In(mustField[int64](t, "score"), []int{7})
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorIn,
		},
		{
			name: "range bound type",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.Between(mustField[int64](t, "score"), int(1), int(2))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorBetween,
		},
		{
			name: "default applicability",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.LT(mustField[bool](t, "active"), false)
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorLT,
		},
		{
			name: "exact explicit operator set",
			build: func(builder *weave.Builder[Filter, Expression]) {
				field, err := NewFieldWithOperators[string]("name", weave.OperatorEQ)
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
			filter, err := builder.Build()
			if filter != nil {
				t.Fatalf("Build() filter = %#v, want nil", filter)
			}
			assertStructuredCompileError(
				t,
				err,
				test.wantSentinel,
				test.wantCode,
				test.wantOperator,
				0,
			)
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "$where") {
				t.Fatalf("error leaks field or query payload: %q", err.Error())
			}
		})
	}

	definedField := mustField[score](t, "score")
	filter, err := mustFactory(t).New().EQ(definedField, score(7)).Build()
	if err != nil || filter == nil {
		t.Fatalf("assignable defined value Build() = (%#v, %v)", filter, err)
	}
}

type leakingBSONValue string

func (value leakingBSONValue) MarshalBSONValue() (byte, []byte, error) {
	return 0, nil, fmt.Errorf("upstream leaked %s", value)
}

type panickingBSONValue string

func (value panickingBSONValue) MarshalBSONValue() (byte, []byte, error) {
	panic("upstream panic leaked " + string(value))
}

func TestBSONPreflightFailuresAndPanicsAreRedactedAndZero(t *testing.T) {
	for _, test := range []struct {
		name   string
		secret string
		build  func(*weave.Builder[Filter, Expression])
	}{
		{
			name:   "marshal error",
			secret: "marshal-secret",
			build: func(builder *weave.Builder[Filter, Expression]) {
				field := mustField[leakingBSONValue](t, "value")
				builder.EQ(field, leakingBSONValue("marshal-secret"))
			},
		},
		{
			name:   "marshal panic",
			secret: "panic-secret",
			build: func(builder *weave.Builder[Filter, Expression]) {
				field := mustField[panickingBSONValue](t, "value")
				builder.EQ(field, panickingBSONValue("panic-secret"))
			},
		},
		{
			name:   "uint64 overflow",
			secret: "",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.EQ(mustField[uint64](t, "value"), uint64(math.MaxUint64))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			filter, err := builder.Build()
			if filter != nil {
				t.Fatalf("Build() filter = %#v, want nil", filter)
			}
			structured := assertStructuredCompileError(
				t,
				err,
				weave.ErrInvalidValue,
				weave.CodeInvalidValue,
				weave.OperatorEQ,
				0,
			)
			if structured.Cause != errBSONValueNotEncodable {
				t.Fatalf("redacted Cause = %v", structured.Cause)
			}
			if test.secret != "" && (strings.Contains(err.Error(), test.secret) ||
				strings.Contains(structured.Cause.Error(), test.secret)) {
				t.Fatalf("error chain leaks secret %q", test.secret)
			}
		})
	}
}

func TestLiteralTextRejectsInvalidUTF8AndNULWithoutLeakingInput(t *testing.T) {
	field := mustField[string](t, "text")
	for _, text := range []string{
		"secret-before\x00secret-after",
		string([]byte{'s', 'e', 'c', 'r', 'e', 't', 0xff}),
	} {
		filter, err := mustFactory(t).New().Contains(field, text).Build()
		if filter != nil {
			t.Fatalf("Build() filter = %#v, want nil", filter)
		}
		assertStructuredCompileError(
			t,
			err,
			weave.ErrInvalidValue,
			weave.CodeInvalidValue,
			weave.OperatorContains,
			0,
		)
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaks literal text: %q", err.Error())
		}
	}
}

func TestNativeAndExprAreOpaqueAndNotRecursivelyValidated(t *testing.T) {
	invalidNestedPayload := make(chan int)
	native := bson.D{{Key: "$caller_owned", Value: invalidNestedPayload}}
	expression := bson.D{{Key: "$another_caller_owned", Value: invalidNestedPayload}}
	filter, err := mustFactory(t).New().
		Native(native).
		AnyOf(func(group *Group) { group.Expr(expression) }).
		Build()
	if err != nil || filter == nil {
		t.Fatalf("opaque escape hatches Build() = (%#v, %v)", filter, err)
	}
	if _, err := bson.Marshal(filter); err == nil {
		t.Fatal("test payload unexpectedly marshaled; recursive validation proof is inconclusive")
	}
}

func TestNilNativeAndExprAreRejectedButEmptyDocumentsAreValid(t *testing.T) {
	for _, test := range []struct {
		name    string
		build   func(*weave.Builder[Filter, Expression])
		feature weave.Feature
	}{
		{
			name:    "nil Native",
			build:   func(builder *weave.Builder[Filter, Expression]) { builder.Native(nil) },
			feature: weave.FeatureNativeCondition,
		},
		{
			name:    "nil Expr",
			build:   func(builder *weave.Builder[Filter, Expression]) { builder.Expr(nil) },
			feature: weave.FeatureNativeExpression,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			filter, err := builder.Build()
			if filter != nil {
				t.Fatalf("Build() filter = %#v, want nil", filter)
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

	for _, build := range []func(*weave.Builder[Filter, Expression]){
		func(builder *weave.Builder[Filter, Expression]) { builder.Native(FilterOf()) },
		func(builder *weave.Builder[Filter, Expression]) { builder.Expr(FilterOf()) },
	} {
		builder := mustFactory(t).New()
		build(builder)
		filter, err := builder.Build()
		if err != nil || filter == nil || len(filter) != 0 {
			t.Fatalf("empty escape hatch Build() = (%#v, %v)", filter, err)
		}
	}
}

func TestDirectEmissionFailureIsStructuredAndZero(t *testing.T) {
	filter, err := emitPredicate(validatedNode{})
	if filter != nil {
		t.Fatalf("emitPredicate() filter = %#v, want nil", filter)
	}
	if !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("emitPredicate() error = %v, want ErrCompile", err)
	}
	var structured *weave.Error
	if !errors.As(err, &structured) ||
		structured.Code != weave.CodeCompileFailure ||
		structured.Phase != weave.PhaseEmit ||
		structured.Cause != errUnexpectedEmitPlan {
		t.Fatalf("emitPredicate() structured error = %#v", structured)
	}
}

func assertStructuredCompileError(
	t testing.TB,
	err error,
	wantSentinel error,
	wantCode weave.ErrorCode,
	wantOperator weave.Operator,
	wantFeature weave.Feature,
) *weave.Error {
	t.Helper()
	if err == nil || !errors.Is(err, wantSentinel) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("error = %v, want %v and ErrCompile", err, wantSentinel)
	}
	var structured *weave.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error type = %T, want *weave.Error", err)
	}
	if structured.Code != wantCode || structured.Phase != weave.PhaseValidate ||
		structured.Operator != wantOperator || structured.Feature != wantFeature {
		t.Fatalf("structured error = %#v", structured)
	}
	return structured
}
