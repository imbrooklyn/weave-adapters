package mongo

import (
	"bytes"
	"errors"
	"reflect"
	"regexp"
	"sync"
	"testing"

	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestEveryStandardOperatorHasDeterministicGuardedBSONShape(t *testing.T) {
	number := mustField[int64](t, "record.score")
	text := mustField[string](t, "record.name")
	tests := []struct {
		name  string
		build func(*weave.Builder[Filter, Expression])
		want  bson.D
	}{
		{
			name:  "EQ",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.EQ(number, int64(7)) },
			want:  guardedExpected("record.score", "$eq", int64(7)),
		},
		{
			name:  "NEQ",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.NEQ(number, int64(7)) },
			want:  guardedExpected("record.score", "$ne", int64(7)),
		},
		{
			name:  "LT",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.LT(number, int64(7)) },
			want:  guardedExpected("record.score", "$lt", int64(7)),
		},
		{
			name:  "LTE",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.LTE(number, int64(7)) },
			want:  guardedExpected("record.score", "$lte", int64(7)),
		},
		{
			name:  "GT",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.GT(number, int64(7)) },
			want:  guardedExpected("record.score", "$gt", int64(7)),
		},
		{
			name:  "GTE",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.GTE(number, int64(7)) },
			want:  guardedExpected("record.score", "$gte", int64(7)),
		},
		{
			name: "In",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.In(number, []int64{3, 7, 11})
			},
			want: guardedExpected("record.score", "$in", bson.A{int64(3), int64(7), int64(11)}),
		},
		{
			name: "NotIn",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.NotIn(number, []int64{3, 7, 11})
			},
			want: guardedExpected("record.score", "$nin", bson.A{int64(3), int64(7), int64(11)}),
		},
		{
			name: "Between",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.Between(number, int64(3), int64(11))
			},
			want: guardedExpectedDocument(
				"record.score",
				bson.D{
					{Key: "$gte", Value: int64(3)},
					{Key: "$lte", Value: int64(11)},
				},
			),
		},
		{
			name:  "IsNull",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.IsNull(number) },
			want: bson.D{{Key: "$and", Value: bson.A{
				fieldExpected("record.score", "$exists", true),
				fieldExpected("record.score", "$eq", nil),
			}}},
		},
		{
			name:  "NotNull",
			build: func(builder *weave.Builder[Filter, Expression]) { builder.NotNull(number) },
			want: bson.D{{Key: "$and", Value: bson.A{
				fieldExpected("record.score", "$exists", true),
				fieldExpected("record.score", "$ne", nil),
			}}},
		},
		{
			name: "Contains",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.Contains(text, `a.^$*+?()[]{}|\z`)
			},
			want: guardedExpected(
				"record.name",
				"$regex",
				regexp.QuoteMeta(`a.^$*+?()[]{}|\z`),
			),
		},
		{
			name: "HasPrefix",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.HasPrefix(text, "line\n")
			},
			want: guardedExpected(
				"record.name",
				"$regex",
				`\A`+regexp.QuoteMeta("line\n"),
			),
		},
		{
			name: "HasSuffix",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.HasSuffix(text, "line\n")
			},
			want: guardedExpected(
				"record.name",
				"$regex",
				regexp.QuoteMeta("line\n")+`\z`,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			got, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Build() = %#v\nwant %#v", got, test.want)
			}
			assertOrderedBSONOnly(t, got)
			if _, err := bson.Marshal(got); err != nil {
				t.Fatalf("bson.Marshal() error = %v", err)
			}
		})
	}
}

func TestFourLogicFormsConstantsAndRootOrder(t *testing.T) {
	first := bson.D{{Key: "first", Value: 1}}
	second := bson.D{{Key: "second", Value: 2}}
	number := mustField[int64](t, "score")
	logicTests := []struct {
		name  string
		build func(*weave.Builder[Filter, Expression])
		want  bson.D
	}{
		{
			name: "AllOf",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.AllOf(func(group *Group) { group.Expr(first); group.Expr(second) })
			},
			want: bson.D{{Key: "$and", Value: bson.A{first, second}}},
		},
		{
			name: "AnyOf",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.AnyOf(func(group *Group) { group.Expr(first); group.Expr(second) })
			},
			want: bson.D{{Key: "$or", Value: bson.A{first, second}}},
		},
		{
			name: "NoneOf",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.NoneOf(func(group *Group) { group.Expr(first); group.Expr(second) })
			},
			want: bson.D{{Key: "$nor", Value: bson.A{first, second}}},
		},
		{
			name: "NotAllOf",
			build: func(builder *weave.Builder[Filter, Expression]) {
				builder.NotAllOf(func(group *Group) { group.Expr(first); group.Expr(second) })
			},
			want: bson.D{{Key: "$nor", Value: bson.A{
				bson.D{{Key: "$and", Value: bson.A{first, second}}},
			}}},
		},
	}
	for _, test := range logicTests {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			got, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Build() = %#v, want %#v", got, test.want)
			}
		})
	}

	constantTests := []struct {
		name  string
		build func(*weave.Builder[Filter, Expression])
		want  bson.D
	}{
		{name: "empty root", build: func(*weave.Builder[Filter, Expression]) {}, want: bson.D{}},
		{name: "empty AllOf", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.AllOf(func(*Group) {})
		}, want: bson.D{}},
		{name: "empty AnyOf", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.AnyOf(func(*Group) {})
		}, want: bson.D{{Key: "$expr", Value: false}}},
		{name: "empty NoneOf", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.NoneOf(func(*Group) {})
		}, want: bson.D{}},
		{name: "empty NotAllOf", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.NotAllOf(func(*Group) {})
		}, want: bson.D{{Key: "$expr", Value: false}}},
		{name: "empty In", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.In(number, []int64{})
		}, want: bson.D{{Key: "$expr", Value: false}}},
		{name: "empty NotIn", build: func(builder *weave.Builder[Filter, Expression]) {
			builder.NotIn(number, []int64{})
		}, want: bson.D{}},
	}
	for _, test := range constantTests {
		t.Run(test.name, func(t *testing.T) {
			builder := mustFactory(t).New()
			test.build(builder)
			got, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Build() = %#v, want non-nil %#v", got, test.want)
			}
		})
	}

	native := bson.D{{Key: "native", Value: true}}
	expression := bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{1, 1}}}}}
	got, err := mustFactory(t).New().
		Native(native).
		EQ(number, int64(7)).
		Expr(expression).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	want := bson.D{{Key: "$and", Value: bson.A{
		native,
		guardedExpected("score", "$eq", int64(7)),
		expression,
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root order = %#v, want %#v", got, want)
	}
}

func TestNullableInNormalizationKeepsValueThenExplicitNullOrder(t *testing.T) {
	field := mustField[int64](t, "nullable_score")
	value := int64(7)
	got, err := mustFactory(t).New().
		In(field, []*int64{&value, nil}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	want := bson.D{{Key: "$or", Value: bson.A{
		guardedExpected("nullable_score", "$in", bson.A{int64(7)}),
		bson.D{{Key: "$and", Value: bson.A{
			fieldExpected("nullable_score", "$exists", true),
			fieldExpected("nullable_score", "$eq", nil),
		}}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nullable In Build() = %#v, want %#v", got, want)
	}

	got, err = mustFactory(t).New().
		In(field, []*int64{nil, nil}).
		Build()
	want = bson.D{{Key: "$and", Value: bson.A{
		fieldExpected("nullable_score", "$exists", true),
		fieldExpected("nullable_score", "$eq", nil),
	}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("all-nil In Build() = (%#v, %v), want %#v", got, err, want)
	}
}

func TestNativeAndExprShallowCloneTopLevelAndBorrowNestedState(t *testing.T) {
	for _, test := range []struct {
		name  string
		build func(*weave.Builder[Filter, Expression], bson.D)
	}{
		{
			name: "Native",
			build: func(builder *weave.Builder[Filter, Expression], document bson.D) {
				builder.Native(document)
			},
		},
		{
			name: "Expr",
			build: func(builder *weave.Builder[Filter, Expression], document bson.D) {
				builder.Expr(document)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			nested := bson.D{{Key: "inner", Value: 1}}
			input := bson.D{{Key: "outer", Value: nested}}
			builder := mustFactory(t).New()
			test.build(builder, input)
			got, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			got[0].Key = "mutated-top"
			if input[0].Key != "outer" {
				t.Fatal("Compile() retained escape-hatch top-level backing storage")
			}
			outputNested := got[0].Value.(bson.D)
			outputNested[0].Value = 9
			if nested[0].Value != 9 {
				t.Fatal("Compile() unexpectedly deep-cloned borrowed nested state")
			}
		})
	}

	empty, err := mustFactory(t).New().Native(FilterOf()).Build()
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty Native Build() = (%#v, %v)", empty, err)
	}
}

type borrowedStandardValue struct {
	Labels []string `bson:"labels"`
}

func TestStandardValueRemainsABSONValueAndBorrowsNestedState(t *testing.T) {
	field := mustField[borrowedStandardValue](t, "document_value")
	labels := []string{"before"}
	value := borrowedStandardValue{Labels: labels}
	filter, err := mustFactory(t).New().EQ(field, value).Build()
	if err != nil {
		t.Fatal(err)
	}

	guards := filter[0].Value.(bson.A)
	operation := guards[2].(bson.D)
	operators := operation[0].Value.(bson.D)
	gotValue, ok := operators[0].Value.(borrowedStandardValue)
	if !ok || !reflect.DeepEqual(gotValue, value) {
		t.Fatalf("ordinary BSON value = %#v, want unchanged %#v", operators[0].Value, value)
	}

	gotValue.Labels[0] = "after"
	if labels[0] != "after" {
		t.Fatal("Compile() unexpectedly deep-cloned nested ordinary-value state")
	}
}

func TestCompilerConcurrentReuseIsByteDeterministicAndOwnsBSONTopology(t *testing.T) {
	compiler := mustCompiler(t)
	factory := weave.NewFactory[Filter, Expression](compiler)
	number := mustField[int64](t, "record.score")
	text := mustField[string](t, "record.name")
	predicate, err := factory.New().
		EQ(number, int64(9)).
		Contains(text, `shared.*[$or]`).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := bson.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	for iteration := range 100 {
		got, err := compiler.Compile(predicate)
		if err != nil {
			t.Fatalf("Compile(%d) error = %v", iteration, err)
		}
		gotBytes, err := bson.Marshal(got)
		if err != nil || !bytes.Equal(gotBytes, wantBytes) {
			t.Fatalf("Compile(%d) bytes = %x, want %x, err=%v", iteration, gotBytes, wantBytes, err)
		}
	}

	mutated, err := compiler.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	mutated[0].Key = "$mutated"
	again, err := compiler.Compile(predicate)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("Compile() reused top-level BSON topology: (%#v, %v)", again, err)
	}
	children := again[0].Value.(bson.A)
	children[0].(bson.D)[0].Key = "$mutated_nested"
	again, err = compiler.Compile(predicate)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("Compile() reused nested standard BSON topology: (%#v, %v)", again, err)
	}

	const workers = 64
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := compiler.Compile(predicate)
			if err != nil {
				errorsFound <- err
				return
			}
			gotBytes, err := bson.Marshal(got)
			if err != nil {
				errorsFound <- err
				return
			}
			if !bytes.Equal(gotBytes, wantBytes) {
				errorsFound <- errors.New("concurrent Compile returned different BSON bytes")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Compile() error = %v", err)
	}
}

func guardedExpected(path, operator string, value any) bson.D {
	return guardedExpectedDocument(
		path,
		bson.D{{Key: operator, Value: value}},
	)
}

func guardedExpectedDocument(path string, operation bson.D) bson.D {
	return bson.D{{Key: "$and", Value: bson.A{
		fieldExpected(path, "$exists", true),
		fieldExpected(path, "$ne", nil),
		bson.D{{Key: path, Value: operation}},
	}}}
}

func fieldExpected(path, operator string, value any) bson.D {
	return bson.D{{Key: path, Value: bson.D{{Key: operator, Value: value}}}}
}

func assertOrderedBSONOnly(t testing.TB, value any) {
	t.Helper()
	switch typed := value.(type) {
	case bson.D:
		for _, element := range typed {
			assertOrderedBSONOnly(t, element.Value)
		}
	case bson.A:
		for _, element := range typed {
			assertOrderedBSONOnly(t, element)
		}
	case bson.M:
		t.Fatalf("standard output contains unordered bson.M: %#v", typed)
	case map[string]any:
		t.Fatalf("standard output contains unordered map: %#v", typed)
	case []any:
		t.Fatalf("standard output contains plain []any instead of bson.A: %#v", typed)
	case bson.Regex:
		t.Fatalf("standard text output contains a BSON regex value instead of a literal pattern string: %#v", typed)
	}
}
