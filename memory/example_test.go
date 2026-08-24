package memory_test

import (
	"fmt"

	"github.com/imbrooklyn/weave-adapters/memory"
)

func ExampleCompiler() {
	type user struct {
		name string
		age  int
	}
	age, err := memory.NewField(
		"age",
		func(value user) (int, memory.State) {
			return value.age, memory.StateValue
		},
		memory.OrderedSemantics[int](),
	)
	if err != nil {
		panic(err)
	}
	name, err := memory.NewField(
		"name",
		func(value user) (string, memory.State) {
			return value.name, memory.StateValue
		},
		memory.StringSemantics(),
	)
	if err != nil {
		panic(err)
	}

	condition, err := memory.NewFactory[user]().New().
		GTE(age, 18).
		HasPrefix(name, "A").
		Build()
	if err != nil {
		panic(err)
	}
	for _, value := range []user{
		{name: "Ada", age: 36},
		{name: "Alan", age: 17},
		{name: "Grace", age: 37},
	} {
		matched, matchErr := condition.Match(value)
		if matchErr != nil {
			panic(matchErr)
		}
		if matched {
			fmt.Println(value.name)
		}
	}

	// Output:
	// Ada
}

func ExampleState() {
	type document struct {
		score   *int
		present bool
	}
	score, err := memory.NewField(
		"score",
		func(value document) (int, memory.State) {
			if !value.present {
				return 0, memory.StateMissing
			}
			if value.score == nil {
				return 0, memory.StateNull
			}
			return *value.score, memory.StateValue
		},
		memory.OrderedSemantics[int](),
	)
	if err != nil {
		panic(err)
	}
	condition, err := memory.NewFactory[document]().New().IsNull(score).Build()
	if err != nil {
		panic(err)
	}

	value := 7
	for _, test := range []struct {
		name  string
		value document
	}{
		{name: "explicit null", value: document{present: true}},
		{name: "missing", value: document{}},
		{name: "value", value: document{score: &value, present: true}},
	} {
		matched, matchErr := condition.Match(test.value)
		if matchErr != nil {
			panic(matchErr)
		}
		fmt.Printf("%s: %t\n", test.name, matched)
	}

	// Output:
	// explicit null: true
	// missing: false
	// value: false
}
