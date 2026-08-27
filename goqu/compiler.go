package goqu

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
)

type compilerState struct {
	profile Profile
}

// Compiler owns one immutable SQL profile. It does not own a database,
// dataset, context, logger, transaction, or query value. A valid Compiler is
// request-stateless, can be copied, and is safe for concurrent use.
type Compiler struct {
	state *compilerState
}

var (
	_ weave.Compiler[Expressions, Expression] = Compiler{}
	_ weave.FieldCapabilityResolver           = Compiler{}
)

var compilerCapabilities = weave.Capabilities{
	Operators: weave.NewOperatorSet(standardOperators()...),
	Features: weave.NewFeatureSet(
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	),
}

// NewCompiler validates profile and returns a request-stateless Compiler with
// immutable dialect semantics.
func NewCompiler(profile Profile) (Compiler, error) {
	if !profile.valid() {
		return Compiler{}, fmt.Errorf(
			"goqu: invalid compiler profile: %w",
			weave.ErrInvalidValue,
		)
	}
	return Compiler{state: &compilerState{profile: profile}}, nil
}

// NewFactory validates profile and returns a Factory bound to a new
// request-stateless Compiler. The returned Factory is safe for concurrent
// predicate construction and compilation as documented by Weave.
func NewFactory(profile Profile) (*Factory, error) {
	compiler, err := NewCompiler(profile)
	if err != nil {
		return nil, err
	}
	return weave.NewFactory[Expressions, Expression](compiler), nil
}

// Capabilities returns support for every standard Weave operator and both
// native features for a valid configured Compiler. Per-field applicability is
// still controlled by each Field's exact immutable OperatorSet. A zero or
// otherwise invalid Compiler returns zero capabilities.
func (compiler Compiler) Capabilities() weave.Capabilities {
	if !compiler.valid() {
		return weave.Capabilities{}
	}
	return compilerCapabilities
}

// CapabilitiesFor returns the immutable operator set declared by a Field
// created by this package. A Compiler's later Compile validation remains
// authoritative.
func (compiler Compiler) CapabilitiesFor(
	value any,
) (weave.FieldCapabilities, error) {
	if !compiler.valid() {
		return weave.FieldCapabilities{}, fmt.Errorf(
			"goqu: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}
	descriptor, ok := value.(fieldDescriptor)
	if !ok {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	metadata := descriptor.goquFieldMetadata()
	if !metadata.validFor(reflect.TypeOf(value)) {
		return weave.FieldCapabilities{}, invalidFieldDefinitionError()
	}
	return weave.FieldCapabilities{Operators: metadata.operators}, nil
}

// Compile performs a complete validation pass followed by a separate
// emission pass. Validation visits nodes in stable preorder depth-first
// order. Every failure returns nil Expressions and a structured weave.Error
// in weave.PhaseValidate or weave.PhaseEmit. Concurrent calls are safe when
// borrowed Native and Expr payload state remains immutable.
func (compiler Compiler) Compile(
	predicate weave.Predicate[Expressions, Expression],
) (Expressions, error) {
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
	expressions, err := emitPredicate(validated)
	if err != nil {
		return nil, err
	}
	return expressions, nil
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
