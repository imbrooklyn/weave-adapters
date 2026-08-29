package elasticsearch

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
)

type compilerState struct {
	profile      Profile
	mapping      *mappingState
	fieldCaps    map[*fieldState]weave.OperatorSet
	capabilities weave.Capabilities
}

// Compiler owns one immutable Elasticsearch Profile and Mapping. It does not
// own a client, transport, index, context, credential, request builder,
// logger, session, or per-request query value. A valid Compiler is
// request-stateless, can be copied, and its own state is safe for concurrent
// use. Callers remain responsible for not mutating a Predicate or borrowed
// Native/Expr payload concurrently with Compile.
type Compiler struct {
	state *compilerState
}

var (
	_ weave.Compiler[Query, Expression] = Compiler{}
	_ weave.FieldCapabilityResolver     = Compiler{}
)

// NewCompiler validates profile and mapping and returns a request-stateless
// Compiler with an immutable capability snapshot.
func NewCompiler(profile Profile, mapping Mapping) (Compiler, error) {
	if !profile.valid() {
		return Compiler{}, fmt.Errorf(
			"elasticsearch: invalid compiler profile: %w",
			weave.ErrInvalidValue,
		)
	}
	if !mapping.Valid() {
		return Compiler{}, fmt.Errorf(
			"elasticsearch: invalid compiler mapping: %w",
			weave.ErrInvalidState,
		)
	}

	fieldCaps := make(map[*fieldState]weave.OperatorSet, len(mapping.state.fields))
	operatorPresent := make(map[weave.Operator]bool, len(allStandardOperators()))
	for field := range mapping.state.fields {
		effective := effectiveFieldOperators(profile, field)
		fieldCaps[field] = effective
		for _, operator := range allStandardOperators() {
			operatorPresent[operator] = operatorPresent[operator] ||
				effective.Has(operator)
		}
	}
	operators := make([]weave.Operator, 0, len(operatorPresent))
	for _, operator := range allStandardOperators() {
		if operatorPresent[operator] {
			operators = append(operators, operator)
		}
	}

	return Compiler{state: &compilerState{
		profile:   profile,
		mapping:   mapping.state,
		fieldCaps: fieldCaps,
		capabilities: weave.Capabilities{
			Operators: weave.NewOperatorSet(operators...),
			Features: weave.NewFeatureSet(
				weave.FeatureNativeCondition,
				weave.FeatureNativeExpression,
			),
		},
	}}, nil
}

// NewFactory validates profile and mapping and returns a Factory bound to a
// new request-stateless Compiler.
func NewFactory(profile Profile, mapping Mapping) (*Factory, error) {
	compiler, err := NewCompiler(profile, mapping)
	if err != nil {
		return nil, err
	}
	return weave.NewFactory[Query, Expression](compiler), nil
}

// Capabilities returns the immutable union of effective per-field operators
// together with Native and Expr support. An invalid Compiler returns zero
// capabilities.
func (compiler Compiler) Capabilities() weave.Capabilities {
	if !compiler.valid() {
		return weave.Capabilities{}
	}
	return compiler.state.capabilities
}

// CapabilitiesFor returns the exact Field operator set after applying Mapping
// identity and the configured expensive-query Profile. Compile validation
// remains authoritative.
func (compiler Compiler) CapabilitiesFor(
	value any,
) (weave.FieldCapabilities, error) {
	if !compiler.valid() {
		return weave.FieldCapabilities{}, fmt.Errorf(
			"elasticsearch: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}
	descriptor, ok := value.(MappedField)
	if !ok || isNilMappedField(descriptor) {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	metadata := descriptor.elasticsearchFieldMetadata()
	if !metadata.validFor(reflect.TypeOf(value)) ||
		!compiler.state.mapping.contains(metadata.state) {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	return weave.FieldCapabilities{
		Operators: compiler.state.fieldCaps[metadata.state],
	}, nil
}

// Compile performs a complete stable preorder validation pass followed by a
// separate deterministic emission pass. Every failure returns a nil Query and
// a structured, redacted weave.Error. Compile never discovers mapping or
// cluster settings and retains no predicate or per-call plan state. Standard
// output is fresh per call; Native and raw Expr payloads retain their upstream
// borrowed ownership.
func (compiler Compiler) Compile(
	predicate weave.Predicate[Query, Expression],
) (Query, error) {
	if !compiler.valid() {
		return nil, &weave.Error{
			Code:  weave.CodeInvalidState,
			Phase: weave.PhaseValidate,
		}
	}
	validated, err := compiler.validatePredicate(predicate)
	if err != nil {
		return nil, err
	}
	query, err := emitPredicate(validated)
	if err != nil {
		return nil, err
	}
	return query, nil
}

func (compiler Compiler) valid() bool {
	return compiler.state != nil && compiler.state.profile.valid() &&
		compiler.state.mapping != nil && compiler.state.fieldCaps != nil
}

func (compiler Compiler) fieldMetadata(value any) (fieldMetadata, error) {
	if isNilLike(value) {
		return fieldMetadata{}, invalidFieldDefinitionError()
	}
	descriptor, ok := value.(MappedField)
	if !ok || isNilMappedField(descriptor) {
		return fieldMetadata{}, invalidFieldDefinitionError()
	}
	metadata := descriptor.elasticsearchFieldMetadata()
	if !metadata.validFor(reflect.TypeOf(value)) ||
		!compiler.state.mapping.contains(metadata.state) {
		return fieldMetadata{}, invalidFieldDefinitionError()
	}
	return metadata, nil
}

func isNilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return reflected.IsNil()
	default:
		return false
	}
}

func effectiveFieldOperators(
	profile Profile,
	field *fieldState,
) weave.OperatorSet {
	if field == nil || !validFieldState(field) {
		return weave.OperatorSet{}
	}
	operators := make([]weave.Operator, 0, field.operators.Count())
	for _, operator := range allStandardOperators() {
		if !field.operators.Has(operator) {
			continue
		}
		if field.mappingType == MappingKeyword &&
			!profile.allowsExpensiveQueries() {
			switch operator {
			case weave.OperatorContains,
				weave.OperatorHasPrefix,
				weave.OperatorHasSuffix:
				continue
			}
		}
		operators = append(operators, operator)
	}
	return weave.NewOperatorSet(operators...)
}
