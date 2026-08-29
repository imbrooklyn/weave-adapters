package elasticsearch

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/imbrooklyn/weave"
)

// ScalarType identifies the logical scalar represented by a mapped field. Its
// zero value is invalid.
type ScalarType uint8

const (
	// ScalarString is represented by string or a defined string type.
	ScalarString ScalarType = iota + 1
	// ScalarInteger is represented by a signed integer or defined signed integer.
	ScalarInteger
	// ScalarFloat is represented by float32, float64, or a defined float type.
	ScalarFloat
	// ScalarDateTime is represented by time.Time.
	ScalarDateTime
	// ScalarBoolean is represented by bool or a defined bool type.
	ScalarBoolean
)

func (scalar ScalarType) valid() bool {
	return scalar >= ScalarString && scalar <= ScalarBoolean
}

// String returns a stable English diagnostic identifier for scalar.
func (scalar ScalarType) String() string {
	switch scalar {
	case ScalarString:
		return "string"
	case ScalarInteger:
		return "integer"
	case ScalarFloat:
		return "float"
	case ScalarDateTime:
		return "date_time"
	case ScalarBoolean:
		return "boolean"
	default:
		return "scalar_type(" + strconv.FormatUint(uint64(scalar), 10) + ")"
	}
}

// MappingType identifies the Elasticsearch mapping family whose indexed-term
// behavior is asserted by a Field. Its zero value is invalid.
type MappingType uint8

const (
	// MappingKeyword selects a non-analyzed keyword field.
	MappingKeyword MappingType = iota + 1
	// MappingWildcard selects a non-analyzed wildcard field.
	MappingWildcard
	// MappingLong selects a signed integral numeric field lowered through the
	// typed LongNumberRangeQuery variant.
	MappingLong
	// MappingDouble selects a floating-point numeric field lowered through the
	// typed NumberRangeQuery variant.
	MappingDouble
	// MappingDate selects a date field lowered through the typed DateRangeQuery
	// variant.
	MappingDate
	// MappingBoolean selects a Boolean field.
	MappingBoolean
	// MappingText selects an analyzed text field. It has no standard Weave
	// operators and is available only to reviewed upstream Expr queries.
	MappingText
)

func (mappingType MappingType) valid() bool {
	return mappingType >= MappingKeyword && mappingType <= MappingText
}

// String returns a stable English diagnostic identifier for mappingType.
func (mappingType MappingType) String() string {
	switch mappingType {
	case MappingKeyword:
		return "keyword"
	case MappingWildcard:
		return "wildcard"
	case MappingLong:
		return "long"
	case MappingDouble:
		return "double"
	case MappingDate:
		return "date"
	case MappingBoolean:
		return "boolean"
	case MappingText:
		return "text"
	default:
		return "mapping_type(" + strconv.FormatUint(uint64(mappingType), 10) + ")"
	}
}

// Scalar returns the logical scalar required by mappingType. It returns zero
// for an invalid mapping type.
func (mappingType MappingType) Scalar() ScalarType {
	switch mappingType {
	case MappingKeyword, MappingWildcard, MappingText:
		return ScalarString
	case MappingLong:
		return ScalarInteger
	case MappingDouble:
		return ScalarFloat
	case MappingDate:
		return ScalarDateTime
	case MappingBoolean:
		return ScalarBoolean
	default:
		return 0
	}
}

// Analyzed reports whether mappingType uses analyzed full-text semantics.
func (mappingType MappingType) Analyzed() bool {
	return mappingType == MappingText
}

// Keyword reports whether mappingType is the Elasticsearch keyword type.
func (mappingType MappingType) Keyword() bool {
	return mappingType == MappingKeyword
}

// NullKind identifies how an explicit logical NULL is distinguished from a
// missing field and from a non-null Value. Its zero value means no proof.
type NullKind uint8

const (
	// NullUntracked declares no searchable explicit-null marker. IsNull and
	// NotNull are therefore not applicable.
	NullUntracked NullKind = iota
	// NullValueMarker uses a reserved same-field null_value sentinel.
	NullValueMarker
	// NullCompanionMarker uses a separate keyword state field with distinct
	// reserved terms for explicit NULL and non-null Value.
	NullCompanionMarker
)

// String returns a stable English diagnostic identifier for kind.
func (kind NullKind) String() string {
	switch kind {
	case NullUntracked:
		return "untracked"
	case NullValueMarker:
		return "null_value"
	case NullCompanionMarker:
		return "companion"
	default:
		return "null_kind(" + strconv.FormatUint(uint64(kind), 10) + ")"
	}
}

// NullMapping is an immutable typed declaration of a same-field null_value or
// a companion marker. Its zero value means that explicit NULL is not
// distinguishable from missing indexed state.
type NullMapping[T any] struct {
	kind      NullKind
	nullValue T
	companion CompanionMarker
}

// IndexNullAs declares value as the reserved null_value sentinel for the same
// field. NewField copies and validates the scalar. Applications must prevent a
// real non-null value from using the same indexed term.
func IndexNullAs[T any](value T) NullMapping[T] {
	return NullMapping[T]{kind: NullValueMarker, nullValue: value}
}

// MarkNullWith declares that marker carries the explicit NULL versus Value
// state for a field of type T. The marker field must be registered in the same
// Mapping as the resulting Field.
func MarkNullWith[T any](marker CompanionMarker) NullMapping[T] {
	return NullMapping[T]{kind: NullCompanionMarker, companion: marker}
}

// CompanionMarker is an immutable declaration of a single-valued keyword
// field whose two distinct reserved terms identify explicit NULL and non-null
// Value. Its zero value is invalid.
type CompanionMarker struct {
	state *companionMarkerState
}

type companionMarkerState struct {
	field     *fieldState
	nullTerm  string
	valueTerm string
}

// NewCompanionMarker validates a marker Field and its two reserved terms. The
// marker must be a complete, single-valued, non-nested keyword field without a
// normalizer, null mapping, expensive wildcard opt-in, or standard operators.
func NewCompanionMarker(
	marker Field[string],
	nullTerm string,
	valueTerm string,
) (CompanionMarker, error) {
	if !marker.valid() || !validCompanionFieldState(marker.state) ||
		!validMarkerTerm(nullTerm) || !validMarkerTerm(valueTerm) ||
		nullTerm == valueTerm {
		return CompanionMarker{}, invalidCompanionMarkerError()
	}
	return CompanionMarker{state: &companionMarkerState{
		field:     marker.state,
		nullTerm:  nullTerm,
		valueTerm: valueTerm,
	}}, nil
}

// Valid reports whether marker was created by NewCompanionMarker.
func (marker CompanionMarker) Valid() bool {
	return marker.state != nil &&
		validCompanionFieldState(marker.state.field) &&
		validMarkerTerm(marker.state.nullTerm) &&
		validMarkerTerm(marker.state.valueTerm) &&
		marker.state.nullTerm != marker.state.valueTerm
}

// Path returns the companion marker field path, or an empty string for an
// invalid marker.
func (marker CompanionMarker) Path() string {
	if !marker.Valid() {
		return ""
	}
	return marker.state.field.path
}

// FieldSpec is the complete immutable-mapping input used by NewField. The
// Operators set is the exact declared standard-operator set; its zero value is
// an explicit empty set.
type FieldSpec[T any] struct {
	// Path is the canonical dot-separated Elasticsearch field path.
	Path string
	// Type fixes the Elasticsearch mapping family and the compatible Go scalar.
	Type MappingType
	// MultiValued declares array or otherwise multi-valued indexed semantics.
	// Multi-valued fields cannot expose standard Weave operators.
	MultiValued bool
	// Nested declares that the path is scoped through an Elasticsearch nested
	// mapping. Nested fields cannot expose standard Weave operators.
	Nested bool
	// CompleteValueIndex asserts that every logical non-null Value produces the
	// searchable indexed term or point expected by Type, without ignore_above,
	// ignore_malformed, disabled indexing, or an equivalent loss boundary.
	CompleteValueIndex bool
	// Normalizer is the exact keyword normalizer name. Empty means none. A
	// normalizer is accepted only for MappingKeyword.
	Normalizer string
	// AllowExpensiveWildcard explicitly opts a keyword field into leading
	// wildcard lowering for Contains and HasSuffix. The configured Profile must
	// independently assert that the cluster permits expensive queries.
	AllowExpensiveWildcard bool
	// Nulls fixes the explicit-null proof. The zero value advertises neither
	// IsNull nor NotNull.
	Nulls NullMapping[T]
	// Operators is the exact immutable standard-operator declaration.
	Operators weave.OperatorSet
}

// MappedField is the sealed heterogeneous descriptor interface accepted by
// NewMapping. Field[T] values created by this package implement it.
type MappedField interface {
	Path() string
	Capabilities() weave.FieldCapabilities
	elasticsearchFieldMetadata() fieldMetadata
}

// Field is an immutable typed assertion about one explicitly managed
// Elasticsearch mapping field. Valid values contain only package-owned state,
// can be copied, and are safe for concurrent reads.
type Field[T any] struct {
	state *fieldState
}

type fieldState struct {
	path                   string
	mappingType            MappingType
	scalarType             ScalarType
	valueType              reflect.Type
	multiValued            bool
	nested                 bool
	completeValueIndex     bool
	normalizer             string
	allowExpensiveWildcard bool
	nullKind               NullKind
	nullValue              scalarValue
	companion              *companionMarkerState
	operators              weave.OperatorSet
}

type fieldMetadata struct {
	state          *fieldState
	descriptorType reflect.Type
}

type scalarValue struct {
	kind        ScalarType
	stringValue string
	intValue    int64
	floatValue  float64
	boolValue   bool
	timeValue   time.Time
}

// NewField validates spec against T and returns a typed mapping declaration.
// The caller must register the result in a Mapping before using it with a
// Compiler.
func NewField[T any](spec FieldSpec[T]) (Field[T], error) {
	valueType := reflect.TypeFor[T]()
	scalarType := spec.Type.Scalar()
	if !validFieldPath(spec.Path) || !spec.Type.valid() ||
		!scalarAcceptsType(scalarType, valueType) ||
		!validNormalizer(spec.Normalizer) ||
		(spec.Normalizer != "" && spec.Type != MappingKeyword) ||
		(spec.AllowExpensiveWildcard && spec.Type != MappingKeyword) {
		return Field[T]{}, invalidFieldDefinitionError()
	}

	state := &fieldState{
		path:                   spec.Path,
		mappingType:            spec.Type,
		scalarType:             scalarType,
		valueType:              valueType,
		multiValued:            spec.MultiValued,
		nested:                 spec.Nested,
		completeValueIndex:     spec.CompleteValueIndex,
		normalizer:             spec.Normalizer,
		allowExpensiveWildcard: spec.AllowExpensiveWildcard,
		operators:              spec.Operators,
	}

	switch spec.Nulls.kind {
	case NullUntracked:
		state.nullKind = NullUntracked
	case NullValueMarker:
		value, ok := makeScalarValue(scalarType, reflect.ValueOf(spec.Nulls.nullValue))
		if !ok {
			return Field[T]{}, invalidNullMappingError()
		}
		state.nullKind = NullValueMarker
		state.nullValue = value
	case NullCompanionMarker:
		if !spec.Nulls.companion.Valid() {
			return Field[T]{}, invalidNullMappingError()
		}
		state.nullKind = NullCompanionMarker
		state.companion = spec.Nulls.companion.state
	default:
		return Field[T]{}, invalidNullMappingError()
	}

	if state.nullKind != NullUntracked &&
		(state.multiValued || state.nested || state.mappingType.Analyzed() ||
			!state.completeValueIndex) {
		return Field[T]{}, invalidNullMappingError()
	}

	maximum := maximumFieldOperators(state)
	if !operatorSubset(state.operators, maximum) {
		return Field[T]{}, operatorNotApplicableError()
	}
	if !validFieldState(state) {
		return Field[T]{}, invalidFieldDefinitionError()
	}
	return Field[T]{state: state}, nil
}

// Path returns the canonical Elasticsearch field path. An invalid Field
// returns an empty string.
func (field Field[T]) Path() string {
	if !field.valid() {
		return ""
	}
	return field.state.path
}

// MappingType returns the fixed Elasticsearch mapping family.
func (field Field[T]) MappingType() MappingType {
	if !field.valid() {
		return 0
	}
	return field.state.mappingType
}

// ScalarType returns the logical scalar compatible with T.
func (field Field[T]) ScalarType() ScalarType {
	if !field.valid() {
		return 0
	}
	return field.state.scalarType
}

// Keyword reports whether the field is mapped as keyword.
func (field Field[T]) Keyword() bool {
	return field.valid() && field.state.mappingType.Keyword()
}

// Analyzed reports whether the field uses analyzed full-text semantics.
func (field Field[T]) Analyzed() bool {
	return field.valid() && field.state.mappingType.Analyzed()
}

// MultiValued reports the immutable cardinality declaration.
func (field Field[T]) MultiValued() bool {
	return field.valid() && field.state.multiValued
}

// Nested reports whether the field requires Elasticsearch nested query scope.
func (field Field[T]) Nested() bool {
	return field.valid() && field.state.nested
}

// CompleteValueIndex reports the explicit indexed-value coverage assertion.
func (field Field[T]) CompleteValueIndex() bool {
	return field.valid() && field.state.completeValueIndex
}

// Normalizer returns the exact keyword normalizer name. Empty means none.
func (field Field[T]) Normalizer() string {
	if !field.valid() {
		return ""
	}
	return field.state.normalizer
}

// AllowsExpensiveWildcard reports the field-level leading-wildcard opt-in.
func (field Field[T]) AllowsExpensiveWildcard() bool {
	return field.valid() && field.state.allowExpensiveWildcard
}

// NullKind returns the explicit-null proof declared by the field.
func (field Field[T]) NullKind() NullKind {
	if !field.valid() {
		return NullUntracked
	}
	return field.state.nullKind
}

// NullMarkerPath returns the companion marker path, or an empty string when
// the field does not use a companion marker.
func (field Field[T]) NullMarkerPath() string {
	if !field.valid() || field.state.nullKind != NullCompanionMarker {
		return ""
	}
	return field.state.companion.field.path
}

// Capabilities returns the exact immutable operator declaration in FieldSpec.
// Compiler.CapabilitiesFor additionally applies Profile policy and Mapping
// identity.
func (field Field[T]) Capabilities() weave.FieldCapabilities {
	if !field.valid() {
		return weave.FieldCapabilities{}
	}
	return weave.FieldCapabilities{Operators: field.state.operators}
}

func (field Field[T]) elasticsearchFieldMetadata() fieldMetadata {
	return fieldMetadata{
		state:          field.state,
		descriptorType: reflect.TypeFor[Field[T]](),
	}
}

func (field Field[T]) valid() bool {
	return field.state != nil &&
		field.state.valueType == reflect.TypeFor[T]() &&
		validFieldState(field.state)
}

func (metadata fieldMetadata) validFor(actualType reflect.Type) bool {
	return metadata.state != nil && metadata.descriptorType != nil &&
		metadata.descriptorType == actualType &&
		metadata.state.valueType != nil && validFieldState(metadata.state)
}

func validFieldState(state *fieldState) bool {
	if state == nil || !validFieldPath(state.path) || !state.mappingType.valid() ||
		state.scalarType != state.mappingType.Scalar() ||
		!scalarAcceptsType(state.scalarType, state.valueType) ||
		!validNormalizer(state.normalizer) ||
		(state.normalizer != "" && state.mappingType != MappingKeyword) ||
		(state.allowExpensiveWildcard && state.mappingType != MappingKeyword) {
		return false
	}

	switch state.nullKind {
	case NullUntracked:
		if state.companion != nil {
			return false
		}
	case NullValueMarker:
		if state.nullValue.kind != state.scalarType || state.companion != nil {
			return false
		}
	case NullCompanionMarker:
		if state.companion == nil ||
			!validCompanionMarkerState(state.companion) {
			return false
		}
	default:
		return false
	}

	if state.nullKind != NullUntracked &&
		(state.multiValued || state.nested || state.mappingType.Analyzed() ||
			!state.completeValueIndex) {
		return false
	}
	return operatorSubset(state.operators, maximumFieldOperators(state))
}

func validCompanionMarkerState(state *companionMarkerState) bool {
	return state != nil && validCompanionFieldState(state.field) &&
		validMarkerTerm(state.nullTerm) && validMarkerTerm(state.valueTerm) &&
		state.nullTerm != state.valueTerm
}

func validCompanionFieldState(state *fieldState) bool {
	return state != nil && validFieldPath(state.path) &&
		state.mappingType == MappingKeyword && state.scalarType == ScalarString &&
		state.valueType == reflect.TypeFor[string]() &&
		!state.multiValued && !state.nested && state.completeValueIndex &&
		state.normalizer == "" && !state.allowExpensiveWildcard &&
		state.nullKind == NullUntracked && state.companion == nil &&
		state.operators.Count() == 0
}

func maximumFieldOperators(state *fieldState) weave.OperatorSet {
	if state == nil || state.multiValued || state.nested ||
		state.mappingType.Analyzed() || !state.completeValueIndex {
		return weave.OperatorSet{}
	}

	operators := []weave.Operator{
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
	}
	switch state.mappingType {
	case MappingLong, MappingDouble:
		operators = append(operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
			weave.OperatorBetween,
		)
	case MappingDate:
		operators = append(operators,
			weave.OperatorLT,
			weave.OperatorLTE,
			weave.OperatorGT,
			weave.OperatorGTE,
		)
	case MappingKeyword:
		operators = append(operators, weave.OperatorHasPrefix)
		if state.allowExpensiveWildcard {
			operators = append(operators,
				weave.OperatorContains,
				weave.OperatorHasSuffix,
			)
		}
	case MappingWildcard:
		operators = append(operators,
			weave.OperatorContains,
			weave.OperatorHasPrefix,
			weave.OperatorHasSuffix,
		)
	}
	if state.nullKind != NullUntracked {
		operators = append(operators,
			weave.OperatorIsNull,
			weave.OperatorNotNull,
		)
	}
	return weave.NewOperatorSet(operators...)
}

func scalarAcceptsType(scalar ScalarType, valueType reflect.Type) bool {
	if !scalar.valid() || valueType == nil {
		return false
	}
	switch scalar {
	case ScalarString:
		return valueType.Kind() == reflect.String
	case ScalarInteger:
		switch valueType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return valueType != reflect.TypeFor[time.Duration]()
		default:
			return false
		}
	case ScalarFloat:
		return valueType.Kind() == reflect.Float32 || valueType.Kind() == reflect.Float64
	case ScalarDateTime:
		return valueType == reflect.TypeFor[time.Time]()
	case ScalarBoolean:
		return valueType.Kind() == reflect.Bool
	default:
		return false
	}
}

func makeScalarValue(scalar ScalarType, value reflect.Value) (scalarValue, bool) {
	if !value.IsValid() || !scalarAcceptsType(scalar, value.Type()) {
		return scalarValue{}, false
	}
	result := scalarValue{kind: scalar}
	switch scalar {
	case ScalarString:
		result.stringValue = value.String()
		return result, utf8.ValidString(result.stringValue)
	case ScalarInteger:
		result.intValue = value.Int()
		return result, true
	case ScalarFloat:
		result.floatValue = value.Float()
		return result, !math.IsNaN(result.floatValue) && !math.IsInf(result.floatValue, 0)
	case ScalarDateTime:
		result.timeValue = value.Interface().(time.Time)
		return result, true
	case ScalarBoolean:
		result.boolValue = value.Bool()
		return result, true
	default:
		return scalarValue{}, false
	}
}

func operatorSubset(subset weave.OperatorSet, superset weave.OperatorSet) bool {
	for _, operator := range allStandardOperators() {
		if subset.Has(operator) && !superset.Has(operator) {
			return false
		}
	}
	return true
}

func allStandardOperators() []weave.Operator {
	return []weave.Operator{
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
	}
}

func validFieldPath(path string) bool {
	if path == "" || !utf8.ValidString(path) || strings.TrimSpace(path) != path {
		return false
	}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" || segment[0] == '$' {
			return false
		}
		for index, character := range segment {
			if character == 0 || unicode.IsControl(character) {
				return false
			}
			if index == 0 {
				if character != '_' && !unicode.IsLetter(character) {
					return false
				}
				continue
			}
			if character != '_' && !unicode.IsLetter(character) &&
				!unicode.IsDigit(character) {
				return false
			}
		}
	}
	return true
}

func validNormalizer(normalizer string) bool {
	if normalizer == "" {
		return true
	}
	if !utf8.ValidString(normalizer) || strings.TrimSpace(normalizer) != normalizer {
		return false
	}
	for index, character := range normalizer {
		if index == 0 {
			if character != '_' && !unicode.IsLetter(character) {
				return false
			}
			continue
		}
		if character != '_' && character != '-' &&
			!unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func validMarkerTerm(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func invalidFieldDefinitionError() error {
	return fmt.Errorf(
		"elasticsearch: invalid field mapping declaration: %w",
		weave.ErrInvalidField,
	)
}

func invalidNullMappingError() error {
	return fmt.Errorf(
		"elasticsearch: invalid explicit-null mapping declaration: %w",
		weave.ErrInvalidValue,
	)
}

func invalidCompanionMarkerError() error {
	return fmt.Errorf(
		"elasticsearch: invalid companion null marker declaration: %w",
		weave.ErrInvalidField,
	)
}

func operatorNotApplicableError() error {
	return fmt.Errorf(
		"elasticsearch: operator is not applicable to the field mapping: %w",
		weave.ErrOperatorNotApplicable,
	)
}
