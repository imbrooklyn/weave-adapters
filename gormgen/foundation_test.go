package gormgen

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
)

type definedGeneratedID int64

func TestProfileDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile Profile
		want    string
	}{
		{name: "mysql", profile: MySQL, want: "mysql"},
		{name: "postgresql", profile: PostgreSQL, want: "postgresql"},
		{name: "zero", profile: Profile(0), want: "profile(0)"},
		{name: "unknown", profile: Profile(99), want: "profile(99)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.profile.String(); got != test.want {
				t.Fatalf("Profile.String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNewCompilerValidatesProfileAndOptions(t *testing.T) {
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		compiler, err := NewCompiler(profile)
		if err != nil {
			t.Fatalf("NewCompiler(%d) error = %v", profile, err)
		}
		if compiler.state == nil || compiler.state.profile != profile {
			t.Fatalf("NewCompiler(%d) did not preserve profile", profile)
		}
	}

	for _, profile := range []Profile{0, 99} {
		if _, err := NewCompiler(profile); !errors.Is(err, weave.ErrInvalidValue) {
			t.Fatalf("NewCompiler(%d) error = %v, want ErrInvalidValue", profile, err)
		}
	}

	var nilOption Option
	if _, err := NewCompiler(MySQL, nilOption); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("NewCompiler(nil option) error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewCompiler(MySQL, WithRegisteredFieldsOnly()); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewCompiler(empty registered-only) error = %v, want ErrInvalidField", err)
	}
}

func TestNewFieldSpecUsesGeneratedColumnAndEqSignature(t *testing.T) {
	fixture := newFixtureQuery(t)

	stringSpec, err := NewFieldSpec[string](fixture.User.Name)
	if err != nil {
		t.Fatalf("NewFieldSpec[string]() error = %v", err)
	}
	if stringSpec.valueType != reflect.TypeFor[string]() {
		t.Fatalf("string FieldSpec value type = %v", stringSpec.valueType)
	}
	for _, operator := range []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorLT,
		weave.OperatorContains,
		weave.OperatorIsNull,
	} {
		if !stringSpec.operators.Has(operator) {
			t.Errorf("string FieldSpec does not contain %s", operator)
		}
	}
	if stringSpec.operators.Has(weave.OperatorBetween) {
		t.Fatal("string FieldSpec unexpectedly contains Between")
	}

	numberSpec, err := NewFieldSpec[int64](fixture.User.ID)
	if err != nil {
		t.Fatalf("NewFieldSpec[int64]() error = %v", err)
	}
	if !numberSpec.operators.Has(weave.OperatorBetween) ||
		numberSpec.operators.Has(weave.OperatorContains) {
		t.Fatal("numeric FieldSpec operator inference is incorrect")
	}

	timeSpec, err := NewFieldSpec[time.Time](fixture.User.CreatedAt)
	if err != nil {
		t.Fatalf("NewFieldSpec[time.Time]() error = %v", err)
	}
	if !timeSpec.operators.Has(weave.OperatorGTE) ||
		timeSpec.operators.Has(weave.OperatorBetween) {
		t.Fatal("time FieldSpec operator inference is incorrect")
	}

	bytesSpec, err := NewFieldSpec[[]byte](fixture.User.Payload)
	if err != nil {
		t.Fatalf("NewFieldSpec[[]byte]() error = %v", err)
	}
	if bytesSpec.operators.Has(weave.OperatorLT) ||
		bytesSpec.operators.Has(weave.OperatorContains) {
		t.Fatal("[]byte FieldSpec operator inference is too broad")
	}

	if _, err := NewFieldSpec[int64](fixture.User.Name); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("NewFieldSpec[int64](string field) error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewFieldSpec[definedGeneratedID](fixture.User.ID); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("NewFieldSpec[definedGeneratedID](int64 field) error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewFieldSpec[string](fixture.User.Name.Desc()); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewFieldSpec(derived field) error = %v, want ErrInvalidField", err)
	}
	if _, err := NewFieldSpec[string](fixture.User.Name, weave.Operator(999)); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("NewFieldSpec(unknown operator) error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewFieldSpec[int64](fixture.User.ID, weave.OperatorContains); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("NewFieldSpec(numeric text) error = %v, want ErrOperatorNotApplicable", err)
	}
	if _, err := NewFieldSpec[string](fixture.User.Name, weave.OperatorBetween); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("NewFieldSpec(string Between) error = %v, want ErrOperatorNotApplicable", err)
	}
}

func TestFieldSpecReplacementSetAndRegistryOnly(t *testing.T) {
	fixture := newFixtureQuery(t)
	spec, err := NewFieldSpec[string](
		fixture.User.Name,
		weave.OperatorEQ,
		weave.OperatorIsNull,
	)
	if err != nil {
		t.Fatalf("NewFieldSpec() error = %v", err)
	}
	if spec.operators.Count() != 2 ||
		!spec.operators.Has(weave.OperatorEQ) ||
		!spec.operators.Has(weave.OperatorIsNull) {
		t.Fatal("explicit FieldSpec operators are not a replacement set")
	}

	specs := []FieldSpec{spec}
	option := WithFieldSpecs(specs...)
	specs[0] = FieldSpec{}
	compiler, err := NewCompiler(
		PostgreSQL,
		option,
		WithRegisteredFieldsOnly(),
	)
	if err != nil {
		t.Fatalf("NewCompiler(registered-only) error = %v", err)
	}

	metadata, err := compiler.resolveField(fixture.User.Name)
	if err != nil {
		t.Fatalf("resolveField(registered) error = %v", err)
	}
	if metadata.valueType != reflect.TypeFor[string]() ||
		!equalOperatorSets(metadata.operators, spec.operators) {
		t.Fatal("registered FieldSpec metadata was not preserved")
	}
	if _, err := compiler.resolveField(fixture.User.ID); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("resolveField(unregistered) error = %v, want ErrInvalidField", err)
	}

	wrongNativeType := field.NewInt64("weave_users", "name")
	if _, err := compiler.resolveField(wrongNativeType); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("resolveField(wrong native type) error = %v, want ErrInvalidField", err)
	}

	if _, err := NewCompiler(MySQL, WithFieldSpecs(spec, spec)); err != nil {
		t.Fatalf("NewCompiler(identical duplicate specs) error = %v", err)
	}
	conflicting, err := NewFieldSpec[string](fixture.User.Name, weave.OperatorNEQ)
	if err != nil {
		t.Fatalf("NewFieldSpec(conflicting) error = %v", err)
	}
	if _, err := NewCompiler(MySQL, WithFieldSpecs(spec, conflicting)); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewCompiler(conflicting specs) error = %v, want ErrInvalidField", err)
	}
	if _, err := NewCompiler(MySQL, WithFieldSpecs(FieldSpec{})); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewCompiler(zero spec) error = %v, want ErrInvalidField", err)
	}
}

func TestCompilerCapabilitiesAndNormalizedConstants(t *testing.T) {
	fixture := newFixtureQuery(t)
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	capabilities := compiler.Capabilities()
	if capabilities.Operators.Count() != 14 || capabilities.Features.Count() != 2 {
		t.Fatalf(
			"Compiler capabilities = (%d operators, %d features), want (14, 2)",
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
			t.Errorf("Compiler capabilities do not contain %s", operator)
		}
	}
	for _, feature := range []weave.Feature{
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	} {
		if !capabilities.Features.Has(feature) {
			t.Errorf("Compiler capabilities do not contain %s", feature)
		}
	}

	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	conditions, err := factory.New().Build()
	if err != nil {
		t.Fatalf("Build(empty root) error = %v", err)
	}
	if conditions == nil || len(conditions) != 0 {
		t.Fatalf("Build(empty root) = %#v, want non-nil empty Conditions", conditions)
	}

	conditions, err = factory.New().In(fixture.User.ID, []int64{}).Build()
	if err != nil {
		t.Fatalf("Build(empty In) error = %v", err)
	}
	if len(conditions) != 1 {
		t.Fatalf("Build(empty In) condition count = %d, want 1", len(conditions))
	}
	expression, ok := conditions[0].(field.Expr)
	if !ok {
		t.Fatalf("Build(empty In) condition type = %T, want field.Expr", conditions[0])
	}
	raw, ok := any(expression.RawExpr()).(clause.Expr)
	if !ok || raw.SQL != falseTemplate || len(raw.Vars) != 0 {
		t.Fatalf("false constant RawExpr = %#v", expression.RawExpr())
	}

	conditions, err = factory.New().EQ(fixture.User.Name, "alice").Build()
	if err != nil {
		t.Fatalf("Factory Build(EQ) error = %v", err)
	}
	if len(conditions) != 1 {
		t.Fatalf("Factory Build(EQ) condition count = %d, want 1", len(conditions))
	}
}

func TestNormalizedEmptyGroupsAndNullableIn(t *testing.T) {
	fixture := newFixtureQuery(t)
	factory, err := NewFactory(MySQL)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	for name, build := range map[string]func(*weave.Builder[Conditions, Expression]){
		"all of": func(builder *weave.Builder[Conditions, Expression]) {
			builder.AllOf(func(*Group) {})
		},
		"none of": func(builder *weave.Builder[Conditions, Expression]) {
			builder.NoneOf(func(*Group) {})
		},
	} {
		t.Run(name+" identity", func(t *testing.T) {
			builder := factory.New()
			build(builder)
			conditions, err := builder.Build()
			if err != nil || conditions == nil || len(conditions) != 0 {
				t.Fatalf("Build() = (%#v, %v), want non-nil empty Conditions", conditions, err)
			}
		})
	}

	for name, build := range map[string]func(*weave.Builder[Conditions, Expression]){
		"any of": func(builder *weave.Builder[Conditions, Expression]) {
			builder.AnyOf(func(*Group) {})
		},
		"not all of": func(builder *weave.Builder[Conditions, Expression]) {
			builder.NotAllOf(func(*Group) {})
		},
	} {
		t.Run(name+" false", func(t *testing.T) {
			builder := factory.New()
			build(builder)
			conditions, err := builder.Build()
			if err != nil || len(conditions) != 1 {
				t.Fatalf("Build() = (%#v, %v), want one false condition", conditions, err)
			}
			expression := conditions[0].(field.Expr)
			raw := expression.RawExpr().(clause.Expr)
			if raw.SQL != falseTemplate || len(raw.Vars) != 0 {
				t.Fatalf("false constant RawExpr = %#v", raw)
			}
		})
	}

	value := int64(7)
	conditions, err := factory.New().
		In(fixture.User.ID, []*int64{&value, nil}).
		Build()
	if err != nil || len(conditions) != 1 {
		t.Fatalf("Build(nullable In) = (%#v, %v), want one normalized group", conditions, err)
	}
	expression := conditions[0].(field.Expr)
	leaves := collectRawClauses(expression.RawExpr())
	if len(leaves) != 2 ||
		leaves[0].SQL != "(? IS NOT NULL AND ? IN (?))" ||
		leaves[1].SQL != isNullTemplate {
		t.Fatalf("nullable In leaves = %#v", leaves)
	}
}

func TestConditionsOfClonesTopLevelSlice(t *testing.T) {
	first := field.NewUnsafeFieldRaw(trueTemplate)
	second := field.NewUnsafeFieldRaw(falseTemplate)
	input := []gen.Condition{first, second}

	conditions := ConditionsOf(input...)
	input[0] = second
	expression, ok := conditions[0].(field.Expr)
	if !ok {
		t.Fatalf("ConditionsOf result type = %T, want field.Expr", conditions[0])
	}
	raw, ok := any(expression.RawExpr()).(clause.Expr)
	if !ok || raw.SQL != trueTemplate {
		t.Fatal("ConditionsOf result aliases the caller's slice backing array")
	}
}
