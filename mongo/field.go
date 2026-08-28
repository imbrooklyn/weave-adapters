package mongo

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
)

// Field is an immutable typed declaration of one canonical MongoDB field path
// and the standard operators applicable to it. Field values describe query
// types and applicability; they are not an authorization system. Valid values
// contain no caller-owned mutable state, can be copied, and are safe for
// concurrent capability discovery and compilation. Standard Field semantics
// describe one single-valued BSON field, not MongoDB multikey array matching.
type Field[T any] struct {
	seal      *fieldSeal
	path      string
	valueType reflect.Type
	operators weave.OperatorSet
	unsafe    bool
}

type fieldSeal struct{}

var validFieldSeal = &fieldSeal{}

type fieldMetadata struct {
	seal           *fieldSeal
	descriptorType reflect.Type
	path           string
	valueType      reflect.Type
	operators      weave.OperatorSet
	unsafe         bool
}

type fieldDescriptor interface {
	mongoFieldMetadata() fieldMetadata
}

// NewField returns a typed descriptor with conservative operators inferred
// from T. path must contain dot-separated ordinary field-name segments. Each
// segment starts with a Unicode letter or underscore and continues with only
// letters, digits, or underscores. Operator and positional fragments are not
// accepted.
func NewField[T any](path string) (Field[T], error) {
	return newField[T](path, nil, false, false)
}

// NewFieldWithOperators returns a safe typed descriptor whose non-empty
// operator list is the complete replacement set. The constructor rejects
// unknown operators, Between for non-numeric T, and literal-text operators for
// non-string T.
func NewFieldWithOperators[T any](
	path string,
	operators ...weave.Operator,
) (Field[T], error) {
	return newField[T](path, operators, true, false)
}

// UnsafeField returns a descriptor for a trusted schema path that needs
// characters outside NewField's conservative segment grammar. It must never be
// constructed from untrusted request input. Despite its name, it still rejects
// invalid UTF-8, NUL/control characters, empty segments, surrounding
// whitespace, and segments beginning with '$'. An invalid path or operator set
// returns the zero Field, which compilation rejects. An empty operator list
// uses the normal type-based defaults; a non-empty list replaces them exactly.
func UnsafeField[T any](path string, operators ...weave.Operator) Field[T] {
	field, err := newField[T](path, operators, len(operators) != 0, true)
	if err != nil {
		return Field[T]{}
	}
	return field
}

// Path returns the canonical dot-separated path. A zero or otherwise invalid
// Field returns the empty string.
func (field Field[T]) Path() string {
	if !field.valid() {
		return ""
	}
	return field.path
}

// Capabilities returns this descriptor's exact immutable standard-operator
// set. A zero or otherwise invalid Field returns zero capabilities.
func (field Field[T]) Capabilities() weave.FieldCapabilities {
	if !field.valid() {
		return weave.FieldCapabilities{}
	}
	return weave.FieldCapabilities{Operators: field.operators}
}

func newField[T any](
	path string,
	operators []weave.Operator,
	explicit bool,
	unsafe bool,
) (Field[T], error) {
	if !validFieldPath(path, unsafe) {
		return Field[T]{}, invalidFieldDefinitionError()
	}

	valueType := reflect.TypeFor[T]()
	operatorSet, err := fieldOperatorSet(valueType, operators, explicit)
	if err != nil {
		return Field[T]{}, err
	}

	return Field[T]{
		seal:      validFieldSeal,
		path:      path,
		valueType: valueType,
		operators: operatorSet,
		unsafe:    unsafe,
	}, nil
}

func (field Field[T]) mongoFieldMetadata() fieldMetadata {
	return fieldMetadata{
		seal:           field.seal,
		descriptorType: reflect.TypeFor[Field[T]](),
		path:           field.path,
		valueType:      field.valueType,
		operators:      field.operators,
		unsafe:         field.unsafe,
	}
}

func (field Field[T]) valid() bool {
	return field.seal == validFieldSeal &&
		field.valueType == reflect.TypeFor[T]() &&
		validDeclaredOperators(field.valueType, field.operators) &&
		validFieldPath(field.path, field.unsafe)
}

func (metadata fieldMetadata) validFor(actualType reflect.Type) bool {
	return metadata.seal == validFieldSeal &&
		metadata.descriptorType != nil &&
		actualType == metadata.descriptorType &&
		metadata.valueType != nil &&
		validDeclaredOperators(metadata.valueType, metadata.operators) &&
		validFieldPath(metadata.path, metadata.unsafe)
}

func validFieldPath(path string, unsafe bool) bool {
	if path == "" || !utf8.ValidString(path) || strings.TrimSpace(path) != path {
		return false
	}
	for _, segment := range strings.Split(path, ".") {
		if !validFieldSegment(segment, unsafe) {
			return false
		}
	}
	return true
}

func validFieldSegment(segment string, unsafe bool) bool {
	if segment == "" || segment[0] == '$' || strings.TrimSpace(segment) != segment {
		return false
	}
	for index, character := range segment {
		if character == 0 || unicode.IsControl(character) {
			return false
		}
		if unsafe {
			continue
		}
		if index == 0 {
			if character != '_' && !unicode.IsLetter(character) {
				return false
			}
			continue
		}
		if character != '_' && !unicode.IsLetter(character) &&
			!unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func invalidFieldDefinitionError() error {
	return fmt.Errorf(
		"mongo: fields require a canonical ordinary field path: %w",
		weave.ErrInvalidField,
	)
}

func fieldOperatorSet(
	valueType reflect.Type,
	operators []weave.Operator,
	explicit bool,
) (weave.OperatorSet, error) {
	if !explicit {
		return inferredFieldOperators(valueType), nil
	}
	if len(operators) == 0 {
		return weave.OperatorSet{}, fmt.Errorf(
			"mongo: NewFieldWithOperators requires at least one operator: %w",
			weave.ErrInvalidValue,
		)
	}
	for _, operator := range operators {
		if !standardOperator(operator) {
			return weave.OperatorSet{}, fmt.Errorf(
				"mongo: Field contains an invalid operator: %w",
				weave.ErrInvalidValue,
			)
		}
		if operator == weave.OperatorBetween && !numericType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"mongo: Between requires a numeric Field type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
		if textOperator(operator) && !stringType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"mongo: literal-text operators require a string Field type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
	}
	return weave.NewOperatorSet(operators...), nil
}

func inferredFieldOperators(valueType reflect.Type) weave.OperatorSet {
	valueOperators := []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	}
	orderOperators := []weave.Operator{
		weave.OperatorLT,
		weave.OperatorLTE,
		weave.OperatorGT,
		weave.OperatorGTE,
	}

	switch {
	case numericType(valueType):
		return weave.NewOperatorSet(append(
			append(valueOperators, orderOperators...),
			weave.OperatorBetween,
		)...)
	case stringType(valueType):
		return weave.NewOperatorSet(append(
			append(valueOperators, orderOperators...),
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		)...)
	case valueType == reflect.TypeFor[time.Time]():
		return weave.NewOperatorSet(append(valueOperators, orderOperators...)...)
	default:
		return weave.NewOperatorSet(valueOperators...)
	}
}

func numericType(valueType reflect.Type) bool {
	if valueType == nil {
		return false
	}
	switch valueType.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64:
		return true
	default:
		return false
	}
}

func stringType(valueType reflect.Type) bool {
	return valueType != nil && valueType.Kind() == reflect.String
}

func validDeclaredOperators(
	valueType reflect.Type,
	operators weave.OperatorSet,
) bool {
	if valueType == nil || operators.Count() == 0 {
		return false
	}
	recognized := 0
	for _, operator := range standardOperators() {
		if operators.Has(operator) {
			recognized++
		}
	}
	if recognized != operators.Count() {
		return false
	}
	if operators.Has(weave.OperatorBetween) && !numericType(valueType) {
		return false
	}
	for _, operator := range []weave.Operator{
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	} {
		if operators.Has(operator) && !stringType(valueType) {
			return false
		}
	}
	return true
}

func standardOperators() []weave.Operator {
	return []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorLT,
		weave.OperatorLTE,
		weave.OperatorGT,
		weave.OperatorGTE,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorBetween,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	}
}

func standardOperator(operator weave.Operator) bool {
	for _, candidate := range standardOperators() {
		if candidate == operator {
			return true
		}
	}
	return false
}

func textOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix:
		return true
	default:
		return false
	}
}
