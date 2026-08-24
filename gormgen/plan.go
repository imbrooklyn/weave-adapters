package gormgen

import (
	"github.com/imbrooklyn/weave"
	"gorm.io/gen/field"
	"gorm.io/gorm/clause"
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
	column     clause.Column
	values     []any
	text       string
	native     Conditions
	expression field.Expr
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
