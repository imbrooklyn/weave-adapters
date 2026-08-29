package ldap

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/imbrooklyn/weave"
)

// Syntax identifies one supported LDAP attribute syntax and its deterministic
// Go value encoding. Its zero value is invalid.
type Syntax uint8

const (
	// SyntaxDirectoryString accepts string and defined-string query types.
	SyntaxDirectoryString Syntax = iota + 1
	// SyntaxIA5String accepts ASCII string and defined-string query types.
	SyntaxIA5String
	// SyntaxInteger accepts signed or unsigned integer query types.
	SyntaxInteger
	// SyntaxBoolean accepts bool and defined-bool query types.
	SyntaxBoolean
	// SyntaxGeneralizedTime accepts time.Time query values.
	SyntaxGeneralizedTime
	// SyntaxOctetString accepts byte-slice and defined-byte-slice query types.
	SyntaxOctetString
)

// OID returns the LDAP syntax numeric OID, or an empty string for an invalid
// Syntax.
func (syntax Syntax) OID() string {
	switch syntax {
	case SyntaxDirectoryString:
		return "1.3.6.1.4.1.1466.115.121.1.15"
	case SyntaxIA5String:
		return "1.3.6.1.4.1.1466.115.121.1.26"
	case SyntaxInteger:
		return "1.3.6.1.4.1.1466.115.121.1.27"
	case SyntaxBoolean:
		return "1.3.6.1.4.1.1466.115.121.1.7"
	case SyntaxGeneralizedTime:
		return "1.3.6.1.4.1.1466.115.121.1.24"
	case SyntaxOctetString:
		return "1.3.6.1.4.1.1466.115.121.1.40"
	default:
		return ""
	}
}

// String returns a stable English diagnostic identifier for syntax.
func (syntax Syntax) String() string {
	switch syntax {
	case SyntaxDirectoryString:
		return "directory_string"
	case SyntaxIA5String:
		return "ia5_string"
	case SyntaxInteger:
		return "integer"
	case SyntaxBoolean:
		return "boolean"
	case SyntaxGeneralizedTime:
		return "generalized_time"
	case SyntaxOctetString:
		return "octet_string"
	default:
		return "syntax(" + strconv.FormatUint(uint64(syntax), 10) + ")"
	}
}

func (syntax Syntax) valid() bool {
	return syntax.OID() != ""
}

// MatchingRules is an immutable declaration of the numeric matching-rule OIDs
// assigned to an LDAP attribute type. Empty individual OIDs mean that the
// corresponding assertion family is unavailable.
type MatchingRules struct {
	equality  string
	ordering  string
	substring string
}

// NewMatchingRules validates and returns a matching-rule declaration. Each
// non-empty argument must be a numeric OID. Supplying three empty strings is
// valid for an attribute used only for presence or reviewed Expr filters.
func NewMatchingRules(equality, ordering, substring string) (MatchingRules, error) {
	rules := MatchingRules{
		equality:  equality,
		ordering:  ordering,
		substring: substring,
	}
	if !rules.valid() {
		return MatchingRules{}, fmt.Errorf(
			"ldap: matching rules require numeric OIDs: %w",
			weave.ErrInvalidValue,
		)
	}
	return rules, nil
}

// Equality returns the equality matching-rule numeric OID.
func (rules MatchingRules) Equality() string { return rules.equality }

// Ordering returns the ordering matching-rule numeric OID.
func (rules MatchingRules) Ordering() string { return rules.ordering }

// Substring returns the substring matching-rule numeric OID.
func (rules MatchingRules) Substring() string { return rules.substring }

func (rules MatchingRules) valid() bool {
	for _, oid := range []string{rules.equality, rules.ordering, rules.substring} {
		if oid != "" && !validNumericOID(oid) {
			return false
		}
	}
	return true
}

func (rules MatchingRules) contains(oid string) bool {
	return oid != "" &&
		(oid == rules.equality || oid == rules.ordering || oid == rules.substring)
}

// AttributeSpec is the complete immutable-schema input used by NewAttribute.
// Description and OID identify the attribute type; options are not accepted.
// Operators is the exact standard-operator set and must be empty when
// SingleValued is false.
type AttributeSpec struct {
	// Description is the option-free RFC 4512 name or numeric OID used in
	// reviewed Expr input. Keystrings are canonicalized to lowercase.
	Description string
	// OID is the numeric attribute type OID emitted by standard lowering.
	OID string
	// SingleValued declares the logical cardinality. Multi-valued attributes
	// must have an empty Operators set.
	SingleValued bool
	// Syntax fixes the LDAP assertion encoding and compatible Go query type.
	Syntax Syntax
	// Matching fixes the equality, ordering, and substring matching-rule OIDs.
	Matching MatchingRules
	// Operators is the exact immutable standard-operator set.
	Operators weave.OperatorSet
}

// Attribute is an immutable typed LDAP attribute declaration. Standard Weave
// leaves require an Attribute registered in the Compiler's Schema. A valid
// value contains no caller-owned mutable state and is safe to copy and use
// concurrently.
type Attribute[T any] struct {
	state *attributeState
}

type attributeState struct {
	description  string
	oid          string
	singleValued bool
	syntax       Syntax
	matching     MatchingRules
	operators    weave.OperatorSet
	valueType    reflect.Type
}

type attributeMetadata struct {
	state          *attributeState
	descriptorType reflect.Type
}

type attributeDescriptor interface {
	ldapAttributeMetadata() attributeMetadata
}

// NewAttribute validates spec against T and returns a typed declaration. The
// caller must register the result in a Schema before using it in standard
// predicate leaves.
func NewAttribute[T any](spec AttributeSpec) (Attribute[T], error) {
	description := canonicalDescription(spec.Description)
	valueType := reflect.TypeFor[T]()
	if description == "" || !validNumericOID(spec.OID) || !spec.Syntax.valid() ||
		!spec.Matching.valid() || !syntaxAcceptsType(spec.Syntax, valueType) {
		return Attribute[T]{}, invalidAttributeDefinitionError()
	}
	if validNumericOID(description) && description != spec.OID {
		return Attribute[T]{}, invalidAttributeDefinitionError()
	}
	if err := validateAttributeOperators(
		spec.SingleValued,
		spec.Syntax,
		valueType,
		spec.Matching,
		spec.Operators,
	); err != nil {
		return Attribute[T]{}, err
	}
	return Attribute[T]{state: &attributeState{
		description:  description,
		oid:          spec.OID,
		singleValued: spec.SingleValued,
		syntax:       spec.Syntax,
		matching:     spec.Matching,
		operators:    spec.Operators,
		valueType:    valueType,
	}}, nil
}

// Description returns the canonical option-free attribute description.
func (attribute Attribute[T]) Description() string {
	if !attribute.valid() {
		return ""
	}
	return attribute.state.description
}

// OID returns the attribute type's numeric OID.
func (attribute Attribute[T]) OID() string {
	if !attribute.valid() {
		return ""
	}
	return attribute.state.oid
}

// SingleValued reports the cardinality declaration fixed by the Schema.
func (attribute Attribute[T]) SingleValued() bool {
	return attribute.valid() && attribute.state.singleValued
}

// Syntax returns the fixed LDAP syntax declaration.
func (attribute Attribute[T]) Syntax() Syntax {
	if !attribute.valid() {
		return 0
	}
	return attribute.state.syntax
}

// MatchingRules returns the fixed matching-rule declaration.
func (attribute Attribute[T]) MatchingRules() MatchingRules {
	if !attribute.valid() {
		return MatchingRules{}
	}
	return attribute.state.matching
}

// Capabilities returns this descriptor's exact immutable standard-operator
// set. Multi-valued attributes always report zero standard capabilities.
func (attribute Attribute[T]) Capabilities() weave.FieldCapabilities {
	if !attribute.valid() {
		return weave.FieldCapabilities{}
	}
	return weave.FieldCapabilities{Operators: attribute.state.operators}
}

func (attribute Attribute[T]) ldapAttributeMetadata() attributeMetadata {
	return attributeMetadata{
		state:          attribute.state,
		descriptorType: reflect.TypeFor[Attribute[T]](),
	}
}

func (attribute Attribute[T]) valid() bool {
	return validAttributeState(attribute.state) &&
		attribute.state.valueType == reflect.TypeFor[T]()
}

func (metadata attributeMetadata) validFor(actualType reflect.Type) bool {
	return metadata.descriptorType != nil && actualType == metadata.descriptorType &&
		validAttributeState(metadata.state)
}

func validAttributeState(state *attributeState) bool {
	if state == nil || state.description == "" || !validNumericOID(state.oid) ||
		!state.syntax.valid() || !state.matching.valid() || state.valueType == nil ||
		!syntaxAcceptsType(state.syntax, state.valueType) {
		return false
	}
	return validateAttributeOperators(
		state.singleValued,
		state.syntax,
		state.valueType,
		state.matching,
		state.operators,
	) == nil
}

func validateAttributeOperators(
	singleValued bool,
	syntax Syntax,
	valueType reflect.Type,
	rules MatchingRules,
	operators weave.OperatorSet,
) error {
	if !singleValued && operators.Count() != 0 {
		return fmt.Errorf(
			"ldap: multi-valued attributes have no standard operators: %w",
			weave.ErrOperatorNotApplicable,
		)
	}
	for index := range operators.Count() {
		operator, ok := operators.At(index)
		if !ok {
			return invalidAttributeDefinitionError()
		}
		if !standardOperator(operator) {
			return operatorDefinitionError()
		}
		switch operator {
		case weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn:
			if rules.equality == "" {
				return operatorDefinitionError()
			}
		case weave.OperatorLTE:
			if rules.ordering == "" || rules.equality == "" {
				return operatorDefinitionError()
			}
		case weave.OperatorGTE:
			if rules.ordering == "" {
				return operatorDefinitionError()
			}
		case weave.OperatorBetween:
			if rules.ordering == "" || rules.equality == "" ||
				syntax != SyntaxInteger ||
				!integerType(valueType) {
				return operatorDefinitionError()
			}
		case weave.OperatorContains, weave.OperatorHasPrefix,
			weave.OperatorHasSuffix:
			if rules.substring == "" || rules.equality == "" ||
				!stringSyntax(syntax) ||
				valueType.Kind() != reflect.String {
				return operatorDefinitionError()
			}
		case weave.OperatorNotNull:
			// Presence needs no matching rule.
		default:
			return operatorDefinitionError()
		}
	}
	return nil
}

func syntaxAcceptsType(syntax Syntax, valueType reflect.Type) bool {
	if valueType == nil {
		return false
	}
	switch syntax {
	case SyntaxDirectoryString, SyntaxIA5String:
		return valueType.Kind() == reflect.String
	case SyntaxInteger:
		return integerType(valueType)
	case SyntaxBoolean:
		return valueType.Kind() == reflect.Bool
	case SyntaxGeneralizedTime:
		return valueType == reflect.TypeFor[time.Time]()
	case SyntaxOctetString:
		return valueType.Kind() == reflect.Slice &&
			valueType.Elem().Kind() == reflect.Uint8
	default:
		return false
	}
}

func integerType(valueType reflect.Type) bool {
	if valueType == nil {
		return false
	}
	switch valueType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func stringSyntax(syntax Syntax) bool {
	return syntax == SyntaxDirectoryString || syntax == SyntaxIA5String
}

func canonicalDescription(description string) string {
	if validNumericOID(description) {
		return description
	}
	if !validKeyString(description) {
		return ""
	}
	return strings.ToLower(description)
}

func validKeyString(value string) bool {
	if value == "" || !asciiLetter(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !asciiLetter(character) && (character < '0' || character > '9') &&
			character != '-' {
			return false
		}
	}
	return true
}

func asciiLetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func validNumericOID(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return false
		}
		for index := range len(part) {
			if part[index] < '0' || part[index] > '9' {
				return false
			}
		}
	}
	return true
}

func invalidAttributeDefinitionError() error {
	return fmt.Errorf(
		"ldap: invalid typed attribute declaration: %w",
		weave.ErrInvalidField,
	)
}

func operatorDefinitionError() error {
	return fmt.Errorf(
		"ldap: operator is incompatible with attribute schema: %w",
		weave.ErrOperatorNotApplicable,
	)
}
