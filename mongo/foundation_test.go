package mongo

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestProfileDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile Profile
		want    string
	}{
		{name: "mongodb 6.0 plus", profile: MongoDB60Plus, want: "mongodb_6_0_plus"},
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

func TestNewCompilerAndFactoryValidateProfile(t *testing.T) {
	compiler, err := NewCompiler(MongoDB60Plus)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	if compiler.state == nil || compiler.state.profile != MongoDB60Plus {
		t.Fatal("NewCompiler() did not retain its immutable profile")
	}
	capabilities := compiler.Capabilities()
	if capabilities.Operators.Count() != 14 ||
		capabilities.Features.Count() != 2 {
		t.Fatalf("capabilities = %#v, want 14 operators and 2 features", capabilities)
	}
	for _, operator := range standardOperators() {
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
	if factory, err := NewFactory(MongoDB60Plus); err != nil || factory == nil {
		t.Fatalf("NewFactory() = (%#v, %v)", factory, err)
	}

	for _, profile := range []Profile{0, 99} {
		if _, err := NewCompiler(profile); !errors.Is(err, weave.ErrInvalidValue) {
			t.Fatalf("NewCompiler(%d) error = %v, want ErrInvalidValue", profile, err)
		}
		if factory, err := NewFactory(profile); factory != nil ||
			!errors.Is(err, weave.ErrInvalidValue) {
			t.Fatalf("NewFactory(%d) = (%#v, %v), want nil ErrInvalidValue", profile, factory, err)
		}
	}
}

func TestCompilerStateContainsOnlyProfile(t *testing.T) {
	stateType := reflect.TypeFor[compilerState]()
	if stateType.NumField() != 1 || stateType.Field(0).Name != "profile" ||
		stateType.Field(0).Type != reflect.TypeFor[Profile]() {
		t.Fatalf("compilerState shape = %v, want only immutable Profile", stateType)
	}
}

func TestCompileFoundationRejectsInvalidPredicateWithNilFilter(t *testing.T) {
	compiler, err := NewCompiler(MongoDB60Plus)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := compiler.Compile(weave.Predicate[Filter, Expression]{})
	if filter != nil {
		t.Fatalf("Compile() filter = %#v, want nil", filter)
	}
	if !errors.Is(err, weave.ErrInvalidPredicate) || !errors.Is(err, weave.ErrCompile) {
		t.Fatalf("Compile() error = %v, want ErrInvalidPredicate and ErrCompile", err)
	}
	var structured *weave.Error
	if !errors.As(err, &structured) ||
		structured.Code != weave.CodeInvalidPredicate ||
		structured.Phase != weave.PhaseValidate {
		t.Fatalf("Compile() structured error = %#v", structured)
	}

	filter, err = (Compiler{}).Compile(weave.Predicate[Filter, Expression]{})
	if filter != nil || !errors.Is(err, weave.ErrInvalidState) ||
		!errors.Is(err, weave.ErrCompile) {
		t.Fatalf("zero Compiler.Compile() = (%#v, %v)", filter, err)
	}
}

type customScalar struct {
	Value string `bson:"value"`
}

func TestTypedFieldDefaultsAndCapabilityDiscovery(t *testing.T) {
	compiler := mustCompiler(t)
	tests := []struct {
		name    string
		field   any
		has     []weave.Operator
		hasNot  []weave.Operator
		wantCnt int
	}{
		{
			name:    "string",
			field:   mustField[string](t, "profile.name"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorLT, weave.OperatorContains, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorBetween},
			wantCnt: 13,
		},
		{
			name:    "numeric",
			field:   mustField[int64](t, "score"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorLT, weave.OperatorGTE, weave.OperatorBetween},
			hasNot:  []weave.Operator{weave.OperatorContains},
			wantCnt: 11,
		},
		{
			name:    "bool",
			field:   mustField[bool](t, "active"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorNotIn, weave.OperatorNotNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "time",
			field:   mustField[time.Time](t, "created_at"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorLT, weave.OperatorGTE},
			hasNot:  []weave.Operator{weave.OperatorBetween, weave.OperatorContains},
			wantCnt: 10,
		},
		{
			name:    "bytes",
			field:   mustField[[]byte](t, "payload"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "custom",
			field:   mustField[customScalar](t, "custom_value"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorNotIn, weave.OperatorNotNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilities, err := compiler.CapabilitiesFor(test.field)
			if err != nil {
				t.Fatalf("CapabilitiesFor() error = %v", err)
			}
			if capabilities.Operators.Count() != test.wantCnt {
				t.Fatalf("operator count = %d, want %d", capabilities.Operators.Count(), test.wantCnt)
			}
			for _, operator := range test.has {
				if !capabilities.Operators.Has(operator) {
					t.Errorf("missing operator %s", operator)
				}
			}
			for _, operator := range test.hasNot {
				if capabilities.Operators.Has(operator) {
					t.Errorf("unexpected operator %s", operator)
				}
			}
		})
	}
}

func TestFieldPathValidationAndUnsafeBoundary(t *testing.T) {
	for _, path := range []string{
		"name",
		"profile.address.city",
		"_private.value_2",
		"用户.资料_2",
	} {
		field, err := NewField[string](path)
		if err != nil || field.Path() != path {
			t.Fatalf("NewField(%q) = (%#v, %v)", path, field, err)
		}
	}

	invalidUTF8 := string([]byte{0xff, 0xfe})
	for _, path := range []string{
		"",
		" name",
		"name ",
		".name",
		"name.",
		"profile..name",
		"$where",
		"profile.$expr",
		"profile.$[item]",
		"profile.0",
		"profile.*",
		"profile.[0]",
		"profile.name-with-dash",
		"profile.name value",
		"profile.\x00name",
		invalidUTF8,
		`{"$gt": 0}`,
	} {
		field, err := NewField[string](path)
		if !errors.Is(err, weave.ErrInvalidField) || field.Path() != "" {
			t.Fatalf("NewField(%q) = (%#v, %v), want zero ErrInvalidField", path, field, err)
		}
	}

	unsafe := UnsafeField[string]("legacy-name.value with space")
	if unsafe.Path() != "legacy-name.value with space" {
		t.Fatalf("UnsafeField() path = %q", unsafe.Path())
	}
	for _, path := range []string{"$where", "items.$[item]", "a..b", "a.\x00b", "a. value"} {
		if got := UnsafeField[string](path); got.Path() != "" {
			t.Fatalf("UnsafeField(%q) = %#v, want zero", path, got)
		}
	}
}

func TestExplicitFieldOperatorsAreExactAndValidated(t *testing.T) {
	field, err := NewFieldWithOperators[string](
		"value",
		weave.OperatorEQ,
		weave.OperatorEQ,
		weave.OperatorContains,
	)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := field.Capabilities()
	if capabilities.Operators.Count() != 2 ||
		!capabilities.Operators.Has(weave.OperatorEQ) ||
		!capabilities.Operators.Has(weave.OperatorContains) ||
		capabilities.Operators.Has(weave.OperatorNEQ) {
		t.Fatalf("explicit capabilities = %#v", capabilities)
	}

	if _, err := NewFieldWithOperators[string]("value"); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("empty operator list error = %v", err)
	}
	if _, err := NewFieldWithOperators[string]("value", weave.Operator(999)); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("unknown operator error = %v", err)
	}
	if _, err := NewFieldWithOperators[string]("value", weave.OperatorBetween); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("string Between error = %v", err)
	}
	if _, err := NewFieldWithOperators[int64]("value", weave.OperatorContains); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("numeric text error = %v", err)
	}
	if field := UnsafeField[string]("legacy-name", weave.OperatorBetween); field.Path() != "" {
		t.Fatalf("UnsafeField invalid applicability = %#v, want zero", field)
	}
}

func TestFieldCapabilityDiscoveryRejectsNonCanonicalValues(t *testing.T) {
	compiler := mustCompiler(t)
	field := mustField[int64](t, "score")
	wrapper := struct{ Field[int64] }{Field: field}
	for _, value := range []any{
		"score",
		bson.D{{Key: "score", Value: 1}},
		Field[int64]{},
		&field,
		wrapper,
	} {
		if _, err := compiler.CapabilitiesFor(value); !errors.Is(err, weave.ErrInvalidField) {
			t.Fatalf("CapabilitiesFor(%T) error = %v, want ErrInvalidField", value, err)
		}
	}
	if _, err := (Compiler{}).CapabilitiesFor(field); !errors.Is(err, weave.ErrInvalidState) {
		t.Fatalf("zero Compiler CapabilitiesFor() error = %v", err)
	}
}

func TestFilterOfReturnsIndependentTopLevelDocument(t *testing.T) {
	first := bson.E{Key: "a", Value: 1}
	second := bson.E{Key: "b", Value: 2}
	source := []bson.E{first, second}
	cloned := FilterOf(source...)
	source[0] = second
	if !reflect.DeepEqual(cloned[0], first) {
		t.Fatal("FilterOf() retained caller top-level backing storage")
	}
	cloned[1] = first
	if !reflect.DeepEqual(source[1], second) {
		t.Fatal("FilterOf() exposed caller top-level backing storage")
	}
	if empty := FilterOf(); empty == nil || len(empty) != 0 {
		t.Fatalf("FilterOf() = %#v, want non-nil empty document", empty)
	}
}

func mustCompiler(t testing.TB) Compiler {
	t.Helper()
	compiler, err := NewCompiler(MongoDB60Plus)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	return compiler
}

func mustFactory(t testing.TB) *Factory {
	t.Helper()
	factory, err := NewFactory(MongoDB60Plus)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	return factory
}

func mustField[T any](t testing.TB, path string) Field[T] {
	t.Helper()
	field, err := NewField[T](path)
	if err != nil {
		t.Fatalf("NewField(%q) error = %v", path, err)
	}
	return field
}
