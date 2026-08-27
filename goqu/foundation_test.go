package goqu

import (
	"errors"
	"reflect"
	"testing"
	"time"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
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
		capabilities := compiler.Capabilities()
		if capabilities.Operators.Count() != 14 ||
			capabilities.Features.Count() != 2 {
			t.Fatalf("capabilities = %#v, want 14 operators and 2 features", capabilities)
		}

		factory, err := NewFactory(profile)
		if err != nil || factory == nil {
			t.Fatalf("NewFactory(%d) = (%#v, %v)", profile, factory, err)
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

func TestCompileFoundationRejectsInvalidPredicateWithNilExpressions(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}

	expressions, err := compiler.Compile(weave.Predicate[Expressions, Expression]{})
	if expressions != nil {
		t.Fatalf("Compile() expressions = %#v, want nil", expressions)
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

	expressions, err = (Compiler{}).Compile(weave.Predicate[Expressions, Expression]{})
	if expressions != nil || !errors.Is(err, weave.ErrInvalidState) ||
		!errors.Is(err, weave.ErrCompile) {
		t.Fatalf("zero Compiler.Compile() = (%#v, %v)", expressions, err)
	}
}

type customScalar struct {
	value string
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
			field:   mustField(t, sqlbuilder.T("users").Col("name"), string("")),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorLT, weave.OperatorContains, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorBetween},
			wantCnt: 13,
		},
		{
			name:    "numeric",
			field:   mustField(t, sqlbuilder.C("score"), int64(0)),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorLT, weave.OperatorGTE, weave.OperatorBetween},
			hasNot:  []weave.Operator{weave.OperatorContains},
			wantCnt: 11,
		},
		{
			name:    "bool",
			field:   mustField(t, sqlbuilder.C("active"), false),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorNotIn, weave.OperatorNotNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "time",
			field:   mustField(t, sqlbuilder.C("created_at"), time.Time{}),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorLT, weave.OperatorGTE},
			hasNot:  []weave.Operator{weave.OperatorBetween, weave.OperatorContains},
			wantCnt: 10,
		},
		{
			name:    "bytes",
			field:   mustField(t, sqlbuilder.C("payload"), []byte(nil)),
			has:     []weave.Operator{weave.OperatorEQ, weave.OperatorIn, weave.OperatorIsNull},
			hasNot:  []weave.Operator{weave.OperatorLT, weave.OperatorContains},
			wantCnt: 6,
		},
		{
			name:    "custom",
			field:   mustField(t, sqlbuilder.C("custom_value"), customScalar{}),
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

type nilIdentifier struct {
	exp.IdentifierExpression
}

func TestFieldCanonicalizesIdentityAndRejectsUnsafeIdentifiers(t *testing.T) {
	field, err := NewField[string](sqlbuilder.I("public.users.name"))
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}
	identifier := field.Identifier()
	if isNilLike(identifier) || identifier.GetSchema() != "public" ||
		identifier.GetTable() != "users" || identifier.GetCol() != "name" {
		t.Fatalf("Identifier() = %#v", identifier)
	}
	if reflect.TypeOf(identifier) != reflect.TypeOf(sqlbuilder.C("name")) {
		t.Fatalf("Identifier() concrete type = %T, want canonical goqu identifier", identifier)
	}

	var typedNil *nilIdentifier
	invalid := []exp.IdentifierExpression{
		nil,
		typedNil,
		exp.NewIdentifierExpression("", "", ""),
		sqlbuilder.C("*"),
		sqlbuilder.C("users.name"),
		sqlbuilder.C("name;drop_table"),
		exp.NewIdentifierExpression("", "", sqlbuilder.L("raw_sql")),
		sqlbuilder.C("name").Schema("public"),
		sqlbuilder.T("users table").Col("name"),
		sqlbuilder.T("users").Col("name-with-dash"),
	}
	for index, candidate := range invalid {
		if field, err := NewField[string](candidate); !errors.Is(err, weave.ErrInvalidField) ||
			field.Identifier() != nil {
			t.Fatalf("invalid identifier %d = (%#v, %v)", index, field, err)
		}
	}

	unicodeField, err := NewField[string](sqlbuilder.T("用户").Col("名称"))
	if err != nil || unicodeField.Identifier() == nil {
		t.Fatalf("Unicode identifier = (%#v, %v)", unicodeField, err)
	}
}

func TestExplicitFieldOperatorsAreExactAndValidated(t *testing.T) {
	identifier := sqlbuilder.C("value")
	field, err := NewFieldWithOperators[string](
		identifier,
		weave.OperatorEQ,
		weave.OperatorEQ,
		weave.OperatorContains,
	)
	if err != nil {
		t.Fatalf("NewFieldWithOperators() error = %v", err)
	}
	capabilities := field.Capabilities()
	if capabilities.Operators.Count() != 2 ||
		!capabilities.Operators.Has(weave.OperatorEQ) ||
		!capabilities.Operators.Has(weave.OperatorContains) ||
		capabilities.Operators.Has(weave.OperatorNEQ) {
		t.Fatalf("explicit capabilities = %#v", capabilities)
	}

	if _, err := NewFieldWithOperators[string](identifier); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("empty operator list error = %v", err)
	}
	if _, err := NewFieldWithOperators[string](identifier, weave.Operator(999)); !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("unknown operator error = %v", err)
	}
	if _, err := NewFieldWithOperators[string](identifier, weave.OperatorBetween); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("string Between error = %v", err)
	}
	if _, err := NewFieldWithOperators[int64](identifier, weave.OperatorContains); !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("numeric text error = %v", err)
	}
}

func TestFieldCapabilityDiscoveryRejectsNonCanonicalValues(t *testing.T) {
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		t.Fatal(err)
	}
	field, err := NewField[int64](sqlbuilder.C("score"))
	if err != nil {
		t.Fatal(err)
	}
	wrapper := struct{ Field[int64] }{Field: field}
	for _, value := range []any{
		"score",
		sqlbuilder.C("score"),
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

func TestExpressionsOfReturnsIndependentTopLevelSlice(t *testing.T) {
	first := sqlbuilder.C("a").Eq(sqlbuilder.V(1))
	second := sqlbuilder.C("b").Eq(sqlbuilder.V(2))
	source := []exp.Expression{first, second}
	cloned := ExpressionsOf(source...)
	source[0] = second
	if !reflect.DeepEqual(cloned[0], first) {
		t.Fatalf("ExpressionsOf() retained caller backing storage")
	}
	cloned[1] = first
	if !reflect.DeepEqual(source[1], second) {
		t.Fatalf("ExpressionsOf() exposed caller backing storage")
	}
}

func mustField[T any](
	t testing.TB,
	identifier exp.IdentifierExpression,
	_ T,
) Field[T] {
	t.Helper()
	field, err := NewField[T](identifier)
	if err != nil {
		t.Fatalf("NewField() error = %v", err)
	}
	return field
}
