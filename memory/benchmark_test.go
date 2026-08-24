package memory_test

import (
	"strconv"
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/memory"
)

type benchmarkRecord struct {
	number          int
	text            string
	optional        int
	optionalPresent bool
}

type benchmarkFields struct {
	number   memory.Field[benchmarkRecord, int]
	text     memory.Field[benchmarkRecord, string]
	optional memory.Field[benchmarkRecord, int]
}

var (
	benchmarkMatchSink     bool
	benchmarkMatchCount    int
	benchmarkConditionSink memory.Condition[benchmarkRecord]
)

func BenchmarkConditionSingleRecord(b *testing.B) {
	fields := newBenchmarkFields(b)
	condition := compileBenchmarkCondition(b, func(
		builder *weave.Builder[
			memory.Condition[benchmarkRecord],
			memory.Expression[benchmarkRecord],
		],
	) {
		builder.EQ(fields.number, 42)
	})
	record := benchmarkRecord{number: 42, text: "prefix-needle"}

	b.ReportAllocs()
	b.ResetTimer()
	var matched bool
	for range b.N {
		var err error
		matched, err = condition.Match(record)
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkMatchSink = matched
}

func BenchmarkConditionBatch(b *testing.B) {
	fields := newBenchmarkFields(b)
	factory := memory.NewFactory[benchmarkRecord]()
	predicate := newTypicalBenchmarkPredicate(b, factory, fields)
	condition, err := factory.Compile(predicate)
	if err != nil {
		b.Fatalf("Compile() error = %v", err)
	}
	records := make([]benchmarkRecord, 1024)
	for index := range records {
		records[index] = benchmarkRecord{
			number:          index,
			text:            "record-" + strconv.Itoa(index) + "-needle",
			optional:        index % 7,
			optionalPresent: index%11 != 0,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	count := 0
	for range b.N {
		count = 0
		for _, record := range records {
			matched, matchErr := condition.Match(record)
			if matchErr != nil {
				b.Fatal(matchErr)
			}
			if matched {
				count++
			}
		}
	}
	benchmarkMatchCount = count
}

func BenchmarkBuildTypicalAST(b *testing.B) {
	fields := newBenchmarkFields(b)
	factory := memory.NewFactory[benchmarkRecord]()

	b.ReportAllocs()
	b.ResetTimer()
	var condition memory.Condition[benchmarkRecord]
	for range b.N {
		builder := factory.New()
		addTypicalBenchmarkNodes(builder, fields)
		var err error
		condition, err = builder.Build()
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkConditionSink = condition
}

func BenchmarkCompileMembership(b *testing.B) {
	fields := newBenchmarkFields(b)
	factory := memory.NewFactory[benchmarkRecord]()
	for _, size := range []int{100, 1000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			values := make([]int, size)
			for index := range values {
				values[index] = index
			}
			predicate, err := factory.New().In(fields.number, values).Predicate()
			if err != nil {
				b.Fatalf("Predicate() error = %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			var condition memory.Condition[benchmarkRecord]
			for range b.N {
				condition, err = factory.Compile(predicate)
				if err != nil {
					b.Fatal(err)
				}
			}
			benchmarkConditionSink = condition
		})
	}
}

func BenchmarkCompileRepeatedPredicate(b *testing.B) {
	fields := newBenchmarkFields(b)
	factory := memory.NewFactory[benchmarkRecord]()
	predicate := newTypicalBenchmarkPredicate(b, factory, fields)

	b.ReportAllocs()
	b.ResetTimer()
	var condition memory.Condition[benchmarkRecord]
	var err error
	for range b.N {
		condition, err = factory.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkConditionSink = condition
}

func newBenchmarkFields(t testing.TB) benchmarkFields {
	t.Helper()
	number := mustField(
		t,
		"number",
		func(record benchmarkRecord) (int, memory.State) {
			return record.number, memory.StateValue
		},
		memory.OrderedSemantics[int](),
	)
	text := mustField(
		t,
		"text",
		func(record benchmarkRecord) (string, memory.State) {
			return record.text, memory.StateValue
		},
		memory.StringSemantics(),
	)
	optional := mustField(
		t,
		"optional",
		func(record benchmarkRecord) (int, memory.State) {
			if !record.optionalPresent {
				return 0, memory.StateMissing
			}
			return record.optional, memory.StateValue
		},
		memory.OrderedSemantics[int](),
	)
	return benchmarkFields{number: number, text: text, optional: optional}
}

func compileBenchmarkCondition(
	t testing.TB,
	add func(*weave.Builder[
		memory.Condition[benchmarkRecord],
		memory.Expression[benchmarkRecord],
	]),
) memory.Condition[benchmarkRecord] {
	t.Helper()
	builder := memory.NewFactory[benchmarkRecord]().New()
	add(builder)
	condition, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return condition
}

func newTypicalBenchmarkPredicate(
	t testing.TB,
	factory *memory.Factory[benchmarkRecord],
	fields benchmarkFields,
) weave.Predicate[
	memory.Condition[benchmarkRecord],
	memory.Expression[benchmarkRecord],
] {
	t.Helper()
	builder := factory.New()
	addTypicalBenchmarkNodes(builder, fields)
	predicate, err := builder.Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	return predicate
}

func addTypicalBenchmarkNodes(
	builder *weave.Builder[
		memory.Condition[benchmarkRecord],
		memory.Expression[benchmarkRecord],
	],
	fields benchmarkFields,
) {
	builder.GTE(fields.number, 10).
		LTE(fields.number, 900).
		AllOf(func(group *weave.Group[memory.Expression[benchmarkRecord]]) {
			group.AnyOf(func(nested *weave.Group[memory.Expression[benchmarkRecord]]) {
				nested.Contains(fields.text, "needle").
					HasPrefix(fields.text, "prefix-").
					In(fields.number, []int{21, 42, 84, 168})
			}).NoneOf(func(nested *weave.Group[memory.Expression[benchmarkRecord]]) {
				nested.EQ(fields.number, -1).
					IsNull(fields.optional)
			}).NotAllOf(func(nested *weave.Group[memory.Expression[benchmarkRecord]]) {
				nested.LT(fields.optional, 2).
					HasSuffix(fields.text, "disabled")
			})
		})
}
