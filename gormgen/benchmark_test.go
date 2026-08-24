package gormgen

import (
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
)

var (
	benchmarkMetadataSink   nativeFieldMetadata
	benchmarkConditionsSink Conditions
)

func BenchmarkGeneratedFieldMetadataLookup(b *testing.B) {
	fixture := newFixtureQuery(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		metadata, err := inspectNativeField(fixture.SemanticRecord.Text)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkMetadataSink = metadata
	}
}

func BenchmarkGeneratedFieldCapabilitiesLookup(b *testing.B) {
	fixture := newFixtureQuery(b)
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		capabilities, err := compiler.CapabilitiesFor(fixture.SemanticRecord.Text)
		if err != nil {
			b.Fatal(err)
		}
		if capabilities.Operators.Count() == 0 {
			b.Fatal("empty field capabilities")
		}
	}
}

func BenchmarkCompile(b *testing.B) {
	fixture := newFixtureQuery(b)
	compiler, err := NewCompiler(MySQL)
	if err != nil {
		b.Fatal(err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)
	values100 := benchmarkInt64Values(100)
	values1000 := benchmarkInt64Values(1000)
	nullable100 := benchmarkNullableInt64Values(100)

	cases := []struct {
		name  string
		build func(*weave.Builder[Conditions, Expression])
	}{
		{
			name: "five_leaves",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.EQ(fixture.SemanticRecord.Number, int64(2)).
					GTE(fixture.SemanticRecord.Number, int64(1)).
					Contains(fixture.SemanticRecord.Text, "prefix").
					NotNull(fixture.SemanticRecord.NullableText).
					In(fixture.SemanticRecord.Number, []int64{1, 2, 3})
			},
		},
		{
			name: "twenty_leaves",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				for index := range 20 {
					builder.EQ(
						fixture.SemanticRecord.Number,
						int64(index%6+1),
					)
				}
			},
		},
		{
			name: "three_level_group",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.AllOf(func(levelOne *Group) {
					levelOne.AnyOf(func(levelTwo *Group) {
						levelTwo.NoneOf(func(levelThree *Group) {
							levelThree.EQ(fixture.SemanticRecord.Number, int64(1)).
								EQ(fixture.SemanticRecord.Number, int64(2))
						}).EQ(fixture.SemanticRecord.Number, int64(6))
					}).GTE(fixture.SemanticRecord.Number, int64(3))
				})
			},
		},
		{
			name: "in_100",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.In(fixture.SemanticRecord.Number, values100)
			},
		},
		{
			name: "in_1000",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.In(fixture.SemanticRecord.Number, values1000)
			},
		},
		{
			name: "nullable_in_100",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.In(fixture.SemanticRecord.NullableNumber, nullable100)
			},
		},
		{
			name: "native",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.Native(ConditionsOf(
					fixture.SemanticRecord.ID.Eq("r02"),
				)).GTE(fixture.SemanticRecord.Number, int64(2))
			},
		},
		{
			name: "expr",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.AnyOf(func(group *Group) {
					group.Expr(fixture.SemanticRecord.ID.Eq("r02")).
						EQ(fixture.SemanticRecord.Number, int64(6))
				})
			},
		},
	}

	for _, test := range cases {
		b.Run(test.name, func(b *testing.B) {
			builder := factory.New()
			test.build(builder)
			predicate, err := builder.Predicate()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				conditions, err := compiler.Compile(predicate)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkConditionsSink = conditions
			}
		})
	}
}

func BenchmarkCompileParallel(b *testing.B) {
	fixture := newFixtureQuery(b)
	compiler, err := NewCompiler(PostgreSQL)
	if err != nil {
		b.Fatal(err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)
	predicate, err := factory.New().
		GTE(fixture.SemanticRecord.Number, int64(2)).
		AnyOf(func(group *Group) {
			group.Contains(fixture.SemanticRecord.Text, "prefix").
				In(fixture.SemanticRecord.Number, []int64{2, 6})
		}).
		Predicate()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if _, err := compiler.Compile(predicate); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkGeneratedDAODryRun(b *testing.B) {
	fixture := newFixtureQuery(b)
	factory, err := NewFactory(MySQL)
	if err != nil {
		b.Fatal(err)
	}
	conditions, err := factory.New().
		EQ(fixture.SemanticRecord.Number, int64(2)).
		Contains(fixture.SemanticRecord.Text, "prefix %_!").
		In(fixture.SemanticRecord.Number, []int64{1, 2, 3}).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	database := fixture.SemanticRecord.UnderlyingDB()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rows := make([]model.SemanticRecord, 0)
		records := query.Use(database).SemanticRecord
		statement := records.Where(conditions...).UnderlyingDB().Find(&rows)
		if statement.Error != nil {
			b.Fatal(statement.Error)
		}
	}
}

func benchmarkInt64Values(count int) []int64 {
	values := make([]int64, count)
	for index := range values {
		values[index] = int64(index)
	}
	return values
}

func benchmarkNullableInt64Values(count int) []*int64 {
	values := make([]*int64, 0, count+1)
	for index := range count {
		value := int64(index)
		values = append(values, &value)
	}
	return append(values, nil)
}
