// Package usage is a compile fixture for the traditional and generic GORM
// query entry points supported by the native Weave Adapter type boundary.
package usage

import (
	"context"

	weavegorm "github.com/imbrooklyn/weave-adapters/gorm"
	"gorm.io/gorm"
)

// Record is the fixture query model.
type Record struct {
	ID   int64
	Name string
}

// TableName fixes the identifier used by DryRun seam tests.
func (Record) TableName() string {
	return "weave_gorm_records"
}

// Traditional applies one Adapter Condition through DB.Where.
func Traditional(database *gorm.DB, condition weavegorm.Condition) *gorm.DB {
	return database.Where(condition).Find(&[]Record{})
}

// Generics applies the same Adapter Condition through GORM's generic Where
// chain.
func Generics(
	ctx context.Context,
	database *gorm.DB,
	condition weavegorm.Condition,
) ([]Record, error) {
	return gorm.G[Record](database).Where(condition).Find(ctx)
}

// CompiledTraditional constructs and compiles a typed predicate, then applies
// its single native Condition through DB.Where.
func CompiledTraditional(
	database *gorm.DB,
	profile weavegorm.Profile,
) (*gorm.DB, error) {
	factory, err := weavegorm.NewFactory(profile)
	if err != nil {
		return nil, err
	}
	condition, err := factory.New().EQ(
		weavegorm.MustQualifiedField[string](
			"weave_gorm_records",
			"name",
		),
		"alice",
	).Build()
	if err != nil {
		return nil, err
	}
	return Traditional(database, condition), nil
}

// CompiledGenerics constructs and compiles the same typed predicate, then
// applies its Condition through GORM's generic Where chain.
func CompiledGenerics(
	ctx context.Context,
	database *gorm.DB,
	profile weavegorm.Profile,
) ([]Record, error) {
	factory, err := weavegorm.NewFactory(profile)
	if err != nil {
		return nil, err
	}
	condition, err := factory.New().EQ(
		weavegorm.MustQualifiedField[string](
			"weave_gorm_records",
			"name",
		),
		"alice",
	).Build()
	if err != nil {
		return nil, err
	}
	return Generics(ctx, database, condition)
}
