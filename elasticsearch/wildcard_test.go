package elasticsearch

import "testing"

func TestEscapeWildcardLiteral(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "plain", want: "plain"},
		{value: "*?\\", want: `\*\?\\`},
		{value: "a*b?c\\d", want: `a\*b\?c\\d`},
		{value: "雪*?\\山", want: `雪\*\?\\山`},
	}
	for _, test := range tests {
		if got := escapeWildcardLiteral(test.value); got != test.want {
			t.Fatalf("escapeWildcardLiteral(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func FuzzEscapeWildcardLiteral(f *testing.F) {
	for _, seed := range []string{"plain", `*?\\`, "雪*?\\山", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		escaped := escapeWildcardLiteral(value)
		for index := 0; index < len(escaped); index++ {
			switch escaped[index] {
			case '*', '?':
				if index == 0 || escaped[index-1] != '\\' {
					t.Fatalf("unescaped wildcard byte in %q", escaped)
				}
			}
		}
	})
}
