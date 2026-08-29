package elasticsearch

import (
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave"
)

type validatedNode struct {
	kind       weave.Kind
	path       weave.NodePath
	origin     weave.Origin
	operator   weave.Operator
	feature    weave.Feature
	constant   bool
	logic      weave.Logic
	children   []validatedNode
	field      *fieldState
	values     []scalarValue
	text       string
	native     Query
	expression Query
}

func requirementsForPlan(root validatedNode) weave.Requirements {
	operators := make([]weave.Operator, 0)
	features := make([]weave.Feature, 0)
	stack := []validatedNode{root}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.operator != 0 {
			operators = append(operators, current.operator)
		}
		if current.feature != 0 {
			features = append(features, current.feature)
		}
		for index := len(current.children) - 1; index >= 0; index-- {
			stack = append(stack, current.children[index])
		}
	}
	return weave.Requirements{
		Operators: weave.NewOperatorSet(operators...),
		Features:  weave.NewFeatureSet(features...),
	}
}

func equalRequirements(left, right weave.Requirements) bool {
	return left.Operators.Count() == right.Operators.Count() &&
		left.Operators.ContainsAll(right.Operators) &&
		left.Features.Count() == right.Features.Count() &&
		left.Features.ContainsAll(right.Features)
}

func safeQueryCaster(expression Expression) (query *types.Query, ok bool) {
	if isNilLike(expression) {
		return nil, false
	}
	defer func() {
		if recover() != nil {
			query = nil
			ok = false
		}
	}()
	query = expression.QueryCaster()
	return query, query != nil
}
