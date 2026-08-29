package ldap

import (
	"errors"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
)

func TestCompileReturnsStableFirstErrorAndZeroFilter(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := factory.New().
		EQ(fixture.uid, "first-secret-value").
		EQ("second-secret-field", int64(2)).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}

	var fingerprint string
	for iteration := range 8 {
		filter, compileErr := factory.Compile(predicate)
		if filter.Valid() || filter.String() != "" {
			t.Fatalf("Compile(%d) returned a nonzero Filter", iteration)
		}
		if !errors.Is(compileErr, weave.ErrInvalidValue) ||
			!errors.Is(compileErr, weave.ErrCompile) {
			t.Fatalf("Compile(%d) error = %v", iteration, compileErr)
		}
		var detail *weave.Error
		if !errors.As(compileErr, &detail) ||
			detail.Code != weave.CodeInvalidValue ||
			detail.Phase != weave.PhaseValidate ||
			detail.Operator != weave.OperatorEQ ||
			detail.Path.String() != "root.allOf[0].eq" ||
			detail.Origin.Sequence != 1 {
			t.Fatalf("Compile(%d) detail = %#v", iteration, detail)
		}
		got := compileErr.Error() + "|" + detail.Path.String()
		if iteration == 0 {
			fingerprint = got
		} else if got != fingerprint {
			t.Fatalf("Compile(%d) error fingerprint changed", iteration)
		}
		for _, secret := range []string{
			"first-secret-value",
			"second-secret-field",
			"bind-secret-credential",
		} {
			if strings.Contains(compileErr.Error(), secret) {
				t.Fatalf("Compile(%d) error disclosed a secret", iteration)
			}
		}
	}

	zero := Filter{}
	if zero.Valid() || zero.String() != "" {
		t.Fatal("zero Filter is not observably invalid")
	}
}

func TestAllFourLogicFormsEmitDeterministically(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	left := `(&(2.5.4.3=*)(2.5.4.3=Alice))`
	right := `(&(1.3.6.1.1.1.1.0=*)(1.3.6.1.1.1.1.0>=2))`
	tests := []struct {
		name  string
		build func() (Filter, error)
		want  string
	}{
		{
			name: "all of",
			build: func() (Filter, error) {
				return factory.New().AllOf(func(group *Group) {
					group.EQ(fixture.cn, "Alice").GTE(fixture.uid, int64(2))
				}).Build()
			},
			want: "(&" + left + right + ")",
		},
		{
			name: "any of",
			build: func() (Filter, error) {
				return factory.New().AnyOf(func(group *Group) {
					group.EQ(fixture.cn, "Alice").GTE(fixture.uid, int64(2))
				}).Build()
			},
			want: "(|" + left + right + ")",
		},
		{
			name: "none of",
			build: func() (Filter, error) {
				return factory.New().NoneOf(func(group *Group) {
					group.EQ(fixture.cn, "Alice").GTE(fixture.uid, int64(2))
				}).Build()
			},
			want: "(!(|" + left + right + "))",
		},
		{
			name: "not all of",
			build: func() (Filter, error) {
				return factory.New().NotAllOf(func(group *Group) {
					group.EQ(fixture.cn, "Alice").GTE(fixture.uid, int64(2))
				}).Build()
			},
			want: "(!(&" + left + right + "))",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := test.build()
			if err != nil || filter.String() != test.want {
				t.Fatalf("Build() = (%q, %v), want %q", filter.String(), err, test.want)
			}
		})
	}
}

func TestNativeExprAndFieldApplicabilityFailuresAreStructured(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		build func() (Filter, error)
		code  weave.ErrorCode
		phase weave.ErrorPhase
	}{
		{
			name: "zero Native",
			build: func() (Filter, error) {
				return factory.New().Native(Filter{}).Build()
			},
			code: weave.CodeInvalidValue, phase: weave.PhaseValidate,
		},
		{
			name: "invalid Expr",
			build: func() (Filter, error) {
				return factory.New().Expr("(cn=expr-secret").Build()
			},
			code: weave.CodeInvalidValue, phase: weave.PhaseValidate,
		},
		{
			name: "multi-valued standard field",
			build: func() (Filter, error) {
				return factory.New().EQ(fixture.memberOf, "field-secret").Build()
			},
			code: weave.CodeOperatorNotApplicable, phase: weave.PhaseValidate,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, compileErr := test.build()
			if filter.Valid() || !errors.Is(compileErr, weave.ErrCompile) {
				t.Fatalf("Build() = (%q, %v)", filter.String(), compileErr)
			}
			var detail *weave.Error
			if !errors.As(compileErr, &detail) ||
				detail.Code != test.code || detail.Phase != test.phase {
				t.Fatalf("Build() detail = %#v", detail)
			}
			for _, secret := range []string{"expr-secret", "field-secret"} {
				if strings.Contains(compileErr.Error(), secret) {
					t.Fatal("structured compile error disclosed a filter or value")
				}
			}
		})
	}
}
