package mongo

import (
	"strings"
	"testing"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FuzzLiteralTextAlwaysStaysARegexString(f *testing.F) {
	for _, seed := range []string{
		"plain",
		`.*[$or]{1,3}\A\z`,
		"line\n",
		"Unicode 世界",
		"nul\x00value",
		string([]byte{0xff}),
	} {
		f.Add(seed)
	}
	field, err := NewField[string]("text")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, input string) {
		filter, err := mustFactory(t).New().Contains(field, input).Build()
		if !utf8.ValidString(input) || strings.IndexByte(input, 0) >= 0 {
			if err == nil || filter != nil {
				t.Fatalf("invalid input Build() = (%#v, %v)", filter, err)
			}
			return
		}
		if err != nil || filter == nil {
			t.Fatalf("valid input Build() = (%#v, %v)", filter, err)
		}
		assertOrderedBSONOnly(t, filter)
		if _, err := bson.Marshal(filter); err != nil {
			t.Fatalf("bson.Marshal() error = %v", err)
		}
	})
}

func FuzzSafeFieldPathNeverBecomesAnOperatorFragment(f *testing.F) {
	for _, seed := range []string{
		"name",
		"profile.address.city",
		"$where",
		"items.$[value]",
		`{"$expr":true}`,
		"a..b",
		"用户.名称",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, path string) {
		field, err := NewField[string](path)
		if err != nil {
			if field.Path() != "" {
				t.Fatalf("rejected field retained path %q", field.Path())
			}
			return
		}
		for _, segment := range strings.Split(field.Path(), ".") {
			if segment == "" || strings.HasPrefix(segment, "$") {
				t.Fatalf("accepted operator-like path %q", field.Path())
			}
		}
		filter, err := mustFactory(t).New().EQ(field, "value").Build()
		if err != nil || filter == nil {
			t.Fatalf("accepted path Build() = (%#v, %v)", filter, err)
		}
	})
}
