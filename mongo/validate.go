package mongo

import (
	"errors"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var errBSONValueNotEncodable = errors.New(
	"mongo: value is not encodable by the default BSON registry",
)

var errInvalidLiteralText = errors.New(
	"mongo: literal text must be valid UTF-8 without NUL",
)

func (compiler Compiler) validatePredicate(
	predicate weave.Predicate[Filter, Expression],
) (validatedNode, error) {
	root := predicate.Root()
	if !root.Valid() {
		return validatedNode{}, &weave.Error{
			Code:  weave.CodeInvalidPredicate,
			Phase: weave.PhaseValidate,
		}
	}
	validated, err := compiler.validateNode(root, 0, true)
	if err != nil {
		return validatedNode{}, err
	}
	if !equalRequirements(
		requirementsForPlan(validated),
		predicate.Requirements(),
	) {
		return validatedNode{}, invalidPredicateError(root)
	}
	return validated, nil
}

func (compiler Compiler) validateNode(
	node weave.NodeView[Filter, Expression],
	depth int,
	root bool,
) (validatedNode, error) {
	if !node.Valid() {
		return validatedNode{}, invalidPredicateError(node)
	}
	if depth > weave.MaxPredicateDepth {
		return validatedNode{}, validationError(
			node,
			weave.CodeDepthLimit,
			0,
			0,
			nil,
			nil,
			nil,
		)
	}
	origin := node.Origin()
	if (root && origin != (weave.Origin{})) ||
		(!root && origin.Sequence == 0) {
		return validatedNode{}, invalidPredicateError(node)
	}

	validated := validatedNode{
		kind:   node.Kind(),
		path:   node.Path(),
		origin: origin,
	}
	switch node.Kind() {
	case weave.KindConstant:
		view, ok := node.AsConstant()
		if !ok || root {
			return validatedNode{}, invalidPredicateError(node)
		}
		validated.constant = view.Value()
		return validated, nil

	case weave.KindGroup:
		view, ok := node.AsGroup()
		if !ok || !validLogic(view.Logic()) ||
			(root && view.Logic() != weave.LogicAllOf) ||
			(!root && view.ChildCount() == 0) {
			return validatedNode{}, invalidPredicateError(node)
		}
		validated.logic = view.Logic()
		validated.children = make([]validatedNode, 0, view.ChildCount())
		for index := range view.ChildCount() {
			child, ok := view.Child(index)
			if !ok {
				return validatedNode{}, invalidPredicateError(node)
			}
			childPlan, err := compiler.validateNode(child, depth+1, false)
			if err != nil {
				return validatedNode{}, err
			}
			validated.children = append(validated.children, childPlan)
		}
		return validated, nil

	case weave.KindComparison:
		return compiler.validateComparison(node, validated)
	case weave.KindMembership:
		return compiler.validateMembership(node, validated)
	case weave.KindRange:
		return compiler.validateRange(node, validated)
	case weave.KindNull:
		return compiler.validateNull(node, validated)
	case weave.KindText:
		return compiler.validateText(node, validated)
	case weave.KindNativeCondition:
		return compiler.validateNativeCondition(node, validated, depth)
	case weave.KindNativeExpression:
		return compiler.validateNativeExpression(node, validated)
	default:
		return validatedNode{}, invalidPredicateError(node)
	}
}

func (compiler Compiler) validateComparison(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsComparison()
	if !ok || !comparisonOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	metadata, err := compiler.validateField(
		node,
		view.Field(),
		operator,
		view.ValueType(),
	)
	if err != nil {
		return validatedNode{}, err
	}
	value := view.Value()
	if !assignableValue(value, view.ValueType(), metadata.valueType) {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.ValueType(),
			nil,
		)
	}
	if !bsonEncodable(value) {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			reflect.TypeOf(value),
			errBSONValueNotEncodable,
		)
	}
	validated.operator = operator
	validated.fieldPath = metadata.path
	validated.values = []any{value}
	return validated, nil
}

func (compiler Compiler) validateMembership(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsMembership()
	if !ok || !membershipOperator(view.Operator()) || view.ValueCount() == 0 {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	metadata, err := compiler.validateField(
		node,
		view.Field(),
		operator,
		view.ElementType(),
	)
	if err != nil {
		return validatedNode{}, err
	}

	values := make([]any, view.ValueCount())
	for index := range view.ValueCount() {
		value, ok := view.Value(index)
		if !ok || !assignableValue(
			value,
			view.ElementType(),
			metadata.valueType,
		) {
			return validatedNode{}, validationError(
				node,
				weave.CodeInvalidValue,
				operator,
				0,
				reflect.TypeOf(view.Field()),
				reflect.TypeOf(value),
				nil,
			)
		}
		if !bsonEncodable(value) {
			return validatedNode{}, validationError(
				node,
				weave.CodeInvalidValue,
				operator,
				0,
				reflect.TypeOf(view.Field()),
				reflect.TypeOf(value),
				errBSONValueNotEncodable,
			)
		}
		values[index] = value
	}
	validated.operator = operator
	validated.fieldPath = metadata.path
	validated.values = values
	return validated, nil
}

func (compiler Compiler) validateRange(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsRange()
	if !ok || view.Operator() != weave.OperatorBetween {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	metadata, err := compiler.validateField(
		node,
		view.Field(),
		operator,
		view.BoundType(),
	)
	if err != nil {
		return validatedNode{}, err
	}
	for _, value := range []any{view.Lower(), view.Upper()} {
		if !assignableValue(value, view.BoundType(), metadata.valueType) {
			return validatedNode{}, validationError(
				node,
				weave.CodeInvalidValue,
				operator,
				0,
				reflect.TypeOf(view.Field()),
				reflect.TypeOf(value),
				nil,
			)
		}
		if !bsonEncodable(value) {
			return validatedNode{}, validationError(
				node,
				weave.CodeInvalidValue,
				operator,
				0,
				reflect.TypeOf(view.Field()),
				reflect.TypeOf(value),
				errBSONValueNotEncodable,
			)
		}
	}
	validated.operator = operator
	validated.fieldPath = metadata.path
	validated.values = []any{view.Lower(), view.Upper()}
	return validated, nil
}

func (compiler Compiler) validateNull(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsNull()
	if !ok || !nullOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	metadata, err := compiler.validateField(
		node,
		view.Field(),
		operator,
		nil,
	)
	if err != nil {
		return validatedNode{}, err
	}
	validated.operator = operator
	validated.fieldPath = metadata.path
	return validated, nil
}

func (compiler Compiler) validateText(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsText()
	if !ok || !textOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	metadata, err := compiler.validateField(
		node,
		view.Field(),
		operator,
		reflect.TypeFor[string](),
	)
	if err != nil {
		return validatedNode{}, err
	}
	text := view.Value()
	if !stringType(metadata.valueType) || !utf8.ValidString(text) ||
		strings.IndexByte(text, 0) >= 0 {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			reflect.TypeFor[string](),
			errInvalidLiteralText,
		)
	}
	if !bsonEncodable(text) {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			reflect.TypeFor[string](),
			errBSONValueNotEncodable,
		)
	}
	validated.operator = operator
	validated.fieldPath = metadata.path
	validated.text = text
	return validated, nil
}

func (compiler Compiler) validateNativeCondition(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
	depth int,
) (validatedNode, error) {
	view, ok := node.AsNativeCondition()
	if !ok {
		return validatedNode{}, invalidPredicateError(node)
	}
	if !compilerCapabilities.Features.Has(weave.FeatureNativeCondition) {
		return validatedNode{}, validationError(
			node,
			weave.CodeUnsupportedFeature,
			0,
			weave.FeatureNativeCondition,
			nil,
			nil,
			nil,
		)
	}
	if depth != 1 {
		return validatedNode{}, validationError(
			node,
			weave.CodeNonNestableNative,
			0,
			weave.FeatureNativeCondition,
			nil,
			nil,
			nil,
		)
	}
	filter := view.Condition()
	if filter == nil {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			0,
			weave.FeatureNativeCondition,
			nil,
			reflect.TypeOf(filter),
			nil,
		)
	}
	validated.feature = weave.FeatureNativeCondition
	validated.native = filter
	return validated, nil
}

func (compiler Compiler) validateNativeExpression(
	node weave.NodeView[Filter, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsNativeExpression()
	if !ok {
		return validatedNode{}, invalidPredicateError(node)
	}
	if !compilerCapabilities.Features.Has(weave.FeatureNativeExpression) {
		return validatedNode{}, validationError(
			node,
			weave.CodeUnsupportedFeature,
			0,
			weave.FeatureNativeExpression,
			nil,
			nil,
			nil,
		)
	}
	expression := view.Expression()
	if expression == nil {
		return validatedNode{}, validationError(
			node,
			weave.CodeInvalidValue,
			0,
			weave.FeatureNativeExpression,
			nil,
			reflect.TypeOf(expression),
			nil,
		)
	}
	validated.feature = weave.FeatureNativeExpression
	validated.expression = expression
	return validated, nil
}

func (compiler Compiler) validateField(
	node weave.NodeView[Filter, Expression],
	value any,
	operator weave.Operator,
	valueType reflect.Type,
) (fieldMetadata, error) {
	fieldType := reflect.TypeOf(value)
	if isNilLike(value) {
		return fieldMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
			nil,
		)
	}
	descriptor, ok := value.(fieldDescriptor)
	if !ok {
		return fieldMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
			nil,
		)
	}
	metadata := descriptor.mongoFieldMetadata()
	if !metadata.validFor(fieldType) {
		return fieldMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
			nil,
		)
	}
	if !compilerCapabilities.Operators.Has(operator) {
		return fieldMetadata{}, validationError(
			node,
			weave.CodeUnsupportedOperator,
			operator,
			0,
			fieldType,
			valueType,
			nil,
		)
	}
	if !metadata.operators.Has(operator) {
		return fieldMetadata{}, validationError(
			node,
			weave.CodeOperatorNotApplicable,
			operator,
			0,
			fieldType,
			valueType,
			nil,
		)
	}
	return metadata, nil
}

func assignableValue(value any, reported, expected reflect.Type) bool {
	if isNilLike(value) || reported == nil || expected == nil {
		return false
	}
	dynamic := reflect.TypeOf(value)
	return dynamic.AssignableTo(expected)
}

func bsonEncodable(value any) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	_, _, err := bson.MarshalValue(value)
	return err == nil
}

func comparisonOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorLT,
		weave.OperatorLTE,
		weave.OperatorGT,
		weave.OperatorGTE:
		return true
	default:
		return false
	}
}

func membershipOperator(operator weave.Operator) bool {
	return operator == weave.OperatorIn || operator == weave.OperatorNotIn
}

func nullOperator(operator weave.Operator) bool {
	return operator == weave.OperatorIsNull || operator == weave.OperatorNotNull
}

func validLogic(logic weave.Logic) bool {
	switch logic {
	case weave.LogicAllOf,
		weave.LogicAnyOf,
		weave.LogicNoneOf,
		weave.LogicNotAllOf:
		return true
	default:
		return false
	}
}

func invalidPredicateError(
	node weave.NodeView[Filter, Expression],
) *weave.Error {
	return validationError(
		node,
		weave.CodeInvalidPredicate,
		0,
		0,
		nil,
		nil,
		nil,
	)
}

func validationError(
	node weave.NodeView[Filter, Expression],
	code weave.ErrorCode,
	operator weave.Operator,
	feature weave.Feature,
	fieldType reflect.Type,
	valueType reflect.Type,
	cause error,
) *weave.Error {
	return &weave.Error{
		Code:      code,
		Phase:     weave.PhaseValidate,
		Path:      node.Path(),
		Origin:    node.Origin(),
		Operator:  operator,
		Feature:   feature,
		FieldType: fieldType,
		ValueType: valueType,
		Cause:     cause,
	}
}
