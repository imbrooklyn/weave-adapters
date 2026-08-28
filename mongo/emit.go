package mongo

import (
	"errors"
	"regexp"

	"github.com/imbrooklyn/weave"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var errUnexpectedEmitPlan = errors.New("mongo: invalid validated emission plan")

func emitPredicate(validated validatedNode) (Filter, error) {
	if validated.kind != weave.KindGroup ||
		validated.logic != weave.LogicAllOf {
		return nil, emissionError(validated)
	}

	documents := make([]bson.D, len(validated.children))
	for index, child := range validated.children {
		if child.kind == weave.KindNativeCondition {
			if child.feature != weave.FeatureNativeCondition || child.native == nil {
				return nil, emissionError(child)
			}
			documents[index] = cloneDocument(child.native)
			continue
		}
		document, err := emitExpression(child)
		if err != nil {
			return nil, err
		}
		if document == nil {
			return nil, emissionError(child)
		}
		documents[index] = document
	}

	switch len(documents) {
	case 0:
		return bson.D{}, nil
	case 1:
		return cloneDocument(documents[0]), nil
	default:
		return operatorDocument("$and", documentArray(documents)), nil
	}
}

func emitExpression(validated validatedNode) (bson.D, error) {
	switch validated.kind {
	case weave.KindConstant:
		return constantDocument(validated.constant), nil

	case weave.KindGroup:
		if !validLogic(validated.logic) {
			return nil, emissionError(validated)
		}
		children := make([]bson.D, len(validated.children))
		for index := range validated.children {
			if validated.children[index].kind == weave.KindNativeCondition {
				return nil, emissionError(validated.children[index])
			}
			child, err := emitExpression(validated.children[index])
			if err != nil {
				return nil, err
			}
			if child == nil {
				return nil, emissionError(validated.children[index])
			}
			children[index] = child
		}
		return combineDocuments(validated.logic, children), nil

	case weave.KindComparison:
		if len(validated.values) != 1 {
			return nil, emissionError(validated)
		}
		operation, ok := comparisonDocument(
			validated.operator,
			validated.fieldPath,
			validated.values[0],
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return guardedDocument(validated.fieldPath, operation), nil

	case weave.KindMembership:
		operation, ok := membershipDocument(
			validated.operator,
			validated.fieldPath,
			validated.values,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return guardedDocument(validated.fieldPath, operation), nil

	case weave.KindRange:
		if validated.operator != weave.OperatorBetween ||
			len(validated.values) != 2 {
			return nil, emissionError(validated)
		}
		operation := fieldDocument(
			validated.fieldPath,
			bson.D{
				{Key: "$gte", Value: validated.values[0]},
				{Key: "$lte", Value: validated.values[1]},
			},
		)
		return guardedDocument(validated.fieldPath, operation), nil

	case weave.KindNull:
		document, ok := nullDocument(validated.operator, validated.fieldPath)
		if !ok {
			return nil, emissionError(validated)
		}
		return document, nil

	case weave.KindText:
		document, ok := textDocument(
			validated.operator,
			validated.fieldPath,
			validated.text,
		)
		if !ok {
			return nil, emissionError(validated)
		}
		return document, nil

	case weave.KindNativeExpression:
		if validated.feature != weave.FeatureNativeExpression ||
			validated.expression == nil {
			return nil, emissionError(validated)
		}
		return cloneDocument(validated.expression), nil

	default:
		return nil, emissionError(validated)
	}
}

func comparisonDocument(
	operator weave.Operator,
	path string,
	value any,
) (bson.D, bool) {
	var mongoOperator string
	switch operator {
	case weave.OperatorEQ:
		mongoOperator = "$eq"
	case weave.OperatorNEQ:
		mongoOperator = "$ne"
	case weave.OperatorLT:
		mongoOperator = "$lt"
	case weave.OperatorLTE:
		mongoOperator = "$lte"
	case weave.OperatorGT:
		mongoOperator = "$gt"
	case weave.OperatorGTE:
		mongoOperator = "$gte"
	default:
		return nil, false
	}
	return fieldDocument(path, operatorDocument(mongoOperator, value)), true
}

func membershipDocument(
	operator weave.Operator,
	path string,
	values []any,
) (bson.D, bool) {
	if len(values) == 0 {
		return nil, false
	}
	var mongoOperator string
	switch operator {
	case weave.OperatorIn:
		mongoOperator = "$in"
	case weave.OperatorNotIn:
		mongoOperator = "$nin"
	default:
		return nil, false
	}
	array := make(bson.A, len(values))
	copy(array, values)
	return fieldDocument(path, operatorDocument(mongoOperator, array)), true
}

func nullDocument(operator weave.Operator, path string) (bson.D, bool) {
	var operation bson.D
	switch operator {
	case weave.OperatorIsNull:
		operation = fieldDocument(path, operatorDocument("$eq", nil))
	case weave.OperatorNotNull:
		operation = nonNullDocument(path)
	default:
		return nil, false
	}
	return operatorDocument("$and", bson.A{
		existsDocument(path),
		operation,
	}), true
}

func textDocument(operator weave.Operator, path, text string) (bson.D, bool) {
	quoted := regexp.QuoteMeta(text)
	var pattern string
	switch operator {
	case weave.OperatorContains:
		pattern = quoted
	case weave.OperatorHasPrefix:
		pattern = `\A` + quoted
	case weave.OperatorHasSuffix:
		pattern = quoted + `\z`
	default:
		return nil, false
	}
	operation := fieldDocument(
		path,
		operatorDocument("$regex", pattern),
	)
	return guardedDocument(path, operation), true
}

func guardedDocument(path string, operation bson.D) bson.D {
	return operatorDocument("$and", bson.A{
		existsDocument(path),
		nonNullDocument(path),
		operation,
	})
}

func existsDocument(path string) bson.D {
	return fieldDocument(path, operatorDocument("$exists", true))
}

func nonNullDocument(path string) bson.D {
	return fieldDocument(path, operatorDocument("$ne", nil))
}

func fieldDocument(path string, value any) bson.D {
	return bson.D{{Key: path, Value: value}}
}

func operatorDocument(operator string, value any) bson.D {
	return bson.D{{Key: operator, Value: value}}
}

func constantDocument(value bool) bson.D {
	if value {
		return bson.D{}
	}
	return operatorDocument("$expr", false)
}

func combineDocuments(logic weave.Logic, children []bson.D) bson.D {
	if len(children) == 0 {
		switch logic {
		case weave.LogicAllOf, weave.LogicNoneOf:
			return constantDocument(true)
		case weave.LogicAnyOf, weave.LogicNotAllOf:
			return constantDocument(false)
		default:
			return nil
		}
	}
	array := documentArray(children)
	switch logic {
	case weave.LogicAllOf:
		return operatorDocument("$and", array)
	case weave.LogicAnyOf:
		return operatorDocument("$or", array)
	case weave.LogicNoneOf:
		return operatorDocument("$nor", array)
	case weave.LogicNotAllOf:
		return operatorDocument("$nor", bson.A{
			operatorDocument("$and", array),
		})
	default:
		return nil
	}
}

func documentArray(documents []bson.D) bson.A {
	array := make(bson.A, len(documents))
	for index := range documents {
		array[index] = documents[index]
	}
	return array
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
