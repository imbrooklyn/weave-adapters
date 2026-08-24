package gormgen

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
	"gorm.io/gen/field"
)

type compilerOptions struct {
	fields         map[columnIdentity]FieldSpec
	registeredOnly bool
}

type compilerState struct {
	profile        Profile
	fields         map[columnIdentity]FieldSpec
	registeredOnly bool
}

// Compiler owns an immutable SQL profile and generated-field registry. It does
// not own a database, session, context, logger, transaction, or query value.
// A configured Compiler is safe for concurrent use.
type Compiler struct {
	state *compilerState
}

var (
	_ weave.Compiler[Conditions, Expression] = Compiler{}
	_ weave.FieldCapabilityResolver          = Compiler{}
)

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

// NewCompiler validates profile and options, then returns a request-stateless
// Compiler with an immutable field registry.
func NewCompiler(profile Profile, options ...Option) (Compiler, error) {
	if !profile.valid() {
		return Compiler{}, fmt.Errorf(
			"gormgen: invalid compiler profile: %w",
			weave.ErrInvalidValue,
		)
	}

	configuration := compilerOptions{
		fields: make(map[columnIdentity]FieldSpec),
	}
	for _, option := range options {
		if isNilLike(option) {
			return Compiler{}, fmt.Errorf(
				"gormgen: nil compiler option: %w",
				weave.ErrInvalidValue,
			)
		}
		if err := option.apply(&configuration); err != nil {
			return Compiler{}, err
		}
	}
	if configuration.registeredOnly && len(configuration.fields) == 0 {
		return Compiler{}, fmt.Errorf(
			"gormgen: registered-fields-only mode requires at least one FieldSpec: %w",
			weave.ErrInvalidField,
		)
	}

	registry := make(map[columnIdentity]FieldSpec, len(configuration.fields))
	for identity, spec := range configuration.fields {
		registry[identity] = spec
	}
	return Compiler{state: &compilerState{
		profile:        profile,
		fields:         registry,
		registeredOnly: configuration.registeredOnly,
	}}, nil
}

// NewFactory validates profile and options, then returns a Factory bound to a
// new request-stateless Compiler.
func NewFactory(profile Profile, options ...Option) (*Factory, error) {
	compiler, err := NewCompiler(profile, options...)
	if err != nil {
		return nil, err
	}
	return weave.NewFactory[Conditions, Expression](compiler), nil
}

// Capabilities returns support for every standard Weave operator and both
// native features for a valid configured Compiler. Field-level applicability
// remains controlled by generated metadata and optional FieldSpec values. A
// zero or otherwise invalid Compiler returns zero capabilities.
func (compiler Compiler) Capabilities() weave.Capabilities {
	if !compiler.valid() {
		return weave.Capabilities{}
	}
	return compilerCapabilities
}

// CapabilitiesFor reports the standard operators applicable to a pure
// generated column after applying the immutable registry configuration.
func (compiler Compiler) CapabilitiesFor(
	value any,
) (weave.FieldCapabilities, error) {
	if !compiler.valid() {
		return weave.FieldCapabilities{}, fmt.Errorf(
			"gormgen: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}
	native, ok := value.(field.Expr)
	if !ok {
		return weave.FieldCapabilities{}, invalidGeneratedFieldError()
	}
	metadata, err := compiler.resolveField(native)
	if err != nil {
		return weave.FieldCapabilities{}, err
	}
	return weave.FieldCapabilities{Operators: metadata.operators}, nil
}

// Compile performs a complete validation pass followed by a separate emission
// pass. Validation visits nodes in stable preorder depth-first order. Every
// failure returns a nil Conditions value and a structured weave.Error in
// weave.PhaseValidate or weave.PhaseEmit.
func (compiler Compiler) Compile(
	predicate weave.Predicate[Conditions, Expression],
) (Conditions, error) {
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
	conditions, err := emitPredicate(validated)
	if err != nil {
		return nil, err
	}
	return ConditionsOf(conditions...), nil
}

func (compiler Compiler) resolveField(
	native field.Expr,
) (nativeFieldMetadata, error) {
	if !compiler.valid() {
		return nativeFieldMetadata{}, fmt.Errorf(
			"gormgen: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}

	metadata, err := inspectNativeField(native)
	if err != nil {
		return nativeFieldMetadata{}, err
	}
	identity := columnIdentity{
		table: metadata.column.Table,
		name:  metadata.column.Name,
	}
	if spec, ok := compiler.state.fields[identity]; ok {
		if metadata.nativeType != spec.nativeType ||
			!spec.valueType.AssignableTo(metadata.valueType) {
			return nativeFieldMetadata{}, invalidGeneratedFieldError()
		}
		metadata.valueType = spec.valueType
		metadata.operators = spec.operators
		return metadata, nil
	}
	if compiler.state.registeredOnly {
		return nativeFieldMetadata{}, invalidGeneratedFieldError()
	}
	return metadata, nil
}

func (compiler Compiler) valid() bool {
	if compiler.state == nil ||
		!compiler.state.profile.valid() ||
		compiler.state.fields == nil ||
		(compiler.state.registeredOnly && len(compiler.state.fields) == 0) {
		return false
	}
	for identity, spec := range compiler.state.fields {
		if !spec.valid() || identity != spec.identity() {
			return false
		}
	}
	return true
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
