package memory_test

import (
	"math"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/memory"
)

type fuzzRecord struct {
	number      float64
	numberState memory.State
	text        string
	textState   memory.State
}

type fuzzFields struct {
	number memory.Field[fuzzRecord, float64]
	text   memory.Field[fuzzRecord, string]
}

type fuzzNodeKind uint8

const (
	fuzzConstantTrue fuzzNodeKind = iota
	fuzzConstantFalse
	fuzzEQ
	fuzzNEQ
	fuzzLT
	fuzzLTE
	fuzzGT
	fuzzGTE
	fuzzIn
	fuzzNotIn
	fuzzNullableIn
	fuzzBetween
	fuzzIsNull
	fuzzNotNull
	fuzzContains
	fuzzHasPrefix
	fuzzHasSuffix
	fuzzAllOf
	fuzzAnyOf
	fuzzNoneOf
	fuzzNotAllOf
)

const fuzzLeafKindCount = int(fuzzHasSuffix) + 1

type fuzzNode struct {
	kind     fuzzNodeKind
	number   float64
	numbers  []float64
	lower    float64
	upper    float64
	text     string
	children []*fuzzNode
}

type fuzzCursor struct {
	data  []byte
	index int
	nodes int
	depth int
}

// FuzzMemoryMatchesOracle compares compiled memory Conditions with an
// independent evaluator over the same generated predicate specification.
func FuzzMemoryMatchesOracle(f *testing.F) {
	fields := newFuzzFields(f)
	f.Add([]byte{})
	f.Add([]byte{0, 0, byte(fuzzNullableIn), 0, 1, 5})
	f.Add([]byte{0, 0, byte(fuzzLT), 5})
	f.Add([]byte{0, 0, byte(fuzzContains), 3})
	deepSeed := []byte{31, 0}
	for range 31 {
		deepSeed = append(deepSeed, byte(fuzzAllOf), 0)
	}
	deepSeed = append(deepSeed, byte(fuzzEQ), 2)
	f.Add(deepSeed)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 512 {
			data = data[:512]
		}
		specification := generateFuzzTree(data)
		factory := memory.NewFactory[fuzzRecord]()
		root := rootFuzzSink{builder: factory.New()}
		for _, child := range specification.children {
			emitFuzzNode(root, fields, child)
		}
		condition, err := root.builder.Build()
		if err != nil {
			t.Fatalf("Build() error = %v for input %x", err, data)
		}

		for index, record := range fuzzRecords() {
			want := evaluateFuzzNode(specification, record)
			got, err := condition.Match(record)
			if err != nil {
				t.Fatalf(
					"Match(record %d) error = %v for input %x",
					index,
					err,
					data,
				)
			}
			if got != want {
				t.Fatalf(
					"Match(record %d) = %v, want %v for input %x",
					index,
					got,
					want,
					data,
				)
			}
		}
	})
}

func newFuzzFields(t testing.TB) fuzzFields {
	t.Helper()
	number, err := memory.NewField(
		"number",
		func(record fuzzRecord) (float64, memory.State) {
			return record.number, record.numberState
		},
		memory.OrderedSemantics[float64](),
	)
	if err != nil {
		t.Fatalf("NewField(number) error = %v", err)
	}
	text, err := memory.NewField(
		"text",
		func(record fuzzRecord) (string, memory.State) {
			return record.text, record.textState
		},
		memory.StringSemantics(),
	)
	if err != nil {
		t.Fatalf("NewField(text) error = %v", err)
	}
	return fuzzFields{number: number, text: text}
}

func fuzzRecords() []fuzzRecord {
	return []fuzzRecord{
		{number: -1, numberState: memory.StateValue, text: "", textState: memory.StateValue},
		{number: 0, numberState: memory.StateValue, text: "plain", textState: memory.StateValue},
		{
			number:      math.NaN(),
			numberState: memory.StateValue,
			text:        "%_! .*+?[](){}^$|\\",
			textState:   memory.StateValue,
		},
		{
			numberState: memory.StateNull,
			text:        "\u4e16\u754c\nend",
			textState:   memory.StateValue,
		},
		{numberState: memory.StateMissing, textState: memory.StateNull},
		{number: 5, numberState: memory.StateValue, textState: memory.StateMissing},
	}
}

func generateFuzzTree(data []byte) *fuzzNode {
	cursor := &fuzzCursor{data: data}
	cursor.depth = 1 + int(cursor.next()%32)
	children := 1 + int(cursor.next()%4)
	root := &fuzzNode{kind: fuzzAllOf, children: make([]*fuzzNode, 0, children)}
	for range children {
		root.children = append(root.children, cursor.node(1))
	}
	return root
}

func (c *fuzzCursor) node(depth int) *fuzzNode {
	c.nodes++
	kindValue := int(c.next()) % (int(fuzzNotAllOf) + 1)
	if depth >= c.depth || c.nodes >= 96 {
		kindValue %= fuzzLeafKindCount
	}
	kind := fuzzNodeKind(kindValue)
	node := &fuzzNode{kind: kind}
	switch kind {
	case fuzzEQ, fuzzNEQ, fuzzLT, fuzzLTE, fuzzGT, fuzzGTE:
		node.number = fuzzNumber(c.next())
	case fuzzIn, fuzzNotIn:
		node.numbers = c.numberSlice(false)
	case fuzzNullableIn:
		node.numbers = c.numberSlice(true)
	case fuzzBetween:
		node.lower = fuzzFiniteNumber(c.next())
		node.upper = fuzzFiniteNumber(c.next())
		if node.lower > node.upper {
			node.lower, node.upper = node.upper, node.lower
		}
	case fuzzContains, fuzzHasPrefix, fuzzHasSuffix:
		node.text = fuzzText(c.next())
	case fuzzAllOf, fuzzAnyOf, fuzzNoneOf, fuzzNotAllOf:
		childCount := 1 + int(c.next()%3)
		node.children = make([]*fuzzNode, 0, childCount)
		for range childCount {
			node.children = append(node.children, c.node(depth+1))
		}
	}
	return node
}

func (c *fuzzCursor) numberSlice(nullable bool) []float64 {
	count := 1 + int(c.next()%4)
	if nullable {
		count = int(c.next() % 4)
	}
	values := make([]float64, count)
	for index := range values {
		values[index] = fuzzNumber(c.next())
	}
	return values
}

func (c *fuzzCursor) next() byte {
	if len(c.data) != 0 {
		value := c.data[c.index%len(c.data)]
		c.index++
		return value
	}
	value := byte(c.index*37 + 11)
	c.index++
	return value
}

func fuzzNumber(value byte) float64 {
	switch value % 8 {
	case 0:
		return -2
	case 1:
		return -1
	case 2:
		return 0
	case 3:
		return 1
	case 4:
		return 2
	case 5:
		return math.NaN()
	case 6:
		return math.Inf(-1)
	default:
		return math.Inf(1)
	}
}

func fuzzFiniteNumber(value byte) float64 {
	return float64(int8(value%11) - 5)
}

func fuzzText(value byte) string {
	return [...]string{
		"",
		"%_!",
		".*+?[](){}^$|\\",
		"\u4e16\u754c",
		"\n",
		"plain",
		"end",
	}[value%7]
}

type fuzzSink interface {
	constant(bool)
	comparison(weave.Operator, any, float64)
	membership(weave.Operator, any, []float64, bool)
	between(any, float64, float64)
	null(weave.Operator, any)
	text(weave.Operator, any, string)
	group(weave.Logic, func(fuzzSink))
}

type rootFuzzSink struct {
	builder *weave.Builder[
		memory.Condition[fuzzRecord],
		memory.Expression[fuzzRecord],
	]
}

type groupFuzzSink struct {
	value *weave.Group[memory.Expression[fuzzRecord]]
}

func emitFuzzNode(sink fuzzSink, fields fuzzFields, node *fuzzNode) {
	switch node.kind {
	case fuzzConstantTrue:
		sink.constant(true)
	case fuzzConstantFalse:
		sink.constant(false)
	case fuzzEQ:
		sink.comparison(weave.OperatorEQ, fields.number, node.number)
	case fuzzNEQ:
		sink.comparison(weave.OperatorNEQ, fields.number, node.number)
	case fuzzLT:
		sink.comparison(weave.OperatorLT, fields.number, node.number)
	case fuzzLTE:
		sink.comparison(weave.OperatorLTE, fields.number, node.number)
	case fuzzGT:
		sink.comparison(weave.OperatorGT, fields.number, node.number)
	case fuzzGTE:
		sink.comparison(weave.OperatorGTE, fields.number, node.number)
	case fuzzIn:
		sink.membership(weave.OperatorIn, fields.number, node.numbers, false)
	case fuzzNotIn:
		sink.membership(weave.OperatorNotIn, fields.number, node.numbers, false)
	case fuzzNullableIn:
		sink.membership(weave.OperatorIn, fields.number, node.numbers, true)
	case fuzzBetween:
		sink.between(fields.number, node.lower, node.upper)
	case fuzzIsNull:
		sink.null(weave.OperatorIsNull, fields.number)
	case fuzzNotNull:
		sink.null(weave.OperatorNotNull, fields.number)
	case fuzzContains:
		sink.text(weave.OperatorContains, fields.text, node.text)
	case fuzzHasPrefix:
		sink.text(weave.OperatorHasPrefix, fields.text, node.text)
	case fuzzHasSuffix:
		sink.text(weave.OperatorHasSuffix, fields.text, node.text)
	case fuzzAllOf, fuzzAnyOf, fuzzNoneOf, fuzzNotAllOf:
		sink.group(fuzzLogic(node.kind), func(childSink fuzzSink) {
			for _, child := range node.children {
				emitFuzzNode(childSink, fields, child)
			}
		})
	}
}

func (sink rootFuzzSink) constant(value bool) {
	if value {
		sink.builder.AllOf(func(*weave.Group[memory.Expression[fuzzRecord]]) {})
		return
	}
	sink.builder.AnyOf(func(*weave.Group[memory.Expression[fuzzRecord]]) {})
}

func (sink rootFuzzSink) comparison(operator weave.Operator, field any, value float64) {
	addRootComparison(sink.builder, operator, field, value)
}

func (sink rootFuzzSink) membership(
	operator weave.Operator,
	field any,
	values []float64,
	nullable bool,
) {
	addRootMembership(sink.builder, operator, field, values, nullable)
}

func (sink rootFuzzSink) between(field any, lower, upper float64) {
	sink.builder.Between(field, lower, upper)
}

func (sink rootFuzzSink) null(operator weave.Operator, field any) {
	if operator == weave.OperatorIsNull {
		sink.builder.IsNull(field)
		return
	}
	sink.builder.NotNull(field)
}

func (sink rootFuzzSink) text(operator weave.Operator, field any, value string) {
	addRootText(sink.builder, operator, field, value)
}

func (sink rootFuzzSink) group(logic weave.Logic, scope func(fuzzSink)) {
	addRootGroup(sink.builder, logic, scope)
}

func (sink groupFuzzSink) constant(value bool) {
	if value {
		sink.value.AllOf(func(*weave.Group[memory.Expression[fuzzRecord]]) {})
		return
	}
	sink.value.AnyOf(func(*weave.Group[memory.Expression[fuzzRecord]]) {})
}

func (sink groupFuzzSink) comparison(operator weave.Operator, field any, value float64) {
	addGroupComparison(sink.value, operator, field, value)
}

func (sink groupFuzzSink) membership(
	operator weave.Operator,
	field any,
	values []float64,
	nullable bool,
) {
	addGroupMembership(sink.value, operator, field, values, nullable)
}

func (sink groupFuzzSink) between(field any, lower, upper float64) {
	sink.value.Between(field, lower, upper)
}

func (sink groupFuzzSink) null(operator weave.Operator, field any) {
	if operator == weave.OperatorIsNull {
		sink.value.IsNull(field)
		return
	}
	sink.value.NotNull(field)
}

func (sink groupFuzzSink) text(operator weave.Operator, field any, value string) {
	addGroupText(sink.value, operator, field, value)
}

func (sink groupFuzzSink) group(logic weave.Logic, scope func(fuzzSink)) {
	addNestedGroup(sink.value, logic, scope)
}

func addRootComparison(
	builder *weave.Builder[memory.Condition[fuzzRecord], memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	value float64,
) {
	switch operator {
	case weave.OperatorEQ:
		builder.EQ(field, value)
	case weave.OperatorNEQ:
		builder.NEQ(field, value)
	case weave.OperatorLT:
		builder.LT(field, value)
	case weave.OperatorLTE:
		builder.LTE(field, value)
	case weave.OperatorGT:
		builder.GT(field, value)
	case weave.OperatorGTE:
		builder.GTE(field, value)
	}
}

func addGroupComparison(
	group *weave.Group[memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	value float64,
) {
	switch operator {
	case weave.OperatorEQ:
		group.EQ(field, value)
	case weave.OperatorNEQ:
		group.NEQ(field, value)
	case weave.OperatorLT:
		group.LT(field, value)
	case weave.OperatorLTE:
		group.LTE(field, value)
	case weave.OperatorGT:
		group.GT(field, value)
	case weave.OperatorGTE:
		group.GTE(field, value)
	}
}

func addRootMembership(
	builder *weave.Builder[memory.Condition[fuzzRecord], memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	values []float64,
	nullable bool,
) {
	if nullable {
		builder.In(field, nullableValues(values))
		return
	}
	if operator == weave.OperatorIn {
		builder.In(field, values)
		return
	}
	builder.NotIn(field, values)
}

func addGroupMembership(
	group *weave.Group[memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	values []float64,
	nullable bool,
) {
	if nullable {
		group.In(field, nullableValues(values))
		return
	}
	if operator == weave.OperatorIn {
		group.In(field, values)
		return
	}
	group.NotIn(field, values)
}

func nullableValues(values []float64) []*float64 {
	nullable := make([]*float64, 0, len(values)+1)
	for _, value := range values {
		cloned := value
		nullable = append(nullable, &cloned)
	}
	return append(nullable, nil)
}

func addRootText(
	builder *weave.Builder[memory.Condition[fuzzRecord], memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	value string,
) {
	switch operator {
	case weave.OperatorContains:
		builder.Contains(field, value)
	case weave.OperatorHasPrefix:
		builder.HasPrefix(field, value)
	case weave.OperatorHasSuffix:
		builder.HasSuffix(field, value)
	}
}

func addGroupText(
	group *weave.Group[memory.Expression[fuzzRecord]],
	operator weave.Operator,
	field any,
	value string,
) {
	switch operator {
	case weave.OperatorContains:
		group.Contains(field, value)
	case weave.OperatorHasPrefix:
		group.HasPrefix(field, value)
	case weave.OperatorHasSuffix:
		group.HasSuffix(field, value)
	}
}

func addRootGroup(
	builder *weave.Builder[memory.Condition[fuzzRecord], memory.Expression[fuzzRecord]],
	logic weave.Logic,
	scope func(fuzzSink),
) {
	wrapped := func(group *weave.Group[memory.Expression[fuzzRecord]]) {
		scope(groupFuzzSink{value: group})
	}
	switch logic {
	case weave.LogicAllOf:
		builder.AllOf(wrapped)
	case weave.LogicAnyOf:
		builder.AnyOf(wrapped)
	case weave.LogicNoneOf:
		builder.NoneOf(wrapped)
	case weave.LogicNotAllOf:
		builder.NotAllOf(wrapped)
	}
}

func addNestedGroup(
	group *weave.Group[memory.Expression[fuzzRecord]],
	logic weave.Logic,
	scope func(fuzzSink),
) {
	wrapped := func(child *weave.Group[memory.Expression[fuzzRecord]]) {
		scope(groupFuzzSink{value: child})
	}
	switch logic {
	case weave.LogicAllOf:
		group.AllOf(wrapped)
	case weave.LogicAnyOf:
		group.AnyOf(wrapped)
	case weave.LogicNoneOf:
		group.NoneOf(wrapped)
	case weave.LogicNotAllOf:
		group.NotAllOf(wrapped)
	}
}

func fuzzLogic(kind fuzzNodeKind) weave.Logic {
	switch kind {
	case fuzzAllOf:
		return weave.LogicAllOf
	case fuzzAnyOf:
		return weave.LogicAnyOf
	case fuzzNoneOf:
		return weave.LogicNoneOf
	default:
		return weave.LogicNotAllOf
	}
}

func evaluateFuzzNode(node *fuzzNode, record fuzzRecord) bool {
	switch node.kind {
	case fuzzConstantTrue:
		return true
	case fuzzConstantFalse:
		return false
	case fuzzEQ:
		return record.numberState == memory.StateValue && record.number == node.number
	case fuzzNEQ:
		return record.numberState == memory.StateValue && record.number != node.number
	case fuzzLT:
		return record.numberState == memory.StateValue && record.number < node.number
	case fuzzLTE:
		return record.numberState == memory.StateValue && record.number <= node.number
	case fuzzGT:
		return record.numberState == memory.StateValue && record.number > node.number
	case fuzzGTE:
		return record.numberState == memory.StateValue && record.number >= node.number
	case fuzzIn:
		return record.numberState == memory.StateValue &&
			fuzzContainsNumber(node.numbers, record.number)
	case fuzzNotIn:
		return record.numberState == memory.StateValue &&
			!fuzzContainsNumber(node.numbers, record.number)
	case fuzzNullableIn:
		if record.numberState == memory.StateNull {
			return true
		}
		return record.numberState == memory.StateValue &&
			fuzzContainsNumber(node.numbers, record.number)
	case fuzzBetween:
		return record.numberState == memory.StateValue &&
			record.number >= node.lower &&
			record.number <= node.upper
	case fuzzIsNull:
		return record.numberState == memory.StateNull
	case fuzzNotNull:
		return record.numberState == memory.StateValue
	case fuzzContains:
		return record.textState == memory.StateValue && strings.Contains(record.text, node.text)
	case fuzzHasPrefix:
		return record.textState == memory.StateValue && strings.HasPrefix(record.text, node.text)
	case fuzzHasSuffix:
		return record.textState == memory.StateValue && strings.HasSuffix(record.text, node.text)
	case fuzzAllOf:
		for _, child := range node.children {
			if !evaluateFuzzNode(child, record) {
				return false
			}
		}
		return true
	case fuzzAnyOf:
		for _, child := range node.children {
			if evaluateFuzzNode(child, record) {
				return true
			}
		}
		return false
	case fuzzNoneOf:
		for _, child := range node.children {
			if evaluateFuzzNode(child, record) {
				return false
			}
		}
		return true
	case fuzzNotAllOf:
		for _, child := range node.children {
			if !evaluateFuzzNode(child, record) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func fuzzContainsNumber(values []float64, candidate float64) bool {
	for _, value := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
