package elasticsearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
)

func FuzzCompileLiteralAndRedaction(f *testing.F) {
	fixture := newCompilerFixture(f, Elasticsearch95ExpensiveQueries)
	for _, seed := range []struct {
		value    string
		operator uint8
	}{
		{value: "plain", operator: 0},
		{value: `*?\`, operator: 1},
		{value: `a*b?c\d`, operator: 2},
		{value: "雪*?\\山", operator: 3},
		{value: "", operator: 0},
		{value: string([]byte{0xff}), operator: 1},
	} {
		f.Add(seed.value, seed.operator)
	}

	f.Fuzz(func(t *testing.T, input string, operator uint8) {
		value := boundedElasticsearchFuzzString(input)
		builder := fixture.factory.New()
		switch operator % 4 {
		case 0:
			builder.EQ(fixture.wildcard, value)
		case 1:
			builder.Contains(fixture.wildcard, value)
		case 2:
			builder.HasPrefix(fixture.wildcard, value)
		case 3:
			builder.HasSuffix(fixture.wildcard, value)
		}
		predicate, err := builder.Predicate()
		if err != nil {
			t.Fatal("literal Predicate construction failed")
		}
		query, err := fixture.compiler.Compile(predicate)
		if !utf8.ValidString(value) {
			if query != nil || !errors.Is(err, weave.ErrInvalidValue) ||
				!errors.Is(err, weave.ErrCompile) {
				t.Fatal("invalid UTF-8 did not produce a structured zero result")
			}
		} else {
			if err != nil || query == nil {
				t.Fatal("valid literal did not compile")
			}
			encoded, marshalErr := json.Marshal(query)
			if marshalErr != nil || !json.Valid(encoded) {
				t.Fatal("compiled literal query was not valid JSON")
			}
			repeated, repeatedErr := fixture.compiler.Compile(predicate)
			if repeatedErr != nil {
				t.Fatal("repeated literal Compile failed")
			}
			repeatedJSON, marshalErr := json.Marshal(repeated)
			if marshalErr != nil || !bytes.Equal(repeatedJSON, encoded) {
				t.Fatal("repeated literal Compile changed deterministic JSON")
			}
		}

		sensitive := "ELASTICSEARCH-FUZZ-SECRET-" + value
		redacted, redactionErr := fixture.factory.New().
			EQ(fixture.integer, sensitive).
			Build()
		if redacted != nil || !errors.Is(redactionErr, weave.ErrInvalidValue) ||
			!errors.Is(redactionErr, weave.ErrCompile) {
			t.Fatal("wrong typed value did not produce a structured zero result")
		}
		if strings.Contains(redactionErr.Error(), sensitive) {
			t.Fatal("Compile error disclosed a query value")
		}
	})
}

func boundedElasticsearchFuzzString(value string) string {
	const maximumBytes = 4 * 1024
	if len(value) > maximumBytes {
		return value[:maximumBytes]
	}
	return value
}
