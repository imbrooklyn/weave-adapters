package ldap

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
)

type compilerState struct {
	profile Profile
	schema  *schemaState
}

// Compiler owns one immutable LDAP profile and Schema. It does not own an
// LDAP connection, request, context, bind credential, logger, session, or
// query value. A valid Compiler is request-stateless, can be copied, and is
// safe for concurrent use.
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

// NewCompiler validates profile and schema and returns a request-stateless
// Compiler.
func NewCompiler(profile Profile, schema Schema) (Compiler, error) {
	if !profile.valid() {
		return Compiler{}, fmt.Errorf(
			"ldap: invalid compiler profile: %w",
			weave.ErrInvalidValue,
		)
	}
	if !schema.Valid() {
		return Compiler{}, fmt.Errorf(
			"ldap: invalid compiler schema: %w",
			weave.ErrInvalidState,
		)
	}
	return Compiler{state: &compilerState{
		profile: profile,
		schema:  schema.state,
	}}, nil
}

// NewFactory validates profile and schema and returns a Factory bound to a new
// request-stateless Compiler.
func NewFactory(profile Profile, schema Schema) (*Factory, error) {
	compiler, err := NewCompiler(profile, schema)
	if err != nil {
		return nil, err
	}
	return weave.NewFactory[Filter, Expression](compiler), nil
}

// Capabilities returns the 11 exactly lowerable standard operators and both
// Native features. Per-attribute applicability is the intersection with each
// registered descriptor's immutable operator set. An invalid Compiler returns
// zero capabilities.
func (compiler Compiler) Capabilities() weave.Capabilities {
	if !compiler.valid() {
		return weave.Capabilities{}
	}
	return compilerCapabilities
}

// CapabilitiesFor returns the exact standard-operator set of an Attribute
// registered in this Compiler's Schema. Compile validation remains
// authoritative.
func (compiler Compiler) CapabilitiesFor(value any) (weave.FieldCapabilities, error) {
	if !compiler.valid() {
		return weave.FieldCapabilities{}, fmt.Errorf(
			"ldap: invalid compiler state: %w",
			weave.ErrInvalidState,
		)
	}
	metadata, err := compiler.attributeMetadata(value)
	if err != nil {
		return weave.FieldCapabilities{}, err
	}
	return weave.FieldCapabilities{Operators: metadata.state.operators}, nil
}

// Compile performs a complete stable preorder validation pass followed by a
// separate deterministic emission pass. Every failure returns a zero Filter
// and a structured weave.Error. Error text omits attribute identifiers, query
// values, and Native or Expr payloads. Compile does not retain predicate or
// per-call plan state after it returns.
func (compiler Compiler) Compile(
	predicate weave.Predicate[Filter, Expression],
) (Filter, error) {
	if !compiler.valid() {
		return Filter{}, &weave.Error{
			Code:  weave.CodeInvalidState,
			Phase: weave.PhaseValidate,
		}
	}
	validated, err := compiler.validatePredicate(predicate)
	if err != nil {
		return Filter{}, err
	}
	filterText, err := emitPredicate(validated)
	if err != nil {
		return Filter{}, err
	}
	filter, err := newFilter(compiler.state.schema, filterText, true)
	if err != nil {
		return Filter{}, &weave.Error{
			Code:   weave.CodeCompileFailure,
			Phase:  weave.PhaseEmit,
			Path:   validated.path,
			Origin: validated.origin,
			Cause:  errInvalidFilter,
		}
	}
	return filter, nil
}

func (compiler Compiler) valid() bool {
	return compiler.state != nil && compiler.state.profile.valid() &&
		compiler.state.schema != nil
}

func (compiler Compiler) attributeMetadata(value any) (attributeMetadata, error) {
	if isNilLike(value) {
		return attributeMetadata{}, invalidAttributeDefinitionError()
	}
	descriptor, ok := value.(attributeDescriptor)
	if !ok {
		return attributeMetadata{}, invalidAttributeDefinitionError()
	}
	metadata := descriptor.ldapAttributeMetadata()
	if !metadata.validFor(reflect.TypeOf(value)) ||
		!compiler.state.schema.contains(metadata.state) {
		return attributeMetadata{}, invalidAttributeDefinitionError()
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

func standardOperators() []weave.Operator {
	return []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorLTE,
		weave.OperatorGTE,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorBetween,
		weave.OperatorNotNull,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	}
}

func standardOperator(operator weave.Operator) bool {
	for _, candidate := range standardOperators() {
		if candidate == operator {
			return true
		}
	}
	return false
}

func comparisonOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorEQ, weave.OperatorNEQ, weave.OperatorLT,
		weave.OperatorLTE, weave.OperatorGT, weave.OperatorGTE:
		return true
	default:
		return false
	}
}

func nullOperator(operator weave.Operator) bool {
	return operator == weave.OperatorIsNull || operator == weave.OperatorNotNull
}

func membershipOperator(operator weave.Operator) bool {
	return operator == weave.OperatorIn || operator == weave.OperatorNotIn
}

func textOperator(operator weave.Operator) bool {
	switch operator {
	case weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix:
		return true
	default:
		return false
	}
}

func validLogic(logic weave.Logic) bool {
	switch logic {
	case weave.LogicAllOf, weave.LogicAnyOf,
		weave.LogicNoneOf, weave.LogicNotAllOf:
		return true
	default:
		return false
	}
}
