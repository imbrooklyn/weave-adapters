package gormgen

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
)

// FieldSpec is an immutable declaration of a generated column's expected
// query value type and applicable standard operators. FieldSpec values are
// created by NewFieldSpec and can be installed with WithFieldSpecs.
type FieldSpec struct {
	seal       *fieldSpecSeal
	column     clause.Column
	nativeType reflect.Type
	valueType  reflect.Type
	operators  weave.OperatorSet
}

type fieldSpecSeal struct{}

var validFieldSpecSeal = &fieldSpecSeal{}

type columnIdentity struct {
	table string
	name  string
}

type nativeFieldMetadata struct {
	column     clause.Column
	nativeType reflect.Type
	valueType  reflect.Type
	operators  weave.OperatorSet
}

// NewFieldSpec validates native as a pure GORM Gen column and declares T as
// its expected query value type. T must be directly assignable to the public
// Eq method's value parameter; no reflection conversion is used.
//
// An empty operators list selects the conservative defaults for T. A non-empty
// list is the complete replacement set, not an extension of those defaults.
func NewFieldSpec[T any](
	native field.Expr,
	operators ...weave.Operator,
) (FieldSpec, error) {
	metadata, err := inspectNativeField(native)
	if err != nil {
		return FieldSpec{}, err
	}

	valueType := reflect.TypeFor[T]()
	if valueType == nil || !valueType.AssignableTo(metadata.valueType) {
		return FieldSpec{}, fmt.Errorf(
			"gormgen: FieldSpec value type is not assignable to the generated field: %w",
			weave.ErrInvalidValue,
		)
	}

	operatorSet, err := fieldOperatorSet(valueType, operators)
	if err != nil {
		return FieldSpec{}, err
	}

	return FieldSpec{
		seal:       validFieldSpecSeal,
		column:     metadata.column,
		nativeType: metadata.nativeType,
		valueType:  valueType,
		operators:  operatorSet,
	}, nil
}

func inspectNativeField(native field.Expr) (nativeFieldMetadata, error) {
	if isNilLike(native) {
		return nativeFieldMetadata{}, invalidGeneratedFieldError()
	}
	if err := native.CondError(); err != nil {
		return nativeFieldMetadata{}, invalidGeneratedFieldError()
	}

	column, ok := any(native.RawExpr()).(clause.Column)
	if !ok || column.Raw || column.Alias != "" ||
		strings.TrimSpace(column.Name) == "" || column.Name == "*" {
		return nativeFieldMetadata{}, invalidGeneratedFieldError()
	}

	nativeType := reflect.TypeOf(native)
	method, ok := nativeType.MethodByName("Eq")
	if !ok || method.Type.IsVariadic() ||
		method.Type.NumIn() != 2 || method.Type.In(0) != nativeType ||
		method.Type.NumOut() != 1 ||
		method.Type.Out(0) != reflect.TypeFor[field.Expr]() {
		return nativeFieldMetadata{}, invalidGeneratedFieldError()
	}

	valueType := method.Type.In(1)
	return nativeFieldMetadata{
		column:     column,
		nativeType: nativeType,
		valueType:  valueType,
		operators:  inferredFieldOperators(valueType),
	}, nil
}

func invalidGeneratedFieldError() error {
	return fmt.Errorf(
		"gormgen: standard fields must be pure generated columns with a supported Eq signature: %w",
		weave.ErrInvalidField,
	)
}

func fieldOperatorSet(
	valueType reflect.Type,
	operators []weave.Operator,
) (weave.OperatorSet, error) {
	if len(operators) == 0 {
		return inferredFieldOperators(valueType), nil
	}

	for _, operator := range operators {
		if !standardOperator(operator) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gormgen: FieldSpec contains an invalid operator: %w",
				weave.ErrInvalidValue,
			)
		}
		if operator == weave.OperatorBetween && !numericType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gormgen: Between is not applicable to the FieldSpec value type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
		if textOperator(operator) && !stringType(valueType) {
			return weave.OperatorSet{}, fmt.Errorf(
				"gormgen: literal-text operators require a string FieldSpec value type: %w",
				weave.ErrOperatorNotApplicable,
			)
		}
	}
	return weave.NewOperatorSet(operators...), nil
}

func inferredFieldOperators(valueType reflect.Type) weave.OperatorSet {
	operators := []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	}

	switch {
	case numericType(valueType):
		operators = append(
			operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorBetween,
		)
	case stringType(valueType):
		operators = append(
			operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		)
	case valueType == reflect.TypeFor[time.Time]():
		operators = append(
			operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
		)
	}

	return weave.NewOperatorSet(operators...)
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

func (spec FieldSpec) identity() columnIdentity {
	return columnIdentity{table: spec.column.Table, name: spec.column.Name}
}

func (spec FieldSpec) valid() bool {
	return spec.seal == validFieldSpecSeal &&
		spec.nativeType != nil && spec.valueType != nil &&
		!spec.column.Raw && spec.column.Alias == "" &&
		strings.TrimSpace(spec.column.Name) != "" &&
		spec.column.Name != "*"
}

func equivalentFieldSpecs(left, right FieldSpec) bool {
	return left.valid() && right.valid() &&
		left.column == right.column &&
		left.nativeType == right.nativeType &&
		left.valueType == right.valueType &&
		equalOperatorSets(left.operators, right.operators)
}

func equalOperatorSets(left, right weave.OperatorSet) bool {
	return left.Count() == right.Count() && left.ContainsAll(right)
}
