package elasticsearch

import (
	"encoding/json"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

var (
	_ Expression                                              = (*types.Query)(nil)
	_ Expression                                              = esdsl.NewBoolQuery()
	_ func(*search.Search, types.QueryVariant) *search.Search = (*search.Search).Query
)

func TestUpstreamTypedQueryAndMarshalShape(t *testing.T) {
	term := esdsl.NewTermQuery(
		"status",
		esdsl.NewFieldValue().String("open"),
	)
	rangeQuery := esdsl.NewLongNumberRangeQuery("age").Gte(18).Lte(65)
	terms := esdsl.NewTermsQuery().AddTermsQuery(
		"status",
		esdsl.NewTermsQueryField().FieldValues(
			esdsl.NewFieldValue().String("open"),
			esdsl.NewFieldValue().String("closed"),
		),
	)
	exists := esdsl.NewExistsQuery().Field("deleted_at")
	prefix := esdsl.NewPrefixQuery("name", "al")
	wildcard := esdsl.NewWildcardQuery("name", `*a\*\?\\b*`)
	query := esdsl.NewBoolQuery().
		Filter(term, rangeQuery).
		Must(terms).
		MustNot(exists).
		Should(prefix, wildcard).
		MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)).
		QueryCaster()

	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"bool":{"filter":[{"term":{"status":{"value":"open"}}},{"range":{"age":{"gte":18,"lte":65}}}],"minimum_should_match":1,"must":[{"terms":{"status":["open","closed"]}}],"must_not":[{"exists":{"field":"deleted_at"}}],"should":[{"prefix":{"name":{"value":"al"}}},{"wildcard":{"name":{"value":"*a\\*\\?\\\\b*"}}}]}}`
	if string(encoded) != want {
		t.Fatalf("query JSON mismatch\n got: %s\nwant: %s", encoded, want)
	}
}

func TestUpstreamRangeVariants(t *testing.T) {
	tests := []struct {
		name  string
		query types.QueryVariant
		want  string
	}{
		{
			name:  "long",
			query: esdsl.NewLongNumberRangeQuery("age").Gt(1).Lte(9),
			want:  `{"range":{"age":{"gt":1,"lte":9}}}`,
		},
		{
			name:  "number",
			query: esdsl.NewNumberRangeQuery("score").Gte(types.Float64(1.5)),
			want:  `{"range":{"score":{"gte":1.5}}}`,
		},
		{
			name: "date",
			query: esdsl.NewDateRangeQuery("created_at").
				Gte("2026-01-01T00:00:00Z").Lt("2027-01-01T00:00:00Z"),
			want: `{"range":{"created_at":{"gte":"2026-01-01T00:00:00Z","lt":"2027-01-01T00:00:00Z"}}}`,
		},
		{
			name:  "term",
			query: esdsl.NewTermRangeQuery("code").Gte("a").Lte("z"),
			want:  `{"range":{"code":{"gte":"a","lte":"z"}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.query.QueryCaster())
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if string(encoded) != test.want {
				t.Fatalf("got %s, want %s", encoded, test.want)
			}
		})
	}
}

func TestESDSLBuilderMustBeCastBeforeMarshal(t *testing.T) {
	builder := esdsl.NewTermQuery(
		"status",
		esdsl.NewFieldValue().String("open"),
	)
	direct, err := json.Marshal(builder)
	if err != nil {
		t.Fatalf("marshal builder: %v", err)
	}
	if string(direct) != `{}` {
		t.Fatalf("direct builder JSON = %s, want {}", direct)
	}

	first := builder.QueryCaster()
	second := builder.QueryCaster()
	if first == second {
		t.Fatal("esdsl builder unexpectedly reused its top-level Query")
	}
	if first.QueryCaster() != first {
		t.Fatal("*types.Query QueryCaster did not return its receiver")
	}
}
