package gorm

import (
	"testing"

	"github.com/imbrooklyn/weave"
	"gorm.io/driver/mysql"
	upstream "gorm.io/gorm"
)

var (
	benchmarkConditionSink Condition
	benchmarkStatementSink *upstream.Statement
)

type benchmarkRecord struct {
	ID     int64  `gorm:"column:id"`
	Number int64  `gorm:"column:number_value"`
	Text   string `gorm:"column:text_value"`
}

func (benchmarkRecord) TableName() string {
	return "weave_gorm_benchmark_records"
}

func BenchmarkEmit(b *testing.B) {
	plan := benchmarkPlan(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		condition, err := emitPredicate(plan)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkConditionSink = condition
	}
}

func BenchmarkCompile(b *testing.B) {
	values100 := benchmarkInt64Values(100)
	values1000 := benchmarkInt64Values(1000)
	cases := []struct {
		name  string
		build func(*weave.Builder[Condition, Expression])
	}{
		{name: "five_leaves", build: benchmarkFiveLeaves},
		{
			name: "twenty_leaves",
			build: func(builder *weave.Builder[Condition, Expression]) {
				field := benchmarkNumberField()
				for index := range 20 {
					builder.EQ(field, int64(index%6+1))
				}
			},
		},
		{
			name: "three_level_group",
			build: func(builder *weave.Builder[Condition, Expression]) {
				field := benchmarkNumberField()
				builder.AllOf(func(levelOne *Group) {
					levelOne.AnyOf(func(levelTwo *Group) {
						levelTwo.NoneOf(func(levelThree *Group) {
							levelThree.EQ(field, int64(1)).EQ(field, int64(2))
						}).EQ(field, int64(6))
					}).GTE(field, int64(3))
				})
			},
		},
		{
			name: "in_100",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.In(benchmarkNumberField(), values100)
			},
		},
		{
			name: "in_1000",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.In(benchmarkNumberField(), values1000)
			},
		},
	}

	for _, test := range cases {
		b.Run(test.name, func(b *testing.B) {
			compiler, predicate := benchmarkPredicate(b, test.build)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				condition, err := compiler.Compile(predicate)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkConditionSink = condition
			}
		})
	}
}

func BenchmarkCompileParallel(b *testing.B) {
	compiler, predicate := benchmarkPredicate(b, benchmarkFiveLeaves)
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		var last Condition
		ran := false
		for parallel.Next() {
			ran = true
			condition, err := compiler.Compile(predicate)
			if err != nil {
				b.Error(err)
				continue
			}
			last = condition
		}
		if ran && last == nil {
			b.Error("parallel Compile returned no condition")
		}
	})
}

func BenchmarkDryRunBuild(b *testing.B) {
	values100 := benchmarkInt64Values(100)
	values1000 := benchmarkInt64Values(1000)
	cases := []struct {
		name  string
		build func(*weave.Builder[Condition, Expression])
	}{
		{name: "five_leaves", build: benchmarkFiveLeaves},
		{
			name: "in_100",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.In(benchmarkNumberField(), values100)
			},
		},
		{
			name: "in_1000",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.In(benchmarkNumberField(), values1000)
			},
		},
	}
	database, err := upstream.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
		SkipInitializeWithVersion: true,
	}), &upstream.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
	})
	if err != nil {
		b.Fatal(err)
	}

	for _, test := range cases {
		b.Run(test.name, func(b *testing.B) {
			factory, err := NewFactory(MySQL)
			if err != nil {
				b.Fatal(err)
			}
			builder := factory.New()
			test.build(builder)
			condition, err := builder.Build()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				statement := database.Session(&upstream.Session{NewDB: true}).
					Where(condition).
					Find(&[]benchmarkRecord{}).
					Statement
				if statement.Error != nil {
					b.Fatal(statement.Error)
				}
				benchmarkStatementSink = statement
			}
		})
	}
}

func benchmarkPlan(
	b testing.TB,
	build func(*weave.Builder[Condition, Expression]),
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
	build func(*weave.Builder[Condition, Expression]),
) (Compiler, weave.Predicate[Condition, Expression]) {
	b.Helper()
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		b.Fatal(err)
	}
	factory := weave.NewFactory[Condition, Expression](compiler)
	builder := factory.New()
	build(builder)
	predicate, err := builder.Predicate()
	if err != nil {
		b.Fatal(err)
	}
	return compiler, predicate
}

func benchmarkFiveLeaves(builder *weave.Builder[Condition, Expression]) {
	number := benchmarkNumberField()
	text := MustQualifiedField[string](
		"weave_gorm_benchmark_records",
		"text_value",
	)
	builder.EQ(number, int64(2)).
		GTE(number, int64(1)).
		Contains(text, "prefix %_!").
		NotNull(text).
		In(number, []int64{1, 2, 3})
}

func benchmarkNumberField() Field[int64] {
	return MustQualifiedField[int64](
		"weave_gorm_benchmark_records",
		"number_value",
	)
}

func benchmarkInt64Values(count int) []int64 {
	values := make([]int64, count)
	for index := range values {
		values[index] = int64(index)
	}
	return values
}
