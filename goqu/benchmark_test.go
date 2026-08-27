package goqu

import (
	"strconv"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/imbrooklyn/weave"
)

const benchmarkTable = "weave_goqu_benchmark_records"

var (
	benchmarkExpressionsSink Expressions
	benchmarkSQLSink         string
	benchmarkArgumentsSink   []any
	benchmarkNumberField     = mustBenchmarkField[int64]("number_value")
	benchmarkTextField       = mustBenchmarkField[string]("text_value")
)

func BenchmarkExpressionEmit(b *testing.B) {
	plan := benchmarkPlan(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		expressions, err := emitPredicate(plan)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkExpressionsSink = expressions
	}
}

func BenchmarkPreparedSQL(b *testing.B) {
	for _, profile := range []Profile{MySQL, PostgreSQL} {
		b.Run(profile.String(), func(b *testing.B) {
			compiler, predicate := benchmarkPredicate(b, profile, benchmarkFiveLeaves)
			expressions, err := compiler.Compile(predicate)
			if err != nil {
				b.Fatal(err)
			}
			dataset := sqlbuilder.
				Dialect(profile.dialectName()).
				From(sqlbuilder.T(benchmarkTable)).
				Select(sqlbuilder.T(benchmarkTable).Col("id")).
				Where(expressions...).
				Prepared(true)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				query, arguments, err := dataset.ToSQL()
				if err != nil {
					b.Fatal(err)
				}
				benchmarkSQLSink = query
				benchmarkArgumentsSink = arguments
			}
		})
	}
}

func BenchmarkCompileMembership(b *testing.B) {
	for _, count := range []int{100, 1000} {
		values := benchmarkInt64Values(count)
		b.Run("in_"+strconv.Itoa(count), func(b *testing.B) {
			compiler, predicate := benchmarkPredicate(
				b,
				MySQL,
				func(builder *weave.Builder[Expressions, Expression]) {
					builder.In(benchmarkNumberField, values)
				},
			)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				expressions, err := compiler.Compile(predicate)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkExpressionsSink = expressions
			}
		})
	}
}

func BenchmarkCompileRepeated(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, MySQL, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		expressions, err := compiler.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkExpressionsSink = expressions
	}
}

func BenchmarkCompileConcurrent(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, MySQL, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		var last Expressions
		ran := false
		for parallel.Next() {
			ran = true
			expressions, err := compiler.Compile(predicate)
			if err != nil {
				b.Error(err)
				continue
			}
			last = expressions
		}
		if ran && last == nil {
			b.Error("parallel Compile returned nil Expressions")
		}
	})
}

func benchmarkPlan(
	b testing.TB,
	build func(*weave.Builder[Expressions, Expression]),
) validatedNode {
	b.Helper()
	compiler, predicate := benchmarkPredicate(b, MySQL, build)
	plan, err := compiler.validatePredicate(predicate)
	if err != nil {
		b.Fatal(err)
	}
	return plan
}

func benchmarkPredicate(
	b testing.TB,
	profile Profile,
	build func(*weave.Builder[Expressions, Expression]),
) (Compiler, weave.Predicate[Expressions, Expression]) {
	b.Helper()
	compiler, err := NewCompiler(profile)
	if err != nil {
		b.Fatal(err)
	}
	factory := weave.NewFactory[Expressions, Expression](compiler)
	builder := factory.New()
	build(builder)
	predicate, err := builder.Predicate()
	if err != nil {
		b.Fatal(err)
	}
	return compiler, predicate
}

func benchmarkFiveLeaves(builder *weave.Builder[Expressions, Expression]) {
	builder.EQ(benchmarkNumberField, int64(2)).
		GTE(benchmarkNumberField, int64(1)).
		Contains(benchmarkTextField, "prefix %_!").
		NotNull(benchmarkTextField).
		In(benchmarkNumberField, []int64{1, 2, 3})
}

func mustBenchmarkField[T any](column string) Field[T] {
	field, err := NewField[T](sqlbuilder.T(benchmarkTable).Col(column))
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
