package elasticsearch

import (
	"bytes"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/imbrooklyn/weave"
)

var (
	benchmarkElasticsearchQuery Query
	benchmarkElasticsearchJSON  []byte
)

func BenchmarkQueryEmit(b *testing.B) {
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	predicate := benchmarkPredicate(b, fixture)
	validated, err := fixture.compiler.validatePredicate(predicate)
	if err != nil {
		b.Fatal(err)
	}
	wantQuery, err := emitPredicate(validated)
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, wantQuery)

	b.ReportAllocs()
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		query, emitErr := emitPredicate(validated)
		if emitErr != nil {
			b.Fatal(emitErr)
		}
		benchmarkElasticsearchQuery = query
	}
	b.StopTimer()
	if got := marshalBenchmarkQuery(b, benchmarkElasticsearchQuery); !bytes.Equal(got, want) {
		b.Fatal("emitter output changed during benchmark")
	}
}

func BenchmarkQueryMarshal(b *testing.B) {
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	query, err := fixture.compiler.Compile(benchmarkPredicate(b, fixture))
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, query)

	b.ReportAllocs()
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		encoded, marshalErr := json.Marshal(query)
		if marshalErr != nil {
			b.Fatal(marshalErr)
		}
		benchmarkElasticsearchJSON = encoded
	}
	b.StopTimer()
	if !bytes.Equal(benchmarkElasticsearchJSON, want) {
		b.Fatal("marshal output changed during benchmark")
	}
}

func BenchmarkCompileLargeTerms(b *testing.B) {
	const valueCount = 1024
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	values := make([]int64, valueCount)
	for index := range values {
		values[index] = int64(index)
	}
	predicate, err := fixture.factory.New().In(fixture.integer, values).Predicate()
	if err != nil {
		b.Fatal(err)
	}
	wantQuery, err := fixture.compiler.Compile(predicate)
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, wantQuery)

	b.ReportAllocs()
	b.ReportMetric(valueCount, "values/op")
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		query, compileErr := fixture.compiler.Compile(predicate)
		if compileErr != nil {
			b.Fatal(compileErr)
		}
		benchmarkElasticsearchQuery = query
	}
	b.StopTimer()
	if got := marshalBenchmarkQuery(b, benchmarkElasticsearchQuery); !bytes.Equal(got, want) {
		b.Fatal("large terms output changed during benchmark")
	}
}

func BenchmarkCompileDeepBool(b *testing.B) {
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	predicate, err := fixture.factory.New().
		AllOf(deepElasticsearchBenchmarkScope(
			fixture.integer, weave.MaxPredicateDepth-2,
		)).
		Predicate()
	if err != nil {
		b.Fatal(err)
	}
	wantQuery, err := fixture.compiler.Compile(predicate)
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, wantQuery)

	b.ReportAllocs()
	b.ReportMetric(weave.MaxPredicateDepth, "depth/op")
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		query, compileErr := fixture.compiler.Compile(predicate)
		if compileErr != nil {
			b.Fatal(compileErr)
		}
		benchmarkElasticsearchQuery = query
	}
	b.StopTimer()
	if got := marshalBenchmarkQuery(b, benchmarkElasticsearchQuery); !bytes.Equal(got, want) {
		b.Fatal("deep bool output changed during benchmark")
	}
}

func BenchmarkCompileRepeated(b *testing.B) {
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	predicate := benchmarkPredicate(b, fixture)
	wantQuery, err := fixture.compiler.Compile(predicate)
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, wantQuery)

	b.ReportAllocs()
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		query, compileErr := fixture.compiler.Compile(predicate)
		if compileErr != nil {
			b.Fatal(compileErr)
		}
		benchmarkElasticsearchQuery = query
	}
	b.StopTimer()
	if got := marshalBenchmarkQuery(b, benchmarkElasticsearchQuery); !bytes.Equal(got, want) {
		b.Fatal("repeated Compile output changed during benchmark")
	}
}

func BenchmarkCompileConcurrent(b *testing.B) {
	fixture := newCompilerFixture(b, Elasticsearch95ExpensiveQueries)
	predicate := benchmarkPredicate(b, fixture)
	wantQuery, err := fixture.compiler.Compile(predicate)
	if err != nil {
		b.Fatal(err)
	}
	want := marshalBenchmarkQuery(b, wantQuery)
	var failed atomic.Bool

	b.ReportAllocs()
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		var last Query
		for parallel.Next() {
			query, compileErr := fixture.compiler.Compile(predicate)
			if compileErr != nil {
				failed.Store(true)
				return
			}
			last = query
		}
		if last != nil {
			encoded, marshalErr := json.Marshal(last)
			if marshalErr != nil || !bytes.Equal(encoded, want) {
				failed.Store(true)
			}
		}
	})
	if failed.Load() {
		b.Fatal("concurrent Compile returned an error or changed its output")
	}
}

func benchmarkPredicate(
	testingObject testing.TB,
	fixture compilerFixture,
) weave.Predicate[Query, Expression] {
	testingObject.Helper()
	predicate, err := fixture.factory.New().
		EQ(fixture.keyword, "stable").
		Between(fixture.decimal, 1.25, 9.5).
		AnyOf(func(group *Group) {
			group.Contains(fixture.wildcard, `literal *?\ 世界`).
				NotIn(fixture.integer, []int64{3, 5, 8})
		}).
		Predicate()
	if err != nil {
		testingObject.Fatal(err)
	}
	return predicate
}

func deepElasticsearchBenchmarkScope(
	field Field[int64],
	groupsBelow int,
) Scope {
	return func(group *Group) {
		if groupsBelow == 0 {
			group.EQ(field, int64(42))
			return
		}
		next := deepElasticsearchBenchmarkScope(field, groupsBelow-1)
		switch groupsBelow % 4 {
		case 0:
			group.AllOf(next)
		case 1:
			group.AnyOf(next)
		case 2:
			group.NoneOf(next)
		case 3:
			group.NotAllOf(next)
		}
	}
}

func marshalBenchmarkQuery(testingObject testing.TB, query Query) []byte {
	testingObject.Helper()
	encoded, err := json.Marshal(query)
	if err != nil {
		testingObject.Fatal(err)
	}
	return encoded
}
