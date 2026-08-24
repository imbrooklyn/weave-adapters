package gormgen

import (
	"fmt"

	"github.com/imbrooklyn/weave"
)

// Option configures immutable Compiler field metadata. Option is sealed so
// only options provided by this package can be used.
type Option interface {
	apply(*compilerOptions) error
	gormgenOption()
}

type fieldSpecsOption struct {
	specs []FieldSpec
}

func (fieldSpecsOption) gormgenOption() {}

func (option fieldSpecsOption) apply(configuration *compilerOptions) error {
	for _, spec := range option.specs {
		if !spec.valid() {
			return fmt.Errorf(
				"gormgen: invalid FieldSpec option: %w",
				weave.ErrInvalidField,
			)
		}
		identity := spec.identity()
		if existing, ok := configuration.fields[identity]; ok {
			if !equivalentFieldSpecs(existing, spec) {
				return fmt.Errorf(
					"gormgen: conflicting FieldSpecs identify the same column: %w",
					weave.ErrInvalidField,
				)
			}
			continue
		}
		configuration.fields[identity] = spec
	}
	return nil
}

type registeredFieldsOnlyOption struct{}

func (registeredFieldsOnlyOption) gormgenOption() {}

func (registeredFieldsOnlyOption) apply(configuration *compilerOptions) error {
	configuration.registeredOnly = true
	return nil
}

// WithFieldSpecs installs immutable field metadata. The input slice is
// shallow-cloned when this function is called.
func WithFieldSpecs(specs ...FieldSpec) Option {
	return fieldSpecsOption{specs: append([]FieldSpec(nil), specs...)}
}

// WithRegisteredFieldsOnly requires every standard field to match an installed
// FieldSpec. NewCompiler rejects this option when no FieldSpec is installed.
func WithRegisteredFieldsOnly() Option {
	return registeredFieldsOnlyOption{}
}
