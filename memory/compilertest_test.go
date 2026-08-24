package memory_test

import (
	"testing"

	"github.com/imbrooklyn/weave-adapters/memory"
	"github.com/imbrooklyn/weave/compilertest"
)

func TestCompilerSemantics(t *testing.T) {
	records := compilertest.Records()
	factory := memory.NewFactory[compilertest.Record]()
	inspectionCalls := 0

	number := mustField(
		t,
		"number",
		func(record compilertest.Record) (int64, memory.State) {
			return record.Number, memory.StateValue
		},
		memory.OrderedSemantics[int64](),
	)
	text := mustField(
		t,
		"text",
		func(record compilertest.Record) (string, memory.State) {
			return record.Text, memory.StateValue
		},
		memory.StringSemantics(),
	)
	nullableNumber := mustField(
		t,
		"nullable_number",
		func(record compilertest.Record) (int64, memory.State) {
			if !record.NullableNumberPresent {
				return 0, memory.StateMissing
			}
			if record.NullableNumber == nil {
				return 0, memory.StateNull
			}
			return *record.NullableNumber, memory.StateValue
		},
		memory.OrderedSemantics[int64](),
	)
	nullableText := mustField(
		t,
		"nullable_text",
		func(record compilertest.Record) (string, memory.State) {
			if !record.NullableTextPresent {
				return "", memory.StateMissing
			}
			if record.NullableText == nil {
				return "", memory.StateNull
			}
			return *record.NullableText, memory.StateValue
		},
		memory.StringSemantics(),
	)
	equalityOnlyText := mustField(
		t,
		"equality_only_text",
		func(record compilertest.Record) (string, memory.State) {
			return record.Text, memory.StateValue
		},
		memory.ComparableSemantics[string](),
	)

	compilertest.Run(t, compilertest.Harness[
		memory.Condition[compilertest.Record],
		memory.Expression[compilertest.Record],
	]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           number,
			Text:             text,
			NullableNumber:   nullableNumber,
			NullableText:     nullableText,
			EqualityOnlyText: equalityOnlyText,
		},
		Resolver: memory.NewCompiler[compilertest.Record](),
		InspectCondition: func(
			_ string,
			condition memory.Condition[compilertest.Record],
		) error {
			inspectionCalls++
			if condition == nil {
				return memory.ErrNilCondition
			}
			return nil
		},
		Execute: func(
			condition memory.Condition[compilertest.Record],
		) ([]string, error) {
			var ids []string
			for _, record := range records {
				matched, err := condition.Match(record)
				if err != nil {
					return nil, err
				}
				if matched {
					ids = append(ids, record.ID)
				}
			}
			return ids, nil
		},
		NativeCondition: func(ids []string) memory.Condition[compilertest.Record] {
			matches := idSet(ids)
			return func(record compilertest.Record) (bool, error) {
				_, matched := matches[record.ID]
				return matched, nil
			}
		},
		NativeExpression: func(ids []string) memory.Expression[compilertest.Record] {
			matches := idSet(ids)
			return func(record compilertest.Record) (bool, error) {
				_, matched := matches[record.ID]
				return matched, nil
			}
		},
		NilLikeNativeCondition: func() memory.Condition[compilertest.Record] {
			return nil
		},
		NilLikeNativeExpression: func() memory.Expression[compilertest.Record] {
			return nil
		},
		DistinguishesMissing: true,
	})
	if inspectionCalls == 0 {
		t.Fatal("compilertest did not invoke InspectCondition")
	}
}

func mustField[R, V any](
	t testing.TB,
	name string,
	accessor memory.Accessor[R, V],
	semantics memory.Semantics[V],
) memory.Field[R, V] {
	t.Helper()
	field, err := memory.NewField(name, accessor, semantics)
	if err != nil {
		t.Fatalf("NewField(%q) error = %v", name, err)
	}
	return field
}

func idSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
