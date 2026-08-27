package goqu

import (
	"fmt"
	"reflect"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
)

// Field is an immutable typed declaration of one canonical goqu identifier
// and the standard operators applicable to it. Field values describe database
// query types and applicability; they are not an authorization system. Valid
// Field values contain no caller-owned mutable state, can be copied, and are
// safe for concurrent capability discovery and compilation.
type Field[T any] struct {
	seal      *fieldSeal
	schema    string
	table     string
	column    string
	valueType reflect.Type
	operators weave.OperatorSet
}

type fieldSeal struct{}

var validFieldSeal = &fieldSeal{}

type fieldMetadata struct {
	seal           *fieldSeal
	descriptorType reflect.Type
	schema         string
	table          string
	column         string
	identifier     exp.IdentifierExpression
	valueType      reflect.Type
	operators      weave.OperatorSet
}

type fieldDescriptor interface {
	goquFieldMetadata() fieldMetadata
}

// NewField returns a typed descriptor with conservative operators inferred
// from T. identifier must describe one ordinary, non-wildcard column using
// safe schema, table, and column segments.
func NewField[T any](identifier exp.IdentifierExpression) (Field[T], error) {
	return newField[T](identifier, nil, false)
}

// NewFieldWithOperators returns a typed descriptor whose non-empty operator
// list is the complete replacement set. The constructor rejects unknown
// operators, Between for non-numeric T, and literal-text operators for
// non-string T.
func NewFieldWithOperators[T any](
	identifier exp.IdentifierExpression,
	operators ...weave.Operator,
) (Field[T], error) {
	return newField[T](identifier, operators, true)
}

// Identifier returns a newly reconstructed canonical goqu identifier. A zero
// or otherwise invalid Field returns nil.
func (field Field[T]) Identifier() exp.IdentifierExpression {
	if !field.valid() {
		return nil
	}
	return exp.NewIdentifierExpression(field.schema, field.table, field.column)
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
	identifier exp.IdentifierExpression,
	operators []weave.Operator,
	explicit bool,
) (Field[T], error) {
	if isNilLike(identifier) || identifier.IsEmpty() {
		return Field[T]{}, invalidFieldDefinitionError()
	}

	column, ok := identifier.GetCol().(string)
	schema, table := identifier.GetSchema(), identifier.GetTable()
	if !ok || !validIdentifierSegment(column) ||
		(schema != "" && (!validIdentifierSegment(schema) || table == "")) ||
		(table != "" && !validIdentifierSegment(table)) {
		return Field[T]{}, invalidFieldDefinitionError()
	}

	valueType := reflect.TypeFor[T]()
	operatorSet, err := fieldOperatorSet(valueType, operators, explicit)
	if err != nil {
		return Field[T]{}, err
	}

	return Field[T]{
		seal:      validFieldSeal,
		schema:    schema,
		table:     table,
		column:    column,
		valueType: valueType,
		operators: operatorSet,
	}, nil
}

func (field Field[T]) goquFieldMetadata() fieldMetadata {
	return fieldMetadata{
		seal:           field.seal,
		descriptorType: reflect.TypeFor[Field[T]](),
		schema:         field.schema,
		table:          field.table,
		column:         field.column,
		identifier: exp.NewIdentifierExpression(
			field.schema,
			field.table,
			field.column,
		),
		valueType: field.valueType,
		operators: field.operators,
	}
}

func (field Field[T]) valid() bool {
	return field.seal == validFieldSeal &&
		field.valueType == reflect.TypeFor[T]() &&
		validDeclaredOperators(field.valueType, field.operators) &&
		validIdentifierParts(field.schema, field.table, field.column)
}

func (metadata fieldMetadata) validFor(actualType reflect.Type) bool {
	return metadata.seal == validFieldSeal &&
		metadata.descriptorType != nil &&
		actualType == metadata.descriptorType &&
		metadata.valueType != nil &&
		validDeclaredOperators(metadata.valueType, metadata.operators) &&
		validIdentifierParts(metadata.schema, metadata.table, metadata.column) &&
		!isNilLike(metadata.identifier)
}

func validIdentifierParts(schema, table, column string) bool {
	return validIdentifierSegment(column) &&
		(schema == "" || (table != "" && validIdentifierSegment(schema))) &&
		(table == "" || validIdentifierSegment(table))
}

func validIdentifierSegment(value string) bool {
	if value == "" || value == "*" || !utf8.ValidString(value) {
		return false
	}
	for index, character := range value {
		if unicode.IsControl(character) {
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
		"goqu: fields require a canonical ordinary identifier: %w",
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
			"goqu: NewFieldWithOperators requires at least one operator: %w",
			weave.ErrInvalidValue,
		)
	}
	for _, operator := range operators {
		if !standardOperator(operator) {
			return weave.OperatorSet{}, fmt.Errorf(
				"goqu: Field contains an invalid operator: %w",
				weave.ErrInvalidValue,
			)
		}
		if operator == weave.OperatorBetween && !numericType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"goqu: Between requires a numeric Field type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
		if textOperator(operator) && !stringType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"goqu: literal-text operators require a string Field type: %w",
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
