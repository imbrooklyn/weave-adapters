// Package usage keeps a compile-checked generated DAO application path.
package usage

import (
	"github.com/imbrooklyn/weave-adapters/gormgen"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
	"gorm.io/gorm"
)

// FindUsers applies compiled conditions through the generated DAO Where
// signature. Tests compile this package as part of the module.
func FindUsers(
	database *gorm.DB,
	conditions gormgen.Conditions,
) ([]*model.User, error) {
	return query.Use(database).User.Where(conditions...).Find()
}

// FindUsersByName compile-checks the complete generated-field, Weave Builder,
// gormgen Compiler, and generated DAO Where path.
func FindUsersByName(
	database *gorm.DB,
	profile gormgen.Profile,
	name string,
) ([]*model.User, error) {
	queries := query.Use(database)
	factory, err := gormgen.NewFactory(profile)
	if err != nil {
		return nil, err
	}
	conditions, err := factory.New().EQ(queries.User.Name, name).Build()
	if err != nil {
		return nil, err
	}
	return queries.User.Where(conditions...).Find()
}
