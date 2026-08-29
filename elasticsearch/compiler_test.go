package elasticsearch

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave"
)

type compilerFixture struct {
	compiler Compiler
	factory  *Factory
	keyword  Field[string]
	wildcard Field[string]
	integer  Field[int64]
	decimal  Field[float64]
	date     Field[time.Time]
	boolean  Field[bool]
}

type panickingQueryVariant struct{}

func (panickingQueryVariant) QueryCaster() *types.Query {
	panic("expression-secret")
}

func TestCompileExactTypedQueries(t *testing.T) {
	fixture := newCompilerFixture(t, Elasticsearch95ExpensiveQueries)
	date := time.Date(2026, time.August, 29, 13, 14, 15, 123, time.FixedZone("probe", 8*60*60))
	tests := []struct {
		name  string
		build func(*weave.Builder[Query, Expression])
		want  string
	}{
		{
			name: "eq with null value guard",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.EQ(fixture.integer, int64(2))
			},
			want: `{"bool":{"filter":[{"bool":{"filter":[{"exists":{"field":"number"}}],"must_not":[{"term":{"number":{"value":-9223372036854775808}}}]}},{"term":{"number":{"value":2}}}]}}`,
		},
		{
			name: "neq with companion guard",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.NEQ(fixture.wildcard, "plain")
			},
			want: `{"bool":{"filter":[{"term":{"text_state":{"value":"value"}}}],"must_not":[{"term":{"text":{"value":"plain"}}}]}}`,
		},
		{
			name: "terms",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.In(fixture.boolean, []bool{true, false})
			},
			want: `{"bool":{"filter":[{"exists":{"field":"active"}},{"terms":{"active":[true,false]}}]}}`,
		},
		{
			name: "double between",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.Between(fixture.decimal, 1.25, 9.5)
			},
			want: `{"bool":{"filter":[{"exists":{"field":"score"}},{"range":{"score":{"gte":1.25,"lte":9.5}}}]}}`,
		},
		{
			name: "date greater than",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.GT(fixture.date, date)
			},
			want: `{"bool":{"filter":[{"exists":{"field":"created_at"}},{"range":{"created_at":{"gt":"2026-08-29T05:14:15.000000123Z"}}}]}}`,
		},
		{
			name: "literal wildcard",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.Contains(fixture.wildcard, `雪*?\山`)
			},
			want: `{"bool":{"filter":[{"term":{"text_state":{"value":"value"}}},{"wildcard":{"text":{"value":"*雪\\*\\?\\\\山*"}}}]}}`,
		},
		{
			name: "literal prefix is not wildcard escaped",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.HasPrefix(fixture.wildcard, `a*?\`)
			},
			want: `{"bool":{"filter":[{"term":{"text_state":{"value":"value"}}},{"prefix":{"text":{"value":"a*?\\"}}}]}}`,
		},
		{
			name: "empty suffix is value guard",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.HasSuffix(fixture.wildcard, "")
			},
			want: `{"term":{"text_state":{"value":"value"}}}`,
		},
		{
			name: "is null companion",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.IsNull(fixture.wildcard)
			},
			want: `{"term":{"text_state":{"value":"null"}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := fixture.factory.New()
			test.build(builder)
			query, err := builder.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if got := marshalQuery(t, query); got != test.want {
				t.Fatalf("query JSON\n got: %s\nwant: %s", got, test.want)
			}
		})
	}
}

func TestCompileConstantsLogicNativeAndExpr(t *testing.T) {
	fixture := newCompilerFixture(t, Elasticsearch95ExpensiveQueries)

	if got := marshalQuery(t, mustBuild(t, fixture.factory, nil)); got != `{"match_all":{}}` {
		t.Fatalf("empty root = %s", got)
	}

	logicTests := []struct {
		name  string
		build func(*weave.Builder[Query, Expression])
		want  string
	}{
		{
			name: "all",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.AllOf(func(group *Group) {
					group.EQ(fixture.keyword, "a").EQ(fixture.keyword, "b")
				})
			},
			want: `"filter"`,
		},
		{
			name: "any",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.AnyOf(func(group *Group) {
					group.EQ(fixture.keyword, "a").EQ(fixture.keyword, "b")
				})
			},
			want: `"minimum_should_match":1`,
		},
		{
			name: "none",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.NoneOf(func(group *Group) {
					group.EQ(fixture.keyword, "a").EQ(fixture.keyword, "b")
				})
			},
			want: `"must_not"`,
		},
		{
			name: "not all",
			build: func(builder *weave.Builder[Query, Expression]) {
				builder.NotAllOf(func(group *Group) {
					group.EQ(fixture.keyword, "a").EQ(fixture.keyword, "b")
				})
			},
			want: `"must_not":[{"bool":{"filter"`,
		},
	}
	for _, test := range logicTests {
		t.Run(test.name, func(t *testing.T) {
			encoded := marshalQuery(t, mustBuild(t, fixture.factory, test.build))
			if !strings.Contains(encoded, test.want) {
				t.Fatalf("query %s does not contain %s", encoded, test.want)
			}
		})
	}

	native := &types.Query{Ids: &types.IdsQuery{Values: []string{"r01"}}}
	builder := fixture.factory.New()
	builder.Native(native)
	compiled, err := builder.Build()
	if err != nil {
		t.Fatalf("Native Build: %v", err)
	}
	if compiled != native {
		t.Fatal("sole root Native did not preserve its borrowed query pointer")
	}

	builder = fixture.factory.New()
	builder.AnyOf(func(group *Group) {
		group.Expr(esdsl.NewTermQuery(
			"status", esdsl.NewFieldValue().String("open"),
		))
		group.EQ(fixture.keyword, "active")
	})
	compiled, err = builder.Build()
	if err != nil {
		t.Fatalf("Expr Build: %v", err)
	}
	encoded := marshalQuery(t, compiled)
	if !strings.Contains(encoded, `{"term":{"status":{"value":"open"}}}`) ||
		!strings.Contains(encoded, `"minimum_should_match":1`) {
		t.Fatalf("Expr query = %s", encoded)
	}
}

func TestCompileOwnershipAndConcurrency(t *testing.T) {
	fixture := newCompilerFixture(t, Elasticsearch95ExpensiveQueries)
	predicate, err := fixture.factory.New().
		EQ(fixture.keyword, "stable").
		Between(fixture.decimal, 1.25, 9.5).
		Predicate()
	if err != nil {
		t.Fatalf("Predicate: %v", err)
	}

	first, err := fixture.compiler.Compile(predicate)
	if err != nil {
		t.Fatalf("first Compile: %v", err)
	}
	second, err := fixture.compiler.Compile(predicate)
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	want := marshalQuery(t, second)
	if first == second || first.Bool == nil || second.Bool == nil ||
		first.Bool == second.Bool || len(first.Bool.Filter) != 2 ||
		first.Bool.Filter[0].Bool == nil ||
		len(first.Bool.Filter[0].Bool.Filter) != 2 {
		t.Fatal("standard Compile did not return independent typed-query state")
	}
	first.Bool.Filter[0].Bool.Filter[1].Term["keyword"] = types.TermQuery{
		Value: "mutated",
	}
	if got := marshalQuery(t, second); got != want {
		t.Fatal("mutating one standard result changed another result")
	}
	third, err := fixture.compiler.Compile(predicate)
	if err != nil || marshalQuery(t, third) != want {
		t.Fatal("mutating one standard result changed later compilation")
	}

	raw := &types.Query{Ids: &types.IdsQuery{Values: []string{"r01"}}}
	rawPredicate, err := fixture.factory.New().Expr(raw).Predicate()
	if err != nil {
		t.Fatalf("raw Expr Predicate: %v", err)
	}
	borrowed, err := fixture.compiler.Compile(rawPredicate)
	if err != nil || borrowed != raw {
		t.Fatal("raw Expr did not preserve its documented borrowed pointer")
	}

	upstream := esdsl.NewTermQuery(
		"reviewed", esdsl.NewFieldValue().String("value"),
	)
	upstreamPredicate, err := fixture.factory.New().Expr(upstream).Predicate()
	if err != nil {
		t.Fatalf("builder Expr Predicate: %v", err)
	}
	upstreamFirst, err := fixture.compiler.Compile(upstreamPredicate)
	if err != nil {
		t.Fatalf("first builder Expr Compile: %v", err)
	}
	upstreamSecond, err := fixture.compiler.Compile(upstreamPredicate)
	if err != nil || upstreamFirst == upstreamSecond {
		t.Fatal("upstream builder did not return fresh top-level Query values")
	}

	const (
		workers  = 32
		compiles = 64
	)
	failures := make(chan string, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range compiles {
				query, compileErr := fixture.compiler.Compile(predicate)
				if compileErr != nil {
					failures <- "concurrent Compile returned an error"
					return
				}
				encoded, marshalErr := json.Marshal(query)
				if marshalErr != nil || string(encoded) != want {
					failures <- "concurrent Compile changed deterministic output"
					return
				}
			}
		}()
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestCompileStableFirstErrorZeroAndRedaction(t *testing.T) {
	fixture := newCompilerFixture(t, Elasticsearch95ExpensiveQueries)
	builder := fixture.factory.New()
	builder.EQ(fixture.integer, "first-secret").
		EQ("second-secret-field", int64(2))
	query, err := builder.Build()
	if query != nil || err == nil {
		t.Fatalf("Build = (%v, %v), want nil error result", query, err)
	}
	var structured *weave.Error
	if !errors.As(err, &structured) ||
		structured.Code != weave.CodeInvalidValue ||
		structured.Phase != weave.PhaseValidate ||
		structured.Operator != weave.OperatorEQ ||
		structured.Origin.Sequence != 1 {
		t.Fatalf("error = %#v", err)
	}
	for _, secret := range []string{"first-secret", "second-secret-field"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}

	builder = fixture.factory.New()
	builder.EQ(fixture.integer, int64(-9223372036854775808))
	query, err = builder.Build()
	if query != nil || !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("reserved sentinel Build = (%v, %v)", query, err)
	}

	builder = fixture.factory.New()
	var nilExpression *types.Query
	builder.Expr(nilExpression)
	query, err = builder.Build()
	if query != nil || !errors.Is(err, weave.ErrInvalidValue) {
		t.Fatalf("nil Expr Build = (%v, %v)", query, err)
	}

	builder = fixture.factory.New()
	builder.Expr(panickingQueryVariant{})
	query, err = builder.Build()
	if query != nil || !errors.Is(err, weave.ErrInvalidValue) ||
		strings.Contains(err.Error(), "expression-secret") {
		t.Fatalf("panicking Expr Build = (%v, %v)", query, err)
	}
}

func TestStrictProfileRejectsKeywordLiteralTextButKeepsWildcard(t *testing.T) {
	fixture := newCompilerFixture(t, Elasticsearch95NoExpensiveQueries)
	keywordCaps, err := fixture.compiler.CapabilitiesFor(fixture.keyword)
	if err != nil {
		t.Fatalf("CapabilitiesFor keyword: %v", err)
	}
	for _, operator := range []weave.Operator{
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	} {
		if keywordCaps.Operators.Has(operator) {
			t.Fatalf("strict keyword exposes %s", operator)
		}
	}
	wildcardCaps, err := fixture.compiler.CapabilitiesFor(fixture.wildcard)
	if err != nil {
		t.Fatalf("CapabilitiesFor wildcard: %v", err)
	}
	for _, operator := range []weave.Operator{
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	} {
		if !wildcardCaps.Operators.Has(operator) {
			t.Fatalf("strict wildcard omits %s", operator)
		}
	}

	builder := fixture.factory.New()
	builder.HasPrefix(fixture.keyword, "prefix")
	query, err := builder.Build()
	if query != nil || !errors.Is(err, weave.ErrOperatorNotApplicable) {
		t.Fatalf("keyword HasPrefix Build = (%v, %v)", query, err)
	}
}

func newCompilerFixture(t testing.TB, profile Profile) compilerFixture {
	t.Helper()
	marker := mustField(t, FieldSpec[string]{
		Path:               "text_state",
		Type:               MappingKeyword,
		CompleteValueIndex: true,
	})
	companion, err := NewCompanionMarker(marker, "null", "value")
	if err != nil {
		t.Fatalf("NewCompanionMarker: %v", err)
	}
	keyword := mustField(t, FieldSpec[string]{
		Path:                   "keyword",
		Type:                   MappingKeyword,
		CompleteValueIndex:     true,
		AllowExpensiveWildcard: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		),
	})
	wildcard := mustField(t, FieldSpec[string]{
		Path:               "text",
		Type:               MappingWildcard,
		CompleteValueIndex: true,
		Nulls:              MarkNullWith[string](companion),
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorIsNull,
			weave.OperatorNotNull,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		),
	})
	integer := mustField(t, FieldSpec[int64]{
		Path:               "number",
		Type:               MappingLong,
		CompleteValueIndex: true,
		Nulls:              IndexNullAs(int64(-9223372036854775808)),
		Operators: weave.NewOperatorSet(
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
		),
	})
	decimal := mustField(t, FieldSpec[float64]{
		Path:               "score",
		Type:               MappingDouble,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorBetween,
		),
	})
	date := mustField(t, FieldSpec[time.Time]{
		Path:               "created_at",
		Type:               MappingDate,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
		),
	})
	boolean := mustField(t, FieldSpec[bool]{
		Path:               "active",
		Type:               MappingBoolean,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
		),
	})
	mapping, err := NewMapping(
		marker, keyword, wildcard, integer, decimal, date, boolean,
	)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	compiler, err := NewCompiler(profile, mapping)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	factory := weave.NewFactory[Query, Expression](compiler)
	return compilerFixture{
		compiler: compiler,
		factory:  factory,
		keyword:  keyword,
		wildcard: wildcard,
		integer:  integer,
		decimal:  decimal,
		date:     date,
		boolean:  boolean,
	}
}

func mustBuild(
	t *testing.T,
	factory *Factory,
	build func(*weave.Builder[Query, Expression]),
) Query {
	t.Helper()
	builder := factory.New()
	if build != nil {
		build(builder)
	}
	query, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return query
}

func marshalQuery(t *testing.T, query Query) string {
	t.Helper()
	if query == nil {
		t.Fatal("query is nil")
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(encoded)
}
