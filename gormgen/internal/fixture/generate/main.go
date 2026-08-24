package main

import (
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"gorm.io/gen"
)

func main() {
	generator := gen.NewGenerator(gen.Config{
		OutPath: "internal/fixture/query",
		Mode: gen.WithoutContext |
			gen.WithDefaultQuery |
			gen.WithQueryInterface,
	})
	generator.ApplyBasic(model.User{}, model.SemanticRecord{})
	generator.Execute()
}
