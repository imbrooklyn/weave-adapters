package elasticsearch

import (
	"fmt"
	"reflect"

	"github.com/imbrooklyn/weave"
)

// Mapping is an immutable registry of typed Elasticsearch Field declarations.
// It is copied by value and is safe for concurrent reads. Its zero value is
// invalid.
type Mapping struct {
	state *mappingState
}

type mappingState struct {
	byPath map[string]*fieldState
	fields map[*fieldState]struct{}
}

// NewMapping validates and registers heterogeneous Field values. Companion
// marker fields must be included in the same Mapping as every field that
// references them. The registry copies no caller-owned maps or slices.
func NewMapping(fields ...MappedField) (Mapping, error) {
	state := &mappingState{
		byPath: make(map[string]*fieldState, len(fields)),
		fields: make(map[*fieldState]struct{}, len(fields)),
	}
	for _, descriptor := range fields {
		if isNilMappedField(descriptor) {
			return Mapping{}, invalidMappingError()
		}
		metadata := descriptor.elasticsearchFieldMetadata()
		if !metadata.validFor(reflect.TypeOf(descriptor)) {
			return Mapping{}, invalidMappingError()
		}
		field := metadata.state
		if _, exists := state.fields[field]; exists {
			return Mapping{}, invalidMappingError()
		}
		if _, exists := state.byPath[field.path]; exists {
			return Mapping{}, invalidMappingError()
		}
		state.fields[field] = struct{}{}
		state.byPath[field.path] = field
	}

	for field := range state.fields {
		if field.nullKind != NullCompanionMarker {
			continue
		}
		marker := field.companion.field
		if marker == field {
			return Mapping{}, invalidMappingError()
		}
		if _, exists := state.fields[marker]; !exists {
			return Mapping{}, invalidMappingError()
		}
	}
	return Mapping{state: state}, nil
}

// Valid reports whether mapping was created by NewMapping.
func (mapping Mapping) Valid() bool {
	return mapping.state != nil && mapping.state.byPath != nil &&
		mapping.state.fields != nil
}

// FieldCount returns the number of registered typed declarations.
func (mapping Mapping) FieldCount() int {
	if !mapping.Valid() {
		return 0
	}
	return len(mapping.state.fields)
}

// HasPath reports whether path is registered. Path is matched exactly and is
// not normalized or expanded.
func (mapping Mapping) HasPath(path string) bool {
	if !mapping.Valid() {
		return false
	}
	_, ok := mapping.state.byPath[path]
	return ok
}

func (state *mappingState) contains(field *fieldState) bool {
	if state == nil || field == nil {
		return false
	}
	_, ok := state.fields[field]
	return ok
}

func isNilMappedField(field MappedField) bool {
	if field == nil {
		return true
	}
	value := reflect.ValueOf(field)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}

func invalidMappingError() error {
	return fmt.Errorf(
		"elasticsearch: invalid or duplicate mapping field: %w",
		weave.ErrInvalidField,
	)
}
