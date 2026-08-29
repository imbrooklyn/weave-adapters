package elasticsearch

import (
	"reflect"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
)

func (compiler Compiler) validatePredicate(
	predicate weave.Predicate[Query, Expression],
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
	if !equalRequirements(requirementsForPlan(validated), predicate.Requirements()) {
		return validatedNode{}, invalidPredicateError(root)
	}
	return validated, nil
}

func (compiler Compiler) validateNode(
	node weave.NodeView[Query, Expression],
	depth int,
	root bool,
) (validatedNode, error) {
	if !node.Valid() {
		return validatedNode{}, invalidPredicateError(node)
	}
	if depth > weave.MaxPredicateDepth {
		return validatedNode{}, validationError(
			node, weave.CodeDepthLimit, 0, 0, nil, nil, nil,
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
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsComparison()
	if !ok || !comparisonOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, err := compiler.validateField(
		node, view.Field(), operator, view.ValueType(),
	)
	if err != nil {
		return validatedNode{}, err
	}
	value, ok := checkedScalarValue(field, view.Value(), view.ValueType())
	if !ok || reservedNullValue(field, value) {
		return validatedNode{}, validationError(
			node, weave.CodeInvalidValue, operator, 0,
			reflect.TypeOf(view.Field()), view.ValueType(), nil,
		)
	}
	validated.operator = operator
	validated.field = field
	validated.values = []scalarValue{value}
	return validated, nil
}

func (compiler Compiler) validateMembership(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsMembership()
	if !ok || !membershipOperator(view.Operator()) || view.ValueCount() == 0 {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, err := compiler.validateField(
		node, view.Field(), operator, view.ElementType(),
	)
	if err != nil {
		return validatedNode{}, err
	}
	values := make([]scalarValue, view.ValueCount())
	for index := range view.ValueCount() {
		raw, ok := view.Value(index)
		if !ok {
			return validatedNode{}, invalidPredicateError(node)
		}
		values[index], ok = checkedScalarValue(field, raw, view.ElementType())
		if !ok || reservedNullValue(field, values[index]) {
			return validatedNode{}, validationError(
				node, weave.CodeInvalidValue, operator, 0,
				reflect.TypeOf(view.Field()), reflect.TypeOf(raw), nil,
			)
		}
	}
	validated.operator = operator
	validated.field = field
	validated.values = values
	return validated, nil
}

func (compiler Compiler) validateRange(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsRange()
	if !ok || view.Operator() != weave.OperatorBetween {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, err := compiler.validateField(
		node, view.Field(), operator, view.BoundType(),
	)
	if err != nil {
		return validatedNode{}, err
	}
	values := make([]scalarValue, 2)
	for index, raw := range []any{view.Lower(), view.Upper()} {
		values[index], ok = checkedScalarValue(field, raw, view.BoundType())
		if !ok || reservedNullValue(field, values[index]) {
			return validatedNode{}, validationError(
				node, weave.CodeInvalidValue, operator, 0,
				reflect.TypeOf(view.Field()), reflect.TypeOf(raw), nil,
			)
		}
	}
	validated.operator = operator
	validated.field = field
	validated.values = values
	return validated, nil
}

func (compiler Compiler) validateNull(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsNull()
	if !ok || !nullOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, err := compiler.validateField(node, view.Field(), operator, nil)
	if err != nil {
		return validatedNode{}, err
	}
	if field.nullKind == NullUntracked {
		return validatedNode{}, validationError(
			node, weave.CodeOperatorNotApplicable, operator, 0,
			reflect.TypeOf(view.Field()), nil, nil,
		)
	}
	validated.operator = operator
	validated.field = field
	return validated, nil
}

func (compiler Compiler) validateText(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsText()
	if !ok || !textOperator(view.Operator()) {
		return validatedNode{}, invalidPredicateError(node)
	}
	operator := view.Operator()
	field, err := compiler.validateField(
		node, view.Field(), operator, reflect.TypeFor[string](),
	)
	if err != nil {
		return validatedNode{}, err
	}
	text := view.Value()
	if !utf8.ValidString(text) ||
		(field.nullKind == NullValueMarker &&
			field.nullValue.stringValue == text) {
		return validatedNode{}, validationError(
			node, weave.CodeInvalidValue, operator, 0,
			reflect.TypeOf(view.Field()), reflect.TypeFor[string](), nil,
		)
	}
	validated.operator = operator
	validated.field = field
	validated.text = text
	return validated, nil
}

func (compiler Compiler) validateNativeCondition(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
	depth int,
) (validatedNode, error) {
	view, ok := node.AsNativeCondition()
	if !ok {
		return validatedNode{}, invalidPredicateError(node)
	}
	if !compiler.state.capabilities.Features.Has(weave.FeatureNativeCondition) {
		return validatedNode{}, validationError(
			node, weave.CodeUnsupportedFeature, 0,
			weave.FeatureNativeCondition, nil, nil, nil,
		)
	}
	if depth != 1 {
		return validatedNode{}, validationError(
			node, weave.CodeNonNestableNative, 0,
			weave.FeatureNativeCondition, nil, nil, nil,
		)
	}
	query := view.Condition()
	if query == nil {
		return validatedNode{}, validationError(
			node, weave.CodeInvalidValue, 0,
			weave.FeatureNativeCondition, nil, reflect.TypeOf(query), nil,
		)
	}
	validated.feature = weave.FeatureNativeCondition
	validated.native = query
	return validated, nil
}

func (compiler Compiler) validateNativeExpression(
	node weave.NodeView[Query, Expression],
	validated validatedNode,
) (validatedNode, error) {
	view, ok := node.AsNativeExpression()
	if !ok {
		return validatedNode{}, invalidPredicateError(node)
	}
	if !compiler.state.capabilities.Features.Has(weave.FeatureNativeExpression) {
		return validatedNode{}, validationError(
			node, weave.CodeUnsupportedFeature, 0,
			weave.FeatureNativeExpression, nil, nil, nil,
		)
	}
	expression := view.Expression()
	query, ok := safeQueryCaster(expression)
	if !ok {
		return validatedNode{}, validationError(
			node, weave.CodeInvalidValue, 0,
			weave.FeatureNativeExpression, nil, reflect.TypeOf(expression), nil,
		)
	}
	validated.feature = weave.FeatureNativeExpression
	validated.expression = query
	return validated, nil
}

func (compiler Compiler) validateField(
	node weave.NodeView[Query, Expression],
	value any,
	operator weave.Operator,
	valueType reflect.Type,
) (*fieldState, error) {
	fieldType := reflect.TypeOf(value)
	metadata, err := compiler.fieldMetadata(value)
	if err != nil {
		return nil, validationError(
			node, weave.CodeInvalidField, operator, 0,
			fieldType, valueType, nil,
		)
	}
	if !compiler.state.capabilities.Operators.Has(operator) {
		return nil, validationError(
			node, weave.CodeUnsupportedOperator, operator, 0,
			fieldType, valueType, nil,
		)
	}
	if !compiler.state.fieldCaps[metadata.state].Has(operator) {
		return nil, validationError(
			node, weave.CodeOperatorNotApplicable, operator, 0,
			fieldType, valueType, nil,
		)
	}
	return metadata.state, nil
}

func checkedScalarValue(
	field *fieldState,
	value any,
	reported reflect.Type,
) (scalarValue, bool) {
	if field == nil || reported == nil || isNilLike(value) {
		return scalarValue{}, false
	}
	dynamic := reflect.TypeOf(value)
	if dynamic == nil || dynamic != reported || dynamic != field.valueType {
		return scalarValue{}, false
	}
	return makeScalarValue(field.scalarType, reflect.ValueOf(value))
}

func reservedNullValue(field *fieldState, value scalarValue) bool {
	return field != nil && field.nullKind == NullValueMarker &&
		equalScalarValue(field.nullValue, value)
}

func equalScalarValue(left, right scalarValue) bool {
	if left.kind == 0 || left.kind != right.kind {
		return false
	}
	switch left.kind {
	case ScalarString:
		return left.stringValue == right.stringValue
	case ScalarInteger:
		return left.intValue == right.intValue
	case ScalarFloat:
		return left.floatValue == right.floatValue
	case ScalarDateTime:
		return left.timeValue.Equal(right.timeValue)
	case ScalarBoolean:
		return left.boolValue == right.boolValue
	default:
		return false
	}
}

func invalidPredicateError(node weave.NodeView[Query, Expression]) *weave.Error {
	return validationError(
		node, weave.CodeInvalidPredicate, 0, 0, nil, nil, nil,
	)
}

func validationError(
	node weave.NodeView[Query, Expression],
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

func comparisonOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorEQ, weave.OperatorNEQ, weave.OperatorLT,
		weave.OperatorLTE, weave.OperatorGT, weave.OperatorGTE:
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
	case weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix:
		return true
	default:
		return false
	}
}

func validLogic(logic weave.Logic) bool {
	switch logic {
	case weave.LogicAllOf, weave.LogicAnyOf,
		weave.LogicNoneOf, weave.LogicNotAllOf:
		return true
	default:
		return false
	}
}
