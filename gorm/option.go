package gorm

import (
	"fmt"

	"github.com/imbrooklyn/weave"
)

// FieldOption configures immutable typed-field metadata. FieldOption is
// sealed so only options provided by this package can be used.
type FieldOption interface {
	apply(*fieldOptions) error
	gormFieldOption()
}

type fieldOptions struct {
	operators    []weave.Operator
	hasOperators bool
}

type operatorsOption struct {
	operators []weave.Operator
}

func (operatorsOption) gormFieldOption() {}

func (option operatorsOption) apply(configuration *fieldOptions) error {
	if configuration.hasOperators {
		return fmt.Errorf(
			"gorm: WithOperators may be applied only once: %w",
			weave.ErrInvalidValue,
		)
	}
	configuration.hasOperators = true
	configuration.operators = append([]weave.Operator(nil), option.operators...)
	return nil
}

// WithOperators declares the complete standard-operator set for a Field. It
// is not additive, and the list must be non-empty. The field constructor
// rejects unknown operators and declarations that conflict with T's fixed
// literal-text or Between type requirements.
func WithOperators(operators ...weave.Operator) FieldOption {
	return operatorsOption{
		operators: append([]weave.Operator(nil), operators...),
	}
}
