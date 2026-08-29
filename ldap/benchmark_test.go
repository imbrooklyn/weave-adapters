package ldap

import (
	"sync/atomic"
	"testing"

	"github.com/imbrooklyn/weave"
)

var (
	benchmarkFilter Filter
	benchmarkText   string
)

func BenchmarkFilterEmit(b *testing.B) {
	fixture := newLDAPFixture(b)
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		b.Fatal(err)
	}
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		b.Fatal(err)
	}
	predicate, err := factory.New().
		EQ(fixture.cn, "literal *()\\ 世界").
		Between(fixture.uid, int64(10), int64(20)).
		AnyOf(func(group *Group) {
			group.HasPrefix(fixture.cn, "prefix").
				NotIn(fixture.uid, []int64{3, 5, 8})
		}).
		Predicate()
	if err != nil {
		b.Fatal(err)
	}
	validated, err := compiler.validatePredicate(predicate)
	if err != nil {
		b.Fatal(err)
	}
	want, err := emitPredicate(validated)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for range b.N {
		got, emitErr := emitPredicate(validated)
		if emitErr != nil {
			b.Fatal(emitErr)
		}
		benchmarkText = got
	}
	if benchmarkText != want {
		b.Fatal("emitter output changed during benchmark")
	}
}

func BenchmarkCompileLargeIn(b *testing.B) {
	compiler, predicate, want := largeInBenchmarkInput(b, 1024)
	b.ReportAllocs()
	b.ReportMetric(1024, "values/op")
	b.SetBytes(int64(len(want.String())))
	b.ResetTimer()
	for range b.N {
		got, err := compiler.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFilter = got
	}
	if benchmarkFilter.String() != want.String() {
		b.Fatal("large In output changed during benchmark")
	}
}

func BenchmarkCompileDeepLogic(b *testing.B) {
	fixture := newLDAPFixture(b)
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		b.Fatal(err)
	}
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		b.Fatal(err)
	}
	predicate, err := factory.New().
		AllOf(deepBenchmarkScope(fixture.uid, weave.MaxPredicateDepth-2)).
		Predicate()
	if err != nil {
		b.Fatal(err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ReportMetric(weave.MaxPredicateDepth, "depth/op")
	b.SetBytes(int64(len(want.String())))
	b.ResetTimer()
	for range b.N {
		got, compileErr := compiler.Compile(predicate)
		if compileErr != nil {
			b.Fatal(compileErr)
		}
		benchmarkFilter = got
	}
	if benchmarkFilter.String() != want.String() {
		b.Fatal("deep Logic output changed during benchmark")
	}
}

func BenchmarkCompileRepeated(b *testing.B) {
	compiler, predicate, want := repeatedBenchmarkInput(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(want.String())))
	b.ResetTimer()
	for range b.N {
		got, err := compiler.Compile(predicate)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFilter = got
	}
	if benchmarkFilter.String() != want.String() {
		b.Fatal("repeated Compile output changed during benchmark")
	}
}

func BenchmarkCompileConcurrent(b *testing.B) {
	compiler, predicate, want := repeatedBenchmarkInput(b)
	var failed atomic.Bool
	b.ReportAllocs()
	b.SetBytes(int64(len(want.String())))
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			got, err := compiler.Compile(predicate)
			if err != nil || got.String() != want.String() {
				failed.Store(true)
			}
		}
	})
	if failed.Load() {
		b.Fatal("concurrent Compile returned an error or changed its output")
	}
}

func largeInBenchmarkInput(
	testingObject testing.TB,
	count int,
) (Compiler, weave.Predicate[Filter, Expression], Filter) {
	testingObject.Helper()
	fixture := newLDAPFixture(testingObject)
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		testingObject.Fatal(err)
	}
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		testingObject.Fatal(err)
	}
	values := make([]int64, count)
	for index := range values {
		values[index] = int64(index)
	}
	predicate, err := factory.New().In(fixture.uid, values).Predicate()
	if err != nil {
		testingObject.Fatal(err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		testingObject.Fatal(err)
	}
	return compiler, predicate, want
}

func repeatedBenchmarkInput(
	testingObject testing.TB,
) (Compiler, weave.Predicate[Filter, Expression], Filter) {
	testingObject.Helper()
	fixture := newLDAPFixture(testingObject)
	compiler, err := NewCompiler(RFC4515, fixture.schema)
	if err != nil {
		testingObject.Fatal(err)
	}
	factory, err := NewFactory(RFC4515, fixture.schema)
	if err != nil {
		testingObject.Fatal(err)
	}
	predicate, err := factory.New().
		EQ(fixture.cn, "Alice").
		Between(fixture.uid, int64(10), int64(20)).
		AnyOf(func(group *Group) {
			group.Contains(fixture.cn, "lic").
				In(fixture.uid, []int64{11, 13, 17, 19})
		}).
		Predicate()
	if err != nil {
		testingObject.Fatal(err)
	}
	want, err := compiler.Compile(predicate)
	if err != nil {
		testingObject.Fatal(err)
	}
	return compiler, predicate, want
}

func deepBenchmarkScope(
	attribute Attribute[int64],
	groupsBelow int,
) Scope {
	return func(group *Group) {
		if groupsBelow == 0 {
			group.EQ(attribute, int64(42))
			return
		}
		next := deepBenchmarkScope(attribute, groupsBelow-1)
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
