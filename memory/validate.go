package memory

import (
	"reflect"

	"github.com/imbrooklyn/weave"
)

type validatedNode[R any] struct {
	kind     weave.Kind
	path     weave.NodePath
	origin   weave.Origin
	operator weave.Operator
	feature  weave.Feature
	constant bool
	logic    weave.Logic
	children []validatedNode[R]
	leaf     conditionPlan[R]
}

func validatePredicate[R any](
	root weave.NodeView[Condition[R], Expression[R]],
) (validatedNode[R], error) {
	if !root.Valid() {
		return validatedNode[R]{}, &weave.Error{
			Code:  weave.CodeInvalidPredicate,
			Phase: weave.PhaseValidate,
		}
	}
	return validateNode(root, 0, true)
}

func validateNode[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	depth int,
	root bool,
) (validatedNode[R], error) {
	if !node.Valid() {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidPredicate,
			0,
			0,
			nil,
			nil,
		)
	}
	if depth > weave.MaxPredicateDepth {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeDepthLimit,
			0,
			0,
			nil,
			nil,
		)
	}

	validated := validatedNode[R]{
		kind:   node.Kind(),
		path:   node.Path(),
		origin: node.Origin(),
	}
	switch node.Kind() {
	case weave.KindConstant:
		view, ok := node.AsConstant()
		if !ok || root {
			return validatedNode[R]{}, invalidPredicateError(node)
		}
		validated.constant = view.Value()
		return validated, nil

	case weave.KindGroup:
		view, ok := node.AsGroup()
		if !ok || !validLogic(view.Logic()) ||
			(root && view.Logic() != weave.LogicAllOf) {
			return validatedNode[R]{}, invalidPredicateError(node)
		}
		validated.logic = view.Logic()
		validated.children = make([]validatedNode[R], 0, view.ChildCount())
		for index := range view.ChildCount() {
			child, ok := view.Child(index)
			if !ok {
				return validatedNode[R]{}, invalidPredicateError(node)
			}
			childPlan, err := validateNode(child, depth+1, false)
			if err != nil {
				return validatedNode[R]{}, err
			}
			validated.children = append(validated.children, childPlan)
		}
		return validated, nil

	case weave.KindComparison:
		return validateComparison(node, validated)
	case weave.KindMembership:
		return validateMembership(node, validated)
	case weave.KindRange:
		return validateRange(node, validated)
	case weave.KindNull:
		return validateNull(node, validated)
	case weave.KindText:
		return validateText(node, validated)
	case weave.KindNativeCondition:
		return validateNativeCondition(node, validated, depth)
	case weave.KindNativeExpression:
		return validateNativeExpression(node, validated)
	default:
		return validatedNode[R]{}, invalidPredicateError(node)
	}
}

func validateComparison[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsComparison()
	if !ok || !comparisonOperator(view.Operator()) {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, metadata, err := resolveField[R](
		node,
		view.Field(),
		operator,
		view.ValueType(),
	)
	if err != nil {
		return validatedNode[R]{}, err
	}
	if err := validateApplicable(
		node,
		metadata,
		operator,
		reflect.TypeOf(view.Field()),
		view.ValueType(),
	); err != nil {
		return validatedNode[R]{}, err
	}
	value := view.Value()
	if !assignableValue(value, view.ValueType(), metadata.valueType) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.ValueType(),
		)
	}
	plan, ok := field.memoryComparisonPlan(operator, value)
	if !ok {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.ValueType(),
		)
	}
	validated.operator = operator
	validated.leaf = plan
	return validated, nil
}

func validateMembership[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsMembership()
	if !ok || !membershipOperator(view.Operator()) || view.ValueCount() == 0 {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, metadata, err := resolveField[R](
		node,
		view.Field(),
		operator,
		view.ElementType(),
	)
	if err != nil {
		return validatedNode[R]{}, err
	}
	if err := validateApplicable(
		node,
		metadata,
		operator,
		reflect.TypeOf(view.Field()),
		view.ElementType(),
	); err != nil {
		return validatedNode[R]{}, err
	}

	values := make([]any, view.ValueCount())
	for index := range view.ValueCount() {
		value, ok := view.Value(index)
		if !ok || !assignableValue(value, reflect.TypeOf(value), metadata.valueType) {
			return validatedNode[R]{}, validationError(
				node,
				weave.CodeInvalidValue,
				operator,
				0,
				reflect.TypeOf(view.Field()),
				reflect.TypeOf(value),
			)
		}
		values[index] = value
	}
	plan, ok := field.memoryMembershipPlan(operator, values)
	if !ok {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.ElementType(),
		)
	}
	validated.operator = operator
	validated.leaf = plan
	return validated, nil
}

func validateRange[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsRange()
	if !ok || view.Operator() != weave.OperatorBetween {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, metadata, err := resolveField[R](
		node,
		view.Field(),
		operator,
		view.BoundType(),
	)
	if err != nil {
		return validatedNode[R]{}, err
	}
	if err := validateApplicable(
		node,
		metadata,
		operator,
		reflect.TypeOf(view.Field()),
		view.BoundType(),
	); err != nil {
		return validatedNode[R]{}, err
	}
	if !assignableValue(view.Lower(), view.BoundType(), metadata.valueType) ||
		!assignableValue(view.Upper(), view.BoundType(), metadata.valueType) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.BoundType(),
		)
	}
	plan, ok := field.memoryRangePlan(view.Lower(), view.Upper())
	if !ok {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			operator,
			0,
			reflect.TypeOf(view.Field()),
			view.BoundType(),
		)
	}
	validated.operator = operator
	validated.leaf = plan
	return validated, nil
}

func validateNull[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsNull()
	if !ok || !nullOperator(view.Operator()) {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, metadata, err := resolveField[R](node, view.Field(), operator, nil)
	if err != nil {
		return validatedNode[R]{}, err
	}
	if err := validateApplicable(
		node,
		metadata,
		operator,
		reflect.TypeOf(view.Field()),
		nil,
	); err != nil {
		return validatedNode[R]{}, err
	}
	validated.operator = operator
	validated.leaf = field.memoryNullPlan(operator)
	return validated, nil
}

func validateText[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsText()
	if !ok || !textOperator(view.Operator()) {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	valueType := reflect.TypeFor[string]()
	field, metadata, err := resolveField[R](
		node,
		view.Field(),
		operator,
		valueType,
	)
	if err != nil {
		return validatedNode[R]{}, err
	}
	if err := validateApplicable(
		node,
		metadata,
		operator,
		reflect.TypeOf(view.Field()),
		valueType,
	); err != nil {
		return validatedNode[R]{}, err
	}
	validated.operator = operator
	validated.leaf = field.memoryTextPlan(operator, view.Value())
	return validated, nil
}

func validateNativeCondition[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
	depth int,
) (validatedNode[R], error) {
	view, ok := node.AsNativeCondition()
	if !ok {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	if !compilerCapabilities.Features.Has(weave.FeatureNativeCondition) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeUnsupportedFeature,
			0,
			weave.FeatureNativeCondition,
			nil,
			nil,
		)
	}
	if depth != 1 {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeNonNestableNative,
			0,
			weave.FeatureNativeCondition,
			nil,
			nil,
		)
	}
	condition := view.Condition()
	if isNilLike(condition) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			0,
			weave.FeatureNativeCondition,
			nil,
			reflect.TypeOf(condition),
		)
	}
	validated.feature = weave.FeatureNativeCondition
	validated.leaf = func() Condition[R] { return condition }
	return validated, nil
}

func validateNativeExpression[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	validated validatedNode[R],
) (validatedNode[R], error) {
	view, ok := node.AsNativeExpression()
	if !ok {
		return validatedNode[R]{}, invalidPredicateError(node)
	}
	if !compilerCapabilities.Features.Has(weave.FeatureNativeExpression) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeUnsupportedFeature,
			0,
			weave.FeatureNativeExpression,
			nil,
			nil,
		)
	}
	expression := view.Expression()
	if isNilLike(expression) {
		return validatedNode[R]{}, validationError(
			node,
			weave.CodeInvalidValue,
			0,
			weave.FeatureNativeExpression,
			nil,
			reflect.TypeOf(expression),
		)
	}
	validated.feature = weave.FeatureNativeExpression
	validated.leaf = func() Condition[R] {
		return Condition[R](expression)
	}
	return validated, nil
}

func resolveField[R any](
	node weave.NodeView[Condition[R], Expression[R]],
	value any,
	operator weave.Operator,
	valueType reflect.Type,
) (runtimeField[R], fieldDescriptorMetadata, error) {
	fieldType := reflect.TypeOf(value)
	if isNilLike(value) {
		return nil, fieldDescriptorMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
		)
	}
	field, ok := value.(runtimeField[R])
	if !ok {
		return nil, fieldDescriptorMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
		)
	}
	metadata := field.memoryFieldDescriptor()
	if !metadata.valid || metadata.recordType != reflect.TypeFor[R]() {
		return nil, fieldDescriptorMetadata{}, validationError(
			node,
			weave.CodeInvalidField,
			operator,
			0,
			fieldType,
			valueType,
		)
	}
	return field, metadata, nil
}

func validateApplicable[C, E any](
	node weave.NodeView[C, E],
	metadata fieldDescriptorMetadata,
	operator weave.Operator,
	fieldType reflect.Type,
	valueType reflect.Type,
) error {
	if !compilerCapabilities.Operators.Has(operator) {
		return validationError(
			node,
			weave.CodeUnsupportedOperator,
			operator,
			0,
			fieldType,
			valueType,
		)
	}
	if !metadata.capabilities.Operators.Has(operator) {
		return validationError(
			node,
			weave.CodeOperatorNotApplicable,
			operator,
			0,
			fieldType,
			valueType,
		)
	}
	return nil
}

func assignableValue(value any, reported, expected reflect.Type) bool {
	if isNilLike(value) || reported == nil || expected == nil {
		return false
	}
	dynamic := reflect.TypeOf(value)
	return dynamic == reported && dynamic.AssignableTo(expected)
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

func invalidPredicateError[C, E any](
	node weave.NodeView[C, E],
) *weave.Error {
	return validationError(
		node,
		weave.CodeInvalidPredicate,
		0,
		0,
		nil,
		nil,
	)
}

func validationError[C, E any](
	node weave.NodeView[C, E],
	code weave.ErrorCode,
	operator weave.Operator,
	feature weave.Feature,
	fieldType reflect.Type,
	valueType reflect.Type,
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
	}
}
