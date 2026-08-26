package gormgen

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
)

func TestValidationErrorsAreStableStructuredAndRedacted(t *testing.T) {
	fixture := newFixtureQuery(t)
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	secretField := "secret-field-payload"
	secretValue := "secret-query-payload"
	builder := factory.New().
		EQ(secretField, secretValue).
		EQ(fixture.User.ID, secretValue)
	conditions, err := builder.Build()
	if conditions != nil {
		t.Fatalf("Build() conditions = %#v, want nil", conditions)
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
		t.Fatalf("first DFS error metadata = %#v", detail)
	}
	for _, secret := range []string{secretField, secretValue} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks secret %q: %q", secret, err.Error())
		}
	}
}

func TestInvalidGeneratedExpressionDoesNotLeakRawSQL(t *testing.T) {
	factory, err := NewFactory(PostgreSQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	secretSQL := "private_raw_field = 'private-value'"
	compiled, err := factory.New().
		EQ(field.NewUnsafeFieldRaw(secretSQL), "private-query-value").
		Build()
	if compiled != nil {
		t.Fatalf("Build() conditions = %#v, want nil", compiled)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidField,
		weave.CodeInvalidField,
		weave.OperatorEQ,
		0,
	)
	for _, secret := range []string{secretSQL, "private-query-value"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks raw field or query value: %q", err.Error())
		}
	}
}

func TestValidationRejectsValueApplicabilityAndRegistryViolations(t *testing.T) {
	fixture := newFixtureQuery(t)
	tests := []struct {
		name         string
		newFactory   func() (*Factory, error)
		build        func(*weave.Builder[Conditions, Expression])
		wantSentinel error
		wantCode     weave.ErrorCode
		wantOperator weave.Operator
	}{
		{
			name:       "invalid value type",
			newFactory: func() (*Factory, error) { return NewFactory(MySQL) },
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.EQ(fixture.User.ID, "private-value")
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name:       "convertible defined value type",
			newFactory: func() (*Factory, error) { return NewFactory(MySQL) },
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.EQ(fixture.User.ID, definedGeneratedID(7))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorEQ,
		},
		{
			name:       "invalid membership element type",
			newFactory: func() (*Factory, error) { return NewFactory(MySQL) },
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.In(fixture.User.ID, []int{7})
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorIn,
		},
		{
			name:       "invalid range bound type",
			newFactory: func() (*Factory, error) { return NewFactory(MySQL) },
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Between(fixture.User.ID, int(1), int(2))
			},
			wantSentinel: weave.ErrInvalidValue,
			wantCode:     weave.CodeInvalidValue,
			wantOperator: weave.OperatorBetween,
		},
		{
			name:       "operator not applicable",
			newFactory: func() (*Factory, error) { return NewFactory(MySQL) },
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Contains(fixture.User.Active, "private-value")
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorContains,
		},
		{
			name: "FieldSpec restriction",
			newFactory: func() (*Factory, error) {
				spec, err := NewFieldSpec[string](
					fixture.User.Name,
					weave.OperatorEQ,
				)
				if err != nil {
					return nil, err
				}
				return NewFactory(MySQL, WithFieldSpecs(spec))
			},
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Contains(fixture.User.Name, "private-value")
			},
			wantSentinel: weave.ErrOperatorNotApplicable,
			wantCode:     weave.CodeOperatorNotApplicable,
			wantOperator: weave.OperatorContains,
		},
		{
			name: "registered only",
			newFactory: func() (*Factory, error) {
				spec, err := NewFieldSpec[string](fixture.User.Name)
				if err != nil {
					return nil, err
				}
				return NewFactory(
					PostgreSQL,
					WithFieldSpecs(spec),
					WithRegisteredFieldsOnly(),
				)
			},
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.EQ(fixture.User.ID, int64(7))
			},
			wantSentinel: weave.ErrInvalidField,
			wantCode:     weave.CodeInvalidField,
			wantOperator: weave.OperatorEQ,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := test.newFactory()
			if err != nil {
				t.Fatalf("NewFactory() error = %v", err)
			}
			builder := factory.New()
			test.build(builder)
			conditions, err := builder.Build()
			if conditions != nil {
				t.Fatalf("Build() conditions = %#v, want nil", conditions)
			}
			assertStructuredCompileError(
				t,
				err,
				test.wantSentinel,
				test.wantCode,
				test.wantOperator,
				0,
			)
			if strings.Contains(err.Error(), "private-value") {
				t.Fatalf("error leaks query value: %q", err.Error())
			}
		})
	}
}

func TestValidationRejectsInvalidNativeAndExprPayloads(t *testing.T) {
	fixture := newFixtureQuery(t)
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	invalidUpstream := Conditions(gen.Cond(clause.Eq{
		Column: "secret-native-column",
		Value:  "secret-native-value",
	}))

	for name, conditions := range map[string]Conditions{
		"nil element":    {nil},
		"upstream error": invalidUpstream,
	} {
		t.Run(name, func(t *testing.T) {
			compiled, err := factory.New().Native(conditions).Build()
			if compiled != nil {
				t.Fatalf("Build() conditions = %#v, want nil", compiled)
			}
			assertStructuredCompileError(
				t,
				err,
				weave.ErrInvalidValue,
				weave.CodeInvalidValue,
				0,
				weave.FeatureNativeCondition,
			)
			for _, secret := range []string{"secret-native-column", "secret-native-value"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaks Native payload: %q", err.Error())
				}
			}
		})
	}

	var nilExpression *field.String
	compiled, err := factory.New().Expr(nilExpression).Build()
	if compiled != nil {
		t.Fatalf("Build(nil Expr) conditions = %#v, want nil", compiled)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidValue,
		weave.CodeInvalidValue,
		0,
		weave.FeatureNativeExpression,
	)

	validExpression := fixture.User.Name.Eq("safe")
	compiled, err = factory.New().Expr(validExpression).Build()
	if err != nil || len(compiled) != 1 {
		t.Fatalf("Build(valid Expr) = (%#v, %v), want one condition", compiled, err)
	}
}

func TestDirectCompileDefendsStatePredicateAndEmission(t *testing.T) {
	fixture := newFixtureQuery(t)
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)
	predicate, err := factory.New().EQ(fixture.User.ID, int64(1)).Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}

	conditions, err := (Compiler{}).Compile(predicate)
	if conditions != nil {
		t.Fatalf("zero Compiler conditions = %#v, want nil", conditions)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidState,
		weave.CodeInvalidState,
		0,
		0,
	)

	conditions, err = compiler.Compile(weave.Predicate[Conditions, Expression]{})
	if conditions != nil {
		t.Fatalf("invalid Predicate conditions = %#v, want nil", conditions)
	}
	assertStructuredCompileError(
		t,
		err,
		weave.ErrInvalidPredicate,
		weave.CodeInvalidPredicate,
		0,
		0,
	)

	conditions, err = emitPredicate(validatedNode{
		kind:  weave.KindGroup,
		logic: weave.LogicAllOf,
		children: []validatedNode{{
			kind:     weave.KindComparison,
			operator: weave.OperatorEQ,
		}},
	})
	if conditions != nil {
		t.Fatalf("invalid emit plan conditions = %#v, want nil", conditions)
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
