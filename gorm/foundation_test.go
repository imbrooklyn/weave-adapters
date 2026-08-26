package gorm

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

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

func TestNewCompilerAndFactoryValidateProfile(t *testing.T) {
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		compiler, err := NewCompiler(profile)
		if err != nil {
			t.Fatalf("NewCompiler(%d) error = %v", profile, err)
		}
		if compiler.state == nil || compiler.state.profile != profile {
			t.Fatalf("NewCompiler(%d) did not retain its immutable profile", profile)
		}
		if capabilities := compiler.Capabilities(); capabilities.Operators.Count() != 14 || capabilities.Features.Count() != 2 {
			t.Fatalf("capabilities = %#v, want 14 operators and 2 features", capabilities)
		}

		factory, err := NewFactory(profile)
		if err != nil {
			t.Fatalf("NewFactory(%d) error = %v", profile, err)
		}
		if factory == nil {
			t.Fatalf("NewFactory(%d) returned nil", profile)
		}
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

func TestCompileFoundationRejectsInvalidPredicateWithNilCondition(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	condition, err := compiler.Compile(weave.Predicate[Condition, Expression]{})
	if condition != nil {
		t.Fatalf("Compile() condition = %#v, want nil", condition)
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

	condition, err = (Compiler{}).Compile(weave.Predicate[Condition, Expression]{})
	if condition != nil || !errors.Is(err, weave.ErrInvalidState) ||
		!errors.Is(err, weave.ErrCompile) {
		t.Fatalf("zero Compiler.Compile() = (%#v, %v)", condition, err)
	}
}

func TestTypedFieldDefaultsAndCapabilityDiscovery(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	tests := []struct {
		name    string
		field   any
		has     []weave.Operator
		hasNot  []weave.Operator
		wantCnt int
	}{
		{
			name:    "string",
			field:   MustQualifiedField[string]("users", "name"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorContains, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorBetween},
			wantCnt: 9,
		},
		{
			name:    "numeric",
			field:   MustField[int64]("score"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorLT, weave.OperatorGTE, weave.OperatorBetween},
			hasNot:  []weave.Operator{weave.OperatorContains},
			wantCnt: 11,
		},
		{
			name:    "bool",
			field:   MustField[bool]("active"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorNotIn, weave.OperatorNotNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "time",
			field:   MustField[time.Time]("created_at"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorBetween},
			wantCnt: 6,
		},
		{
			name:    "bytes",
			field:   MustField[[]byte]("payload"),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "custom",
			field:   MustField[struct{ Value string }]("custom_value"),
			has:     []weave.Operator{weave.OperatorIsNull, weave.OperatorNotNull},
			hasNot:  []weave.Operator{weave.OperatorEQ, weave.OperatorLT},
			wantCnt: 2,
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
					t.Errorf("capabilities do not contain %s", operator)
				}
			}
			for _, operator := range test.hasNot {
				if capabilities.Operators.Has(operator) {
					t.Errorf("capabilities unexpectedly contain %s", operator)
				}
			}
		})
	}
}

func TestTypedFieldStoresOnlyNonRawColumnIdentity(t *testing.T) {
	field, err := NewQualifiedField[string]("users", "name")
	if err != nil {
		t.Fatalf("NewQualifiedField() error = %v", err)
	}
	want := clause.Column{Table: "users", Name: "name"}
	if field.column != want || field.column.Raw || field.column.Alias != "" {
		t.Fatalf("stored column = %#v, want %#v", field.column, want)
	}
	if field.valueType != reflect.TypeFor[string]() {
		t.Fatalf("stored value type = %v, want string", field.valueType)
	}
}

func TestWithOperatorsIsAnImmutableExplicitDeclaration(t *testing.T) {
	operators := []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorLT,
		weave.OperatorIsNull,
	}
	option := WithOperators(operators...)
	operators[0] = weave.OperatorContains

	field, err := NewField[time.Time]("created_at", option)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}
	capabilities := field.Capabilities()
	if capabilities.Operators.Count() != 3 ||
		!capabilities.Operators.Has(weave.OperatorEQ) ||
		!capabilities.Operators.Has(weave.OperatorLT) ||
		!capabilities.Operators.Has(weave.OperatorIsNull) ||
		capabilities.Operators.Has(weave.OperatorContains) {
		t.Fatalf("explicit capabilities were not preserved")
	}

	stringField, err := NewField[string](
		"name",
		WithOperators(weave.OperatorEQ, weave.OperatorLT),
	)
	if err != nil || !stringField.Capabilities().Operators.Has(weave.OperatorLT) {
		t.Fatalf("explicit string ordering = (%#v, %v)", stringField, err)
	}
}

func TestFieldConstructionRejectsUnsafeOrInapplicableInput(t *testing.T) {
	invalidNames := []string{
		"",
		" ",
		"users.name",
		"*",
		"9name",
		"name;drop",
		"name\x00suffix",
		"name\nsuffix",
	}
	for _, name := range invalidNames {
		if _, err := NewField[string](name); !errors.Is(err, weave.ErrInvalidField) {
			t.Fatalf("NewField(invalid name) error = %v, want ErrInvalidField", err)
		}
	}
	if _, err := NewQualifiedField[string]("", "name"); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("NewQualifiedField(empty table) error = %v, want ErrInvalidField", err)
	}
	if _, err := NewField[string]("name", WithOperators()); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("WithOperators() error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewField[string](
		"name",
		WithOperators(weave.OperatorEQ),
		WithOperators(weave.OperatorNEQ),
	); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("duplicate WithOperators error = %v, want ErrInvalidValue", err)
	}
	if _, err := NewField[int64](
		"id",
		WithOperators(weave.OperatorContains),
	); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("numeric text error = %v, want ErrOperatorNotApplicable", err)
	}
	if _, err := NewField[string](
		"name",
		WithOperators(weave.OperatorBetween),
	); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("string Between error = %v, want ErrOperatorNotApplicable", err)
	}
	if _, err := NewField[string](
		"name",
		WithOperators(weave.Operator(999)),
	); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("unknown operator error = %v, want ErrInvalidValue", err)
	}

	var nilOption FieldOption
	if _, err := NewField[string]("name", nilOption); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("nil option error = %v, want ErrInvalidValue", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustField did not panic for invalid input")
		}
	}()
	_ = MustField[string]("users.name")
}

func TestCapabilitiesForRejectsForeignAndZeroFields(t *testing.T) {
	compiler, err := NewCompiler(PostgreSQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	valid := MustField[string]("name")
	for _, field := range []any{
		"name",
		clause.Column{Name: "name"},
		Field[string]{},
		&valid,
		struct{ Field[string] }{Field: valid},
	} {
		if _, err := compiler.CapabilitiesFor(field); !errors.Is(err, weave.ErrInvalidField) {
			t.Fatalf("CapabilitiesFor(%T) error = %v, want ErrInvalidField", field, err)
		}
	}

	if _, err := (Compiler{}).CapabilitiesFor(MustField[string]("name")); !errors.Is(err, weave.ErrInvalidState) {
		t.Fatalf("zero Compiler CapabilitiesFor error = %v, want ErrInvalidState", err)
	}
}
