package mongo

import (
	"strconv"
	"testing"

	"github.com/imbrooklyn/weave"
)

var (
	benchmarkFilterSink  Filter
	benchmarkNumberField = mustBenchmarkField[int64]("record.number_value")
	benchmarkTextField   = mustBenchmarkField[string]("record.text_value")
)

func BenchmarkBSONEmit(b *testing.B) {
	plan := benchmarkPlan(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		filter, err := emitPredicate(plan)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFilterSink = filter
	}
}

func BenchmarkCompileMembership(b *testing.B) {
	for _, count := range []int{100, 1000} {
		values := benchmarkInt64Values(count)
		b.Run("in_"+strconv.Itoa(count), func(b *testing.B) {
			compiler, predicate := benchmarkPredicate(
				b,
				func(builder *weave.Builder[Filter, Expression]) {
					builder.In(benchmarkNumberField, values)
				},
			)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				filter, err := compiler.Compile(predicate)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkFilterSink = filter
			}
		})
	}
}

func BenchmarkCompileDeepLogic(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, func(
		builder *weave.Builder[Filter, Expression],
	) {
		builder.AllOf(benchmarkNestedLogic(weave.MaxPredicateDepth - 2))
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		filter, err := compiler.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFilterSink = filter
	}
}

func BenchmarkCompileRepeated(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		filter, err := compiler.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFilterSink = filter
	}
}

func BenchmarkCompileConcurrent(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		var last Filter
		ran := false
		for parallel.Next() {
			ran = true
			filter, err := compiler.Compile(predicate)
			if err != nil {
				b.Error(err)
				continue
			}
			last = filter
		}
		if ran && last == nil {
			b.Error("parallel Compile returned a nil Filter")
		}
	})
}

func benchmarkPlan(
	b testing.TB,
	build func(*weave.Builder[Filter, Expression]),
) validatedNode {
	b.Helper()
	compiler, predicate := benchmarkPredicate(b, build)
	plan, err := compiler.validatePredicate(predicate)
	if err != nil {
		b.Fatal(err)
	}
	return plan
}

func benchmarkPredicate(
	b testing.TB,
	build func(*weave.Builder[Filter, Expression]),
) (Compiler, weave.Predicate[Filter, Expression]) {
	b.Helper()
	compiler, err := NewCompiler(MongoDB60Plus)
	if err != nil {
		b.Fatal(err)
	}
	factory := weave.NewFactory[Filter, Expression](compiler)
	builder := factory.New()
	build(builder)
	predicate, err := builder.Predicate()
	if err != nil {
		b.Fatal(err)
	}
	return compiler, predicate
}

func benchmarkFiveLeaves(builder *weave.Builder[Filter, Expression]) {
	builder.EQ(benchmarkNumberField, int64(2)).
		GTE(benchmarkNumberField, int64(1)).
		Contains(benchmarkTextField, `prefix.*[$or]`).
		NotNull(benchmarkTextField).
		In(benchmarkNumberField, []int64{1, 2, 3})
}

func benchmarkNestedLogic(remaining int) weave.Scope[Expression] {
	return func(group *Group) {
		if remaining == 0 {
			group.EQ(benchmarkNumberField, int64(2))
			return
		}
		next := benchmarkNestedLogic(remaining - 1)
		switch remaining % 4 {
		case 0:
			group.AllOf(next)
		case 1:
			group.AnyOf(next)
		case 2:
			group.NoneOf(next)
		default:
			group.NotAllOf(next)
		}
	}
}

func mustBenchmarkField[T any](path string) Field[T] {
	field, err := NewField[T](path)
	if err != nil {
		panic(err)
	}
	return field
}

func benchmarkInt64Values(count int) []int64 {
	values := make([]int64, count)
	for index := range values {
		values[index] = int64(index)
	}
	return values
}
