package elasticsearch

import (
	"errors"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
)

func TestFieldMappingRegistryAndCapabilities(t *testing.T) {
	marker := mustField(t, FieldSpec[string]{
		Path:               "state.name",
		Type:               MappingKeyword,
		CompleteValueIndex: true,
	})
	companion, err := NewCompanionMarker(marker, "null", "value")
	if err != nil {
		t.Fatalf("NewCompanionMarker: %v", err)
	}

	name := mustField(t, FieldSpec[string]{
		Path:                   "name.keyword",
		Type:                   MappingKeyword,
		CompleteValueIndex:     true,
		Normalizer:             "lowercase",
		AllowExpensiveWildcard: true,
		Nulls:                  MarkNullWith[string](companion),
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorIsNull,
			weave.OperatorNotNull,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		),
	})
	age := mustField(t, FieldSpec[int64]{
		Path:               "age",
		Type:               MappingLong,
		CompleteValueIndex: true,
		Nulls:              IndexNullAs[int64](-1),
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorBetween,
			weave.OperatorIsNull,
			weave.OperatorNotNull,
		),
	})
	createdAt := mustField(t, FieldSpec[time.Time]{
		Path:               "created_at",
		Type:               MappingDate,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
		),
	})
	tags := mustField(t, FieldSpec[string]{
		Path:               "tags",
		Type:               MappingKeyword,
		MultiValued:        true,
		CompleteValueIndex: true,
	})
	body := mustField(t, FieldSpec[string]{
		Path:               "body",
		Type:               MappingText,
		CompleteValueIndex: true,
	})

	mapping, err := NewMapping(marker, name, age, createdAt, tags, body)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	if !mapping.Valid() || mapping.FieldCount() != 6 ||
		!mapping.HasPath("name.keyword") || mapping.HasPath("missing") {
		t.Fatalf("unexpected mapping registry state")
	}

	expensive, err := NewCompiler(Elasticsearch95ExpensiveQueries, mapping)
	if err != nil {
		t.Fatalf("NewCompiler expensive: %v", err)
	}
	strict, err := NewCompiler(Elasticsearch95NoExpensiveQueries, mapping)
	if err != nil {
		t.Fatalf("NewCompiler strict: %v", err)
	}

	requireCapabilities(t, expensive, name,
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	)
	requireCapabilities(t, strict, name,
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	)
	requireCapabilities(t, expensive, tags)
	requireCapabilities(t, expensive, body)
	requireCapabilities(t, expensive, createdAt,
		weave.OperatorEQ,
		weave.OperatorLT,
		weave.OperatorLTE,
		weave.OperatorGT,
		weave.OperatorGTE,
	)

	if name.Path() != "name.keyword" || name.MappingType() != MappingKeyword ||
		name.ScalarType() != ScalarString || !name.Keyword() || name.Analyzed() ||
		name.MultiValued() || name.Nested() || !name.CompleteValueIndex() ||
		name.Normalizer() != "lowercase" || !name.AllowsExpensiveWildcard() ||
		name.NullKind() != NullCompanionMarker ||
		name.NullMarkerPath() != "state.name" {
		t.Fatalf("unexpected name descriptor metadata")
	}
	if age.NullKind() != NullValueMarker || age.NullMarkerPath() != "" {
		t.Fatalf("unexpected age null metadata")
	}
	if !tags.MultiValued() || tags.Capabilities().Operators.Count() != 0 {
		t.Fatalf("multi-valued field exposed standard operators")
	}
	if !body.Analyzed() || body.Capabilities().Operators.Count() != 0 {
		t.Fatalf("analyzed field exposed standard operators")
	}
}

func TestWildcardMappingRetainsLiteralTextWithoutExpensiveProfile(t *testing.T) {
	code := mustField(t, FieldSpec[string]{
		Path:               "code.wildcard",
		Type:               MappingWildcard,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		),
	})
	mapping, err := NewMapping(code)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	compiler, err := NewCompiler(Elasticsearch95NoExpensiveQueries, mapping)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	requireCapabilities(t, compiler, code,
		weave.OperatorEQ,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	)
}

func TestFieldAndMappingRejectUnprovedSemantics(t *testing.T) {
	invalidCases := []struct {
		name string
		make func() error
	}{
		{
			name: "analyzed equality",
			make: func() error {
				_, err := NewField[string](FieldSpec[string]{
					Path:               "body",
					Type:               MappingText,
					CompleteValueIndex: true,
					Operators:          weave.NewOperatorSet(weave.OperatorEQ),
				})
				return err
			},
		},
		{
			name: "multi-valued equality",
			make: func() error {
				_, err := NewField[string](FieldSpec[string]{
					Path:               "tags",
					Type:               MappingKeyword,
					MultiValued:        true,
					CompleteValueIndex: true,
					Operators:          weave.NewOperatorSet(weave.OperatorEQ),
				})
				return err
			},
		},
		{
			name: "incomplete index",
			make: func() error {
				_, err := NewField[int64](FieldSpec[int64]{
					Path:      "age",
					Type:      MappingLong,
					Operators: weave.NewOperatorSet(weave.OperatorEQ),
				})
				return err
			},
		},
		{
			name: "null without complete index",
			make: func() error {
				_, err := NewField[string](FieldSpec[string]{
					Path:  "status",
					Type:  MappingKeyword,
					Nulls: IndexNullAs("NULL"),
				})
				return err
			},
		},
		{
			name: "date Between is not constructible in the core API",
			make: func() error {
				_, err := NewField[time.Time](FieldSpec[time.Time]{
					Path:               "created_at",
					Type:               MappingDate,
					CompleteValueIndex: true,
					Operators:          weave.NewOperatorSet(weave.OperatorBetween),
				})
				return err
			},
		},
		{
			name: "normalizer on wildcard",
			make: func() error {
				_, err := NewField[string](FieldSpec[string]{
					Path:       "code",
					Type:       MappingWildcard,
					Normalizer: "lowercase",
				})
				return err
			},
		},
		{
			name: "wrong Go scalar",
			make: func() error {
				_, err := NewField[string](FieldSpec[string]{
					Path: "age",
					Type: MappingLong,
				})
				return err
			},
		},
	}

	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.make(); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	marker := mustField(t, FieldSpec[string]{
		Path:               "state.status",
		Type:               MappingKeyword,
		CompleteValueIndex: true,
	})
	companion, err := NewCompanionMarker(marker, "null", "value")
	if err != nil {
		t.Fatalf("NewCompanionMarker: %v", err)
	}
	status := mustField(t, FieldSpec[string]{
		Path:               "status",
		Type:               MappingKeyword,
		CompleteValueIndex: true,
		Nulls:              MarkNullWith[string](companion),
		Operators: weave.NewOperatorSet(
			weave.OperatorIsNull,
			weave.OperatorNotNull,
		),
	})
	if _, err := NewMapping(status); err == nil {
		t.Fatal("mapping accepted an unregistered companion field")
	}
}

func TestMappingIdentityAndZeroValues(t *testing.T) {
	field := mustField(t, FieldSpec[bool]{
		Path:               "active",
		Type:               MappingBoolean,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ,
			weave.OperatorNEQ,
		),
	})
	mapping, err := NewMapping(field)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	compiler, err := NewCompiler(Elasticsearch95ExpensiveQueries, mapping)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}

	foreign := mustField(t, FieldSpec[bool]{
		Path:               "active",
		Type:               MappingBoolean,
		CompleteValueIndex: true,
		Operators:          weave.NewOperatorSet(weave.OperatorEQ),
	})
	if _, err := compiler.CapabilitiesFor(foreign); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("foreign Field error = %v", err)
	}

	var zeroField Field[bool]
	if zeroField.Path() != "" || zeroField.Capabilities().Operators.Count() != 0 {
		t.Fatal("zero Field is not inert")
	}
	var zeroMapping Mapping
	if zeroMapping.Valid() || zeroMapping.FieldCount() != 0 {
		t.Fatal("zero Mapping is not invalid")
	}
	var zeroCompiler Compiler
	if zeroCompiler.Capabilities().Operators.Count() != 0 ||
		zeroCompiler.Capabilities().Features.Count() != 0 {
		t.Fatal("zero Compiler exposed capabilities")
	}
	query, err := zeroCompiler.Compile(weave.Predicate[Query, Expression]{})
	if query != nil || err == nil {
		t.Fatal("invalid Compiler did not return zero Query and an error")
	}
}

func TestCompilerRejectsInvalidPredicateWithZeroQuery(t *testing.T) {
	mapping, err := NewMapping()
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	compiler, err := NewCompiler(Elasticsearch95ExpensiveQueries, mapping)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	query, err := compiler.Compile(weave.Predicate[Query, Expression]{})
	if query != nil || err == nil {
		t.Fatal("foundation Compile did not return zero Query and an error")
	}
	var structured *weave.Error
	if !errors.As(err, &structured) || structured.Code != weave.CodeInvalidPredicate ||
		structured.Phase != weave.PhaseValidate {
		t.Fatalf("Compile error = %#v", err)
	}
}

func mustField[T any](t testing.TB, spec FieldSpec[T]) Field[T] {
	t.Helper()
	field, err := NewField(spec)
	if err != nil {
		t.Fatalf("NewField: %v", err)
	}
	return field
}

func requireCapabilities[T any](
	t *testing.T,
	compiler Compiler,
	field Field[T],
	want ...weave.Operator,
) {
	t.Helper()
	capabilities, err := compiler.CapabilitiesFor(field)
	if err != nil {
		t.Fatalf("CapabilitiesFor: %v", err)
	}
	if capabilities.Operators.Count() != len(want) {
		t.Fatalf("operator count = %d, want %d", capabilities.Operators.Count(), len(want))
	}
	for _, operator := range want {
		if !capabilities.Operators.Has(operator) {
			t.Fatalf("missing operator %s", operator)
		}
	}
}
