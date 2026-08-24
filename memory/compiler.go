package memory

import (
	"reflect"

	"github.com/imbrooklyn/weave"
)

// Factory binds a memory Compiler to Weave predicate construction.
type Factory[R any] = weave.Factory[Condition[R], Expression[R]]

// Group is the memory expression specialization of weave.Group.
type Group[R any] = weave.Group[Expression[R]]

// Scope is the memory expression specialization of weave.Scope.
type Scope[R any] = weave.Scope[Expression[R]]

// Compiler validates memory Fields and emits record-level Conditions. It has
// no record collection or request state and is safe for concurrent use when
// its borrowed Field, Accessor, Semantics, Native, and Expr values satisfy
// their concurrency contracts.
type Compiler[R any] struct{}

var compilerCapabilities = weave.Capabilities{
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
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	),
	Features: weave.NewFeatureSet(
		weave.FeatureNativeCondition,
		weave.FeatureNativeExpression,
	),
}

// NewCompiler returns a request-stateless Compiler.
func NewCompiler[R any]() Compiler[R] {
	return Compiler[R]{}
}

// NewFactory returns a Factory bound to a new request-stateless Compiler.
func NewFactory[R any]() *Factory[R] {
	return weave.NewFactory[Condition[R], Expression[R]](NewCompiler[R]())
}

// Capabilities returns support for every standard Weave operator and both
// native features. Field-level applicability remains controlled by each
// Field's Semantics.
func (Compiler[R]) Capabilities() weave.Capabilities {
	return compilerCapabilities
}

// CapabilitiesFor reports the operations described by a valid typed memory
// Field. Fields created for another record type are rejected.
func (Compiler[R]) CapabilitiesFor(
	field any,
) (weave.FieldCapabilities, error) {
	if isNilLike(field) {
		return weave.FieldCapabilities{}, weave.ErrInvalidField
	}
	descriptor, ok := field.(fieldDescriptor)
	if !ok {
		return weave.FieldCapabilities{}, weave.ErrInvalidField
	}
	metadata := descriptor.memoryFieldDescriptor()
	if !metadata.valid || metadata.recordType != reflect.TypeFor[R]() {
		return weave.FieldCapabilities{}, weave.ErrInvalidField
	}
	return metadata.capabilities, nil
}

// Compile performs a complete validation pass followed by a separate emission
// pass. Validation visits nodes in stable preorder depth-first order. Every
// failure returns a nil Condition and a structured weave.Error in
// weave.PhaseValidate or weave.PhaseEmit.
func (Compiler[R]) Compile(
	predicate weave.Predicate[Condition[R], Expression[R]],
) (Condition[R], error) {
	root := predicate.Root()
	validated, err := validatePredicate(root)
	if err != nil {
		return nil, err
	}
	condition, err := emitPredicate(validated)
	if err != nil {
		return nil, err
	}
	return condition, nil
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
		reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
