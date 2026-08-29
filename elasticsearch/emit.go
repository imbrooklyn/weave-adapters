package elasticsearch

import (
	"errors"
	"time"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave"
)

var errUnexpectedEmitPlan = errors.New(
	"elasticsearch: invalid validated emission plan",
)

func emitPredicate(validated validatedNode) (Query, error) {
	if validated.kind != weave.KindGroup ||
		validated.logic != weave.LogicAllOf {
		return nil, emissionError(validated)
	}
	children := make([]Query, len(validated.children))
	for index, child := range validated.children {
		if child.kind == weave.KindNativeCondition {
			if child.feature != weave.FeatureNativeCondition || child.native == nil {
				return nil, emissionError(child)
			}
			children[index] = child.native
			continue
		}
		query, err := emitExpression(child)
		if err != nil {
			return nil, err
		}
		if query == nil {
			return nil, emissionError(child)
		}
		children[index] = query
	}
	return combineLogic(weave.LogicAllOf, children, validated)
}

func emitExpression(validated validatedNode) (Query, error) {
	switch validated.kind {
	case weave.KindConstant:
		return constantQuery(validated.constant), nil
	case weave.KindGroup:
		if !validLogic(validated.logic) || len(validated.children) == 0 {
			return nil, emissionError(validated)
		}
		children := make([]Query, len(validated.children))
		for index, child := range validated.children {
			if child.kind == weave.KindNativeCondition {
				return nil, emissionError(child)
			}
			query, err := emitExpression(child)
			if err != nil {
				return nil, err
			}
			if query == nil {
				return nil, emissionError(child)
			}
			children[index] = query
		}
		return combineLogic(validated.logic, children, validated)
	case weave.KindComparison:
		if validated.field == nil || len(validated.values) != 1 {
			return nil, emissionError(validated)
		}
		return comparisonQuery(
			validated.operator, validated.field, validated.values[0], validated,
		)
	case weave.KindMembership:
		if validated.field == nil || len(validated.values) == 0 {
			return nil, emissionError(validated)
		}
		return membershipQuery(
			validated.operator, validated.field, validated.values, validated,
		)
	case weave.KindRange:
		if validated.operator != weave.OperatorBetween ||
			validated.field == nil || len(validated.values) != 2 {
			return nil, emissionError(validated)
		}
		rangeQuery, ok := betweenQuery(
			validated.field, validated.values[0], validated.values[1],
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return guardedPositiveQuery(validated.field, rangeQuery), nil
	case weave.KindNull:
		query, ok := nullQuery(validated.operator, validated.field)
		if !ok {
			return nil, emissionError(validated)
		}
		return query, nil
	case weave.KindText:
		query, ok := textQuery(
			validated.operator, validated.field, validated.text,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return query, nil
	case weave.KindNativeExpression:
		if validated.feature != weave.FeatureNativeExpression ||
			validated.expression == nil {
			return nil, emissionError(validated)
		}
		return validated.expression, nil
	default:
		return nil, emissionError(validated)
	}
}

func combineLogic(
	logic weave.Logic,
	children []Query,
	validated validatedNode,
) (Query, error) {
	if len(children) == 0 {
		switch logic {
		case weave.LogicAllOf, weave.LogicNoneOf:
			return constantQuery(true), nil
		case weave.LogicAnyOf, weave.LogicNotAllOf:
			return constantQuery(false), nil
		default:
			return nil, emissionError(validated)
		}
	}
	if len(children) == 1 && logic == weave.LogicAllOf {
		return children[0], nil
	}
	values, ok := queryValues(children)
	if !ok {
		return nil, emissionError(validated)
	}
	switch logic {
	case weave.LogicAllOf:
		return boolQuery(values, nil, nil), nil
	case weave.LogicAnyOf:
		return boolQuery(nil, nil, values), nil
	case weave.LogicNoneOf:
		return boolQuery(nil, values, nil), nil
	case weave.LogicNotAllOf:
		all := children[0]
		if len(children) > 1 {
			all = boolQuery(values, nil, nil)
		}
		return boolQuery(nil, []types.Query{*all}, nil), nil
	default:
		return nil, emissionError(validated)
	}
}

func comparisonQuery(
	operator weave.Operator,
	field *fieldState,
	value scalarValue,
	validated validatedNode,
) (Query, error) {
	var operation Query
	switch operator {
	case weave.OperatorEQ, weave.OperatorNEQ:
		operation = termQuery(field.path, value)
	case weave.OperatorLT, weave.OperatorLTE,
		weave.OperatorGT, weave.OperatorGTE:
		var ok bool
		operation, ok = comparisonRangeQuery(field, operator, value)
		if !ok {
			return nil, emissionError(validated)
		}
	default:
		return nil, emissionError(validated)
	}
	if operator == weave.OperatorNEQ {
		return guardedNegativeQuery(field, operation), nil
	}
	return guardedPositiveQuery(field, operation), nil
}

func membershipQuery(
	operator weave.Operator,
	field *fieldState,
	values []scalarValue,
	validated validatedNode,
) (Query, error) {
	operation := termsQuery(field.path, values)
	switch operator {
	case weave.OperatorIn:
		return guardedPositiveQuery(field, operation), nil
	case weave.OperatorNotIn:
		return guardedNegativeQuery(field, operation), nil
	default:
		return nil, emissionError(validated)
	}
}

func textQuery(
	operator weave.Operator,
	field *fieldState,
	text string,
) (Query, bool) {
	if field == nil {
		return nil, false
	}
	if text == "" {
		return valueGuard(field), true
	}
	var operation Query
	switch operator {
	case weave.OperatorContains:
		operation = wildcardQuery(field.path, "*"+escapeWildcardLiteral(text)+"*")
	case weave.OperatorHasPrefix:
		operation = prefixQuery(field.path, text)
	case weave.OperatorHasSuffix:
		operation = wildcardQuery(field.path, "*"+escapeWildcardLiteral(text))
	default:
		return nil, false
	}
	return guardedPositiveQuery(field, operation), true
}

func nullQuery(operator weave.Operator, field *fieldState) (Query, bool) {
	if field == nil {
		return nil, false
	}
	switch field.nullKind {
	case NullValueMarker:
		switch operator {
		case weave.OperatorIsNull:
			return termQuery(field.path, field.nullValue), true
		case weave.OperatorNotNull:
			return valueGuard(field), true
		}
	case NullCompanionMarker:
		if field.companion == nil {
			return nil, false
		}
		switch operator {
		case weave.OperatorIsNull:
			return rawStringTermQuery(
				field.companion.field.path, field.companion.nullTerm,
			), true
		case weave.OperatorNotNull:
			return rawStringTermQuery(
				field.companion.field.path, field.companion.valueTerm,
			), true
		}
	}
	return nil, false
}

func guardedPositiveQuery(field *fieldState, operation Query) Query {
	guard := valueGuard(field)
	if guard == nil || operation == nil {
		return nil
	}
	return boolQuery(
		[]types.Query{*guard, *operation}, nil, nil,
	)
}

func guardedNegativeQuery(field *fieldState, operation Query) Query {
	guard := valueGuard(field)
	if guard == nil || operation == nil {
		return nil
	}
	return boolQuery(
		[]types.Query{*guard}, []types.Query{*operation}, nil,
	)
}

func valueGuard(field *fieldState) Query {
	if field == nil {
		return nil
	}
	switch field.nullKind {
	case NullUntracked:
		return existsQuery(field.path)
	case NullValueMarker:
		return boolQuery(
			[]types.Query{*existsQuery(field.path)},
			[]types.Query{*termQuery(field.path, field.nullValue)},
			nil,
		)
	case NullCompanionMarker:
		if field.companion == nil {
			return nil
		}
		return rawStringTermQuery(
			field.companion.field.path, field.companion.valueTerm,
		)
	default:
		return nil
	}
}

func termQuery(path string, value scalarValue) Query {
	return &types.Query{Term: map[string]types.TermQuery{
		path: {Value: fieldValue(value)},
	}}
}

func rawStringTermQuery(path, value string) Query {
	return &types.Query{Term: map[string]types.TermQuery{
		path: {Value: value},
	}}
}

func termsQuery(path string, values []scalarValue) Query {
	fieldValues := make([]types.FieldValue, len(values))
	for index := range values {
		fieldValues[index] = fieldValue(values[index])
	}
	return &types.Query{Terms: &types.TermsQuery{
		TermsQuery: map[string]types.TermsQueryField{
			path: fieldValues,
		},
	}}
}

func existsQuery(path string) Query {
	return &types.Query{Exists: &types.ExistsQuery{Field: path}}
}

func prefixQuery(path, value string) Query {
	return &types.Query{Prefix: map[string]types.PrefixQuery{
		path: {Value: value},
	}}
}

func wildcardQuery(path, pattern string) Query {
	return &types.Query{Wildcard: map[string]types.WildcardQuery{
		path: {Value: &pattern},
	}}
}

func boolQuery(
	filter []types.Query,
	mustNot []types.Query,
	should []types.Query,
) Query {
	query := &types.BoolQuery{
		Filter:  filter,
		MustNot: mustNot,
		Should:  should,
	}
	if len(should) != 0 {
		query.MinimumShouldMatch = 1
	}
	return &types.Query{Bool: query}
}

func constantQuery(value bool) Query {
	if value {
		return &types.Query{MatchAll: &types.MatchAllQuery{}}
	}
	return &types.Query{MatchNone: &types.MatchNoneQuery{}}
}

func queryValues(queries []Query) ([]types.Query, bool) {
	values := make([]types.Query, len(queries))
	for index, query := range queries {
		if query == nil {
			return nil, false
		}
		values[index] = *query
	}
	return values, true
}

func fieldValue(value scalarValue) types.FieldValue {
	switch value.kind {
	case ScalarString:
		return value.stringValue
	case ScalarInteger:
		return value.intValue
	case ScalarFloat:
		return types.Float64(value.floatValue)
	case ScalarDateTime:
		return value.timeValue.UTC().Format(time.RFC3339Nano)
	case ScalarBoolean:
		return value.boolValue
	default:
		return nil
	}
}

func comparisonRangeQuery(
	field *fieldState,
	operator weave.Operator,
	value scalarValue,
) (Query, bool) {
	if field == nil || value.kind != field.scalarType {
		return nil, false
	}
	switch field.mappingType {
	case MappingLong:
		rangeValue := &types.LongNumberRangeQuery{}
		setLongBound(rangeValue, operator, value.intValue)
		union := rangeValue.RangeQueryCaster()
		return rangeContainer(field.path, union), union != nil
	case MappingDouble:
		number := types.Float64(value.floatValue)
		rangeValue := &types.NumberRangeQuery{}
		setNumberBound(rangeValue, operator, number)
		union := rangeValue.RangeQueryCaster()
		return rangeContainer(field.path, union), union != nil
	case MappingDate:
		date := value.timeValue.UTC().Format(time.RFC3339Nano)
		rangeValue := &types.DateRangeQuery{}
		setDateBound(rangeValue, operator, date)
		union := rangeValue.RangeQueryCaster()
		return rangeContainer(field.path, union), union != nil
	default:
		return nil, false
	}
}

func betweenQuery(field *fieldState, lower, upper scalarValue) (Query, bool) {
	if field == nil || lower.kind != field.scalarType || upper.kind != field.scalarType {
		return nil, false
	}
	switch field.mappingType {
	case MappingLong:
		rangeValue := &types.LongNumberRangeQuery{
			Gte: &lower.intValue,
			Lte: &upper.intValue,
		}
		union := rangeValue.RangeQueryCaster()
		return rangeContainer(field.path, union), union != nil
	case MappingDouble:
		lowerValue := types.Float64(lower.floatValue)
		upperValue := types.Float64(upper.floatValue)
		rangeValue := &types.NumberRangeQuery{
			Gte: &lowerValue,
			Lte: &upperValue,
		}
		union := rangeValue.RangeQueryCaster()
		return rangeContainer(field.path, union), union != nil
	default:
		return nil, false
	}
}

func rangeContainer(path string, value *types.RangeQuery) Query {
	if value == nil {
		return nil
	}
	return &types.Query{Range: map[string]types.RangeQuery{path: *value}}
}

func setLongBound(
	query *types.LongNumberRangeQuery,
	operator weave.Operator,
	value int64,
) {
	switch operator {
	case weave.OperatorLT:
		query.Lt = &value
	case weave.OperatorLTE:
		query.Lte = &value
	case weave.OperatorGT:
		query.Gt = &value
	case weave.OperatorGTE:
		query.Gte = &value
	}
}

func setNumberBound(
	query *types.NumberRangeQuery,
	operator weave.Operator,
	value types.Float64,
) {
	switch operator {
	case weave.OperatorLT:
		query.Lt = &value
	case weave.OperatorLTE:
		query.Lte = &value
	case weave.OperatorGT:
		query.Gt = &value
	case weave.OperatorGTE:
		query.Gte = &value
	}
}

func setDateBound(
	query *types.DateRangeQuery,
	operator weave.Operator,
	value string,
) {
	switch operator {
	case weave.OperatorLT:
		query.Lt = &value
	case weave.OperatorLTE:
		query.Lte = &value
	case weave.OperatorGT:
		query.Gt = &value
	case weave.OperatorGTE:
		query.Gte = &value
	}
}

func emissionError(validated validatedNode) *weave.Error {
	return &weave.Error{
		Code:     weave.CodeCompileFailure,
		Phase:    weave.PhaseEmit,
		Path:     validated.path,
		Origin:   validated.origin,
		Operator: validated.operator,
		Feature:  validated.feature,
		Cause:    errUnexpectedEmitPlan,
	}
}
