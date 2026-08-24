package memory

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/imbrooklyn/weave"
)

// Field is an immutable typed field descriptor. Its accessor and semantic
// functions are borrowed; callers must keep their captured state deterministic
// and safe for the intended concurrency.
type Field[R, V any] struct {
	seal         *fieldSeal
	name         string
	accessor     Accessor[R, V]
	semantics    Semantics[V]
	recordType   reflect.Type
	valueType    reflect.Type
	capabilities weave.FieldCapabilities
}

type fieldSeal struct{}

var validFieldSeal = &fieldSeal{}

// NewField validates and returns a typed field descriptor. Name is a
// developer-facing label and is not included in default compile errors. Nil
// Semantics operations are valid and disable their corresponding operator
// families. Standard query values must be assignable to V; Compiler performs
// no conversion.
func NewField[R, V any](
	name string,
	accessor Accessor[R, V],
	semantics Semantics[V],
) (Field[R, V], error) {
	if strings.TrimSpace(name) == "" {
		return Field[R, V]{}, fmt.Errorf(
			"memory: field name must not be blank: %w",
			weave.ErrInvalidField,
		)
	}
	if accessor == nil {
		return Field[R, V]{}, fmt.Errorf(
			"memory: field accessor must not be nil: %w",
			weave.ErrInvalidField,
		)
	}

	return Field[R, V]{
		seal:         validFieldSeal,
		name:         name,
		accessor:     accessor,
		semantics:    semantics,
		recordType:   reflect.TypeFor[R](),
		valueType:    reflect.TypeFor[V](),
		capabilities: fieldCapabilities(semantics),
	}, nil
}

// Name returns the developer-facing field label supplied to NewField.
func (f Field[R, V]) Name() string {
	return f.name
}

// Capabilities returns the standard operators supported by the field's
// semantic function set. Null operators depend only on Accessor state and are
// included for every valid Field.
func (f Field[R, V]) Capabilities() weave.FieldCapabilities {
	return f.capabilities
}

type fieldDescriptorMetadata struct {
	recordType   reflect.Type
	valueType    reflect.Type
	capabilities weave.FieldCapabilities
	valid        bool
}

type fieldDescriptor interface {
	memoryFieldDescriptor() fieldDescriptorMetadata
}

func (f Field[R, V]) memoryFieldDescriptor() fieldDescriptorMetadata {
	return fieldDescriptorMetadata{
		recordType:   f.recordType,
		valueType:    f.valueType,
		capabilities: f.capabilities,
		valid:        f.valid(),
	}
}

func (f Field[R, V]) valid() bool {
	return f.seal == validFieldSeal &&
		strings.TrimSpace(f.name) != "" &&
		f.accessor != nil &&
		f.recordType == reflect.TypeFor[R]() &&
		f.valueType == reflect.TypeFor[V]() &&
		equalFieldCapabilities(f.capabilities, fieldCapabilities(f.semantics))
}

func equalFieldCapabilities(
	left weave.FieldCapabilities,
	right weave.FieldCapabilities,
) bool {
	return left.Operators.Count() == right.Operators.Count() &&
		left.Operators.ContainsAll(right.Operators)
}

func fieldCapabilities[V any](semantics Semantics[V]) weave.FieldCapabilities {
	operators := []weave.Operator{
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	}
	if semantics.equal != nil {
		operators = append(
			operators,
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
		)
	}
	if semantics.compare != nil {
		operators = append(
			operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorBetween,
		)
	}
	if semantics.text != nil {
		operators = append(
			operators,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		)
	}

	return weave.FieldCapabilities{
		Operators: weave.NewOperatorSet(operators...),
	}
}
