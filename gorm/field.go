package gorm

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
	"gorm.io/gorm/clause"
)

// Field is an immutable typed declaration of one non-raw GORM column and the
// standard operators applicable to it. Field values describe database query
// types and applicability; they are not an authorization system.
type Field[T any] struct {
	seal      *fieldSeal
	column    clause.Column
	valueType reflect.Type
	operators weave.OperatorSet
}

type fieldSeal struct{}

var validFieldSeal = &fieldSeal{}

type fieldMetadata struct {
	seal           *fieldSeal
	descriptorType reflect.Type
	column         clause.Column
	valueType      reflect.Type
	operators      weave.OperatorSet
}

type fieldDescriptor interface {
	gormFieldMetadata() fieldMetadata
}

// NewField returns an unqualified typed column descriptor. name must be one
// identifier segment; dotted names must be expressed with NewQualifiedField.
func NewField[T any](
	name string,
	options ...FieldOption,
) (Field[T], error) {
	return newField[T]("", name, false, options)
}

// NewQualifiedField returns a typed descriptor with separate table and column
// identifier segments. Neither segment is interpreted as raw SQL.
func NewQualifiedField[T any](
	table string,
	name string,
	options ...FieldOption,
) (Field[T], error) {
	return newField[T](table, name, true, options)
}

// MustField is NewField for application-assembly declarations. It panics when
// name or an option is invalid.
func MustField[T any](name string, options ...FieldOption) Field[T] {
	field, err := NewField[T](name, options...)
	if err != nil {
		panic(err)
	}
	return field
}

// MustQualifiedField is NewQualifiedField for application-assembly
// declarations. It panics when either identifier segment or an option is
// invalid.
func MustQualifiedField[T any](
	table string,
	name string,
	options ...FieldOption,
) Field[T] {
	field, err := NewQualifiedField[T](table, name, options...)
	if err != nil {
		panic(err)
	}
	return field
}

// Capabilities returns this descriptor's immutable standard-operator set. A
// zero or otherwise invalid Field returns zero capabilities.
func (field Field[T]) Capabilities() weave.FieldCapabilities {
	if !field.valid() {
		return weave.FieldCapabilities{}
	}
	return weave.FieldCapabilities{Operators: field.operators}
}

func newField[T any](
	table string,
	name string,
	qualified bool,
	options []FieldOption,
) (Field[T], error) {
	if !validIdentifierSegment(name) ||
		(qualified && !validIdentifierSegment(table)) {
		return Field[T]{}, invalidFieldDefinitionError()
	}

	configuration := fieldOptions{}
	for _, option := range options {
		if isNilLike(option) {
			return Field[T]{}, fmt.Errorf(
				"gorm: nil field option: %w",
				weave.ErrInvalidValue,
			)
		}
		if err := option.apply(&configuration); err != nil {
			return Field[T]{}, err
		}
	}

	valueType := reflect.TypeFor[T]()
	operators, err := fieldOperatorSet(
		valueType,
		configuration.operators,
		configuration.hasOperators,
	)
	if err != nil {
		return Field[T]{}, err
	}

	return Field[T]{
		seal: validFieldSeal,
		column: clause.Column{
			Table: table,
			Name:  name,
		},
		valueType: valueType,
		operators: operators,
	}, nil
}

func (field Field[T]) gormFieldMetadata() fieldMetadata {
	return fieldMetadata{
		seal:           field.seal,
		descriptorType: reflect.TypeFor[Field[T]](),
		column:         field.column,
		valueType:      field.valueType,
		operators:      field.operators,
	}
}

func (field Field[T]) valid() bool {
	return field.seal == validFieldSeal &&
		field.valueType == reflect.TypeFor[T]() &&
		validDeclaredOperators(field.valueType, field.operators) &&
		!field.column.Raw && field.column.Alias == "" &&
		validIdentifierSegment(field.column.Name) &&
		(field.column.Table == "" || validIdentifierSegment(field.column.Table))
}

func (metadata fieldMetadata) validFor(actualType reflect.Type) bool {
	return metadata.seal == validFieldSeal &&
		metadata.descriptorType != nil &&
		actualType == metadata.descriptorType &&
		metadata.valueType != nil &&
		validDeclaredOperators(metadata.valueType, metadata.operators) &&
		!metadata.column.Raw && metadata.column.Alias == "" &&
		validIdentifierSegment(metadata.column.Name) &&
		(metadata.column.Table == "" || validIdentifierSegment(metadata.column.Table))
}

func validDeclaredOperators(
	valueType reflect.Type,
	operators weave.OperatorSet,
) bool {
	if valueType == nil || operators.Count() == 0 {
		return false
	}
	recognized := 0
	for _, operator := range []weave.Operator{
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
	} {
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

func validIdentifierSegment(value string) bool {
	if value == "" || value == "*" || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) {
		return false
	}
	for index, character := range value {
		if unicode.IsControl(character) || character == '.' {
			return false
		}
		if index == 0 {
			if character != '_' && !unicode.IsLetter(character) {
				return false
			}
			continue
		}
		if character != '_' && character != '$' &&
			!unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func invalidFieldDefinitionError() error {
	return fmt.Errorf(
		"gorm: field names must be valid non-raw identifier segments: %w",
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
			"gorm: WithOperators requires at least one operator: %w",
			weave.ErrInvalidValue,
		)
	}
	for _, operator := range operators {
		if !standardOperator(operator) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gorm: Field contains an invalid operator: %w",
				weave.ErrInvalidValue,
			)
		}
		if operator == weave.OperatorBetween && !numericType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gorm: Between requires a numeric Field type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
		if textOperator(operator) && !stringType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gorm: literal-text operators require a string Field type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
	}
	return weave.NewOperatorSet(operators...), nil
}

func inferredFieldOperators(valueType reflect.Type) weave.OperatorSet {
	nullOperators := []weave.Operator{
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	}
	valueOperators := []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	}

	switch {
	case numericType(valueType):
		return weave.NewOperatorSet(append(valueOperators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorBetween,
		)...)
	case stringType(valueType):
		return weave.NewOperatorSet(append(valueOperators,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		)...)
	case valueType == reflect.TypeFor[bool](),
		valueType == reflect.TypeFor[time.Time](),
		valueType == reflect.TypeFor[[]byte]():
		return weave.NewOperatorSet(valueOperators...)
	default:
		return weave.NewOperatorSet(nullOperators...)
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

func standardOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorEQ,
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
		weave.OperatorHasSuffix:
		return true
	default:
		return false
	}
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
