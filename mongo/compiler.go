package mongo

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
)

type compilerState struct {
	profile Profile
}

// Compiler owns one immutable MongoDB profile. It does not own a client,
// collection, database, context, session, logger, transaction, or query value.
// A valid Compiler is request-stateless, can be copied, and is safe for
// concurrent use.
type Compiler struct {
	state *compilerState
}

var (
	_ weave.Compiler[Filter, Expression] = Compiler{}
	_ weave.FieldCapabilityResolver      = Compiler{}
)

var compilerCapabilities = weave.Capabilities{
	Operators: weave.NewOperatorSet(standardOperators()...),
	Features: weave.NewFeatureSet(
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	),
}

// NewCompiler validates profile and returns a request-stateless Compiler with
// immutable MongoDB filter semantics.
func NewCompiler(profile Profile) (Compiler, error) {
	if !profile.valid() {
		return Compiler{}, fmt.Errorf(
			"mongo: invalid compiler profile: %w",
			weave.ErrInvalidValue,
		)
	}
	return Compiler{state: &compilerState{profile: profile}}, nil
}

// NewFactory validates profile and returns a Factory bound to a new
// request-stateless Compiler.
func NewFactory(profile Profile) (*Factory, error) {
	compiler, err := NewCompiler(profile)
	if err != nil {
		return nil, err
	}
	return weave.NewFactory[Filter, Expression](compiler), nil
}

// Capabilities returns support for every standard Weave operator and both
// native features for a valid configured Compiler. Per-field applicability is
// controlled by each Field's immutable operator set. An invalid Compiler
// returns zero capabilities.
func (compiler Compiler) Capabilities() weave.Capabilities {
	if !compiler.valid() {
		return weave.Capabilities{}
	}
	return compilerCapabilities
}

// CapabilitiesFor returns the immutable operator set declared by a Field
// created by this package. Compile validation remains authoritative.
func (compiler Compiler) CapabilitiesFor(
	value any,
) (weave.FieldCapabilities, error) {
	if !compiler.valid() {
		return weave.FieldCapabilities{}, fmt.Errorf(
			"mongo: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}
	descriptor, ok := value.(fieldDescriptor)
	if !ok {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	metadata := descriptor.mongoFieldMetadata()
	if !metadata.validFor(reflect.TypeOf(value)) {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	return weave.FieldCapabilities{Operators: metadata.operators}, nil
}

// Compile performs a complete stable preorder validation pass followed by a
// separate deterministic emission pass. Every failure returns a nil Filter
// and a structured weave.Error in weave.PhaseValidate or weave.PhaseEmit. The
// first validation error is stable preorder depth-first, and its diagnostic
// text does not include field paths, query values, or Native/Expr payloads.
// Generated standard BSON topology is fresh on every call; ordinary query
// values are not recursively cloned. Native and Expr documents receive a fresh
// top-level slice while their nested values also remain borrowed. Concurrent
// calls are safe when all borrowed state stays immutable.
func (compiler Compiler) Compile(
	predicate weave.Predicate[Filter, Expression],
) (Filter, error) {
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
	filter, err := emitPredicate(validated)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

func (compiler Compiler) valid() bool {
	return compiler.state != nil && compiler.state.profile.valid()
}

func isNilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice,
		reflect.UnsafePointer:
		return reflected.IsNil()
	default:
		return false
	}
}
