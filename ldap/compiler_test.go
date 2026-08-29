package ldap

import (
	"errors"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
)

func TestExactStandardLowering(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		build func() (Filter, error)
		want  string
	}{
		{
			name: "eq",
			build: func() (Filter, error) {
				return factory.New().EQ(fixture.cn, "A*()\\\x00").Build()
			},
			want: `(&(2.5.4.3=*)(2.5.4.3=A\2a\28\29\5c\00))`,
		},
		{
			name: "neq",
			build: func() (Filter, error) {
				return factory.New().NEQ(fixture.cn, "Bob").Build()
			},
			want: `(&(2.5.4.3=*)(!(2.5.4.3=Bob)))`,
		},
		{
			name: "lte",
			build: func() (Filter, error) {
				return factory.New().LTE(fixture.uid, int64(20)).Build()
			},
			want: `(&(1.3.6.1.1.1.1.0=*)(1.3.6.1.1.1.1.0<=20))`,
		},
		{
			name: "gte",
			build: func() (Filter, error) {
				return factory.New().GTE(fixture.uid, int64(10)).Build()
			},
			want: `(&(1.3.6.1.1.1.1.0=*)(1.3.6.1.1.1.1.0>=10))`,
		},
		{
			name: "in",
			build: func() (Filter, error) {
				return factory.New().In(fixture.cn, []string{"Alice", "Bob"}).Build()
			},
			want: `(&(2.5.4.3=*)(|(2.5.4.3=Alice)(2.5.4.3=Bob)))`,
		},
		{
			name: "not in",
			build: func() (Filter, error) {
				return factory.New().NotIn(fixture.cn, []string{"Alice", "Bob"}).Build()
			},
			want: `(&(2.5.4.3=*)(!(|(2.5.4.3=Alice)(2.5.4.3=Bob))))`,
		},
		{
			name: "between",
			build: func() (Filter, error) {
				return factory.New().Between(fixture.uid, int64(10), int64(20)).Build()
			},
			want: `(&(1.3.6.1.1.1.1.0=*)(1.3.6.1.1.1.1.0>=10)(1.3.6.1.1.1.1.0<=20))`,
		},
		{
			name: "not null is presence",
			build: func() (Filter, error) {
				return factory.New().NotNull(fixture.cn).Build()
			},
			want: `(2.5.4.3=*)`,
		},
		{
			name: "contains",
			build: func() (Filter, error) {
				return factory.New().Contains(fixture.cn, "A*b").Build()
			},
			want: `(&(2.5.4.3=*)(2.5.4.3=*A\2ab*))`,
		},
		{
			name: "prefix",
			build: func() (Filter, error) {
				return factory.New().HasPrefix(fixture.cn, "Al").Build()
			},
			want: `(&(2.5.4.3=*)(2.5.4.3=Al*))`,
		},
		{
			name: "suffix",
			build: func() (Filter, error) {
				return factory.New().HasSuffix(fixture.cn, "ice").Build()
			},
			want: `(&(2.5.4.3=*)(2.5.4.3=*ice))`,
		},
		{
			name: "empty literal",
			build: func() (Filter, error) {
				return factory.New().Contains(fixture.cn, "").Build()
			},
			want: `(2.5.4.3=*)`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := test.build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if filter.String() != test.want {
				t.Fatalf("filter = %q, want %q", filter.String(), test.want)
			}
		})
	}
}

func TestConstantsBooleanGroupsNativeAndExpr(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}

	filter, err := factory.New().Build()
	if err != nil || filter.String() != `(|(2.5.4.0=*)(!(2.5.4.0=*)))` {
		t.Fatalf("empty root = (%q, %v)", filter.String(), err)
	}
	filter, err = factory.New().In(fixture.cn, []string{}).Build()
	if err != nil || filter.String() != `(&(2.5.4.0=*)(!(2.5.4.0=*)))` {
		t.Fatalf("empty In = (%q, %v)", filter.String(), err)
	}
	filter, err = factory.New().NotIn(fixture.cn, []string{}).Build()
	if err != nil || filter.String() != `(|(2.5.4.0=*)(!(2.5.4.0=*)))` {
		t.Fatalf("empty NotIn = (%q, %v)", filter.String(), err)
	}

	filter, err = factory.New().AnyOf(func(group *Group) {
		group.EQ(fixture.cn, "Alice")
		group.GTE(fixture.uid, int64(10))
	}).Build()
	wantAny := `(|(&(2.5.4.3=*)(2.5.4.3=Alice))(&(1.3.6.1.1.1.1.0=*)(1.3.6.1.1.1.1.0>=10)))`
	if err != nil || filter.String() != wantAny {
		t.Fatalf("AnyOf = (%q, %v), want %q", filter.String(), err, wantAny)
	}

	filter, err = factory.New().NoneOf(func(group *Group) {
		group.EQ(fixture.cn, "Alice")
		group.EQ(fixture.cn, "Bob")
	}).Build()
	wantNone := `(!(|(&(2.5.4.3=*)(2.5.4.3=Alice))(&(2.5.4.3=*)(2.5.4.3=Bob))))`
	if err != nil || filter.String() != wantNone {
		t.Fatalf("NoneOf = (%q, %v), want %q", filter.String(), err, wantNone)
	}

	native, err := ParseFilter(fixture.schema, `(cn=Native)`)
	if err != nil {
		t.Fatal(err)
	}
	filter, err = factory.New().Native(native).Build()
	if err != nil || filter.String() != `(cn=Native)` {
		t.Fatalf("Native = (%q, %v)", filter.String(), err)
	}
	filter, err = factory.New().AnyOf(func(group *Group) {
		group.Expr(`(memberOf=admins)`)
		group.Expr(`(cn:2.5.13.2:=alice)`)
	}).Build()
	if err != nil || filter.String() != `(|(memberOf=admins)(cn:2.5.13.2:=alice))` {
		t.Fatalf("Expr = (%q, %v)", filter.String(), err)
	}
}

func TestEscapeHatchesEnforceSchemaAndRedact(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	otherSchema, err := NewSchema(fixture.cn)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := ParseFilter(otherSchema, `(cn=foreign-secret)`)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := factory.New().Native(foreign).Build()
	if filter.Valid() || !errors.Is(err, weave.ErrInvalidValue) ||
		strings.Contains(err.Error(), "foreign-secret") {
		t.Fatalf("foreign Native = (%q, %v)", filter.String(), err)
	}

	filter, err = factory.New().Expr(`(unknown=expr-secret)`).Build()
	if filter.Valid() || !errors.Is(err, weave.ErrInvalidValue) ||
		strings.Contains(err.Error(), "expr-secret") || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("invalid Expr = (%q, %v)", filter.String(), err)
	}
}

func TestCompilerCanBeReusedConcurrently(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := factory.New().
		EQ(fixture.cn, "Alice").
		Between(fixture.uid, int64(10), int64(20)).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := factory.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	errorsChannel := make(chan error, 16)
	for range 16 {
		go func() {
			got, compileErr := factory.Compile(predicate)
			if compileErr != nil {
				errorsChannel <- compileErr
				return
			}
			if got.String() != want.String() {
				errorsChannel <- errors.New("non-deterministic filter")
				return
			}
			errorsChannel <- nil
		}()
	}
	for range 16 {
		if err := <-errorsChannel; err != nil {
			t.Fatal(err)
		}
	}
}

func TestGeneratedFilterDepthAccountsForExactLowering(t *testing.T) {
	fixture := newLDAPFixture(t)
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		t.Fatal(err)
	}
	deepExpression := strings.Repeat("(!", weave.MaxPredicateDepth) +
		"(cn=Alice)" + strings.Repeat(")", weave.MaxPredicateDepth)
	predicate, err := factory.New().
		NoneOf(deepExpressionScope(deepExpression, weave.MaxPredicateDepth-2)).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}
	filter, err := factory.Compile(predicate)
	if err != nil || !filter.Valid() {
		t.Fatalf("Compile(maximum-depth lowering) = (%q, %v)", filter.String(), err)
	}

	tooDeep := strings.Repeat("(!", weave.MaxPredicateDepth+1) +
		"(cn=Alice)" + strings.Repeat(")", weave.MaxPredicateDepth+1)
	filter, err = factory.New().Expr(tooDeep).Build()
	if filter.Valid() || !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("Compile(over-depth Expr) = (%q, %v)", filter.String(), err)
	}
}

func deepExpressionScope(expression string, groupsBelow int) Scope {
	return func(group *Group) {
		if groupsBelow == 0 {
			group.Expr(expression)
			return
		}
		group.NoneOf(deepExpressionScope(expression, groupsBelow-1))
	}
}
