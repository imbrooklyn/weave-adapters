package ldap

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/imbrooklyn/weave"
)

// Schema is an immutable allowlist of typed LDAP attribute declarations. A
// Schema is copied by value and is safe for concurrent reads. Its zero value is
// invalid.
type Schema struct {
	state *schemaState
}

type schemaState struct {
	byDescription map[string]*attributeState
	byOID         map[string]*attributeState
	attributes    map[*attributeState]struct{}
}

// NewSchema validates and registers Attribute values. The variadic any form
// permits differently typed Attribute[T] values in one Schema. Only values
// created by NewAttribute are accepted.
func NewSchema(attributes ...any) (Schema, error) {
	state := &schemaState{
		byDescription: make(map[string]*attributeState, len(attributes)),
		byOID:         make(map[string]*attributeState, len(attributes)),
		attributes:    make(map[*attributeState]struct{}, len(attributes)),
	}
	for _, value := range attributes {
		descriptor, ok := value.(attributeDescriptor)
		if !ok {
			return Schema{}, invalidSchemaError()
		}
		metadata := descriptor.ldapAttributeMetadata()
		if !metadata.validFor(reflect.TypeOf(value)) {
			return Schema{}, invalidSchemaError()
		}
		attribute := metadata.state
		if _, exists := state.attributes[attribute]; exists {
			return Schema{}, invalidSchemaError()
		}
		if _, exists := state.byDescription[attribute.description]; exists {
			return Schema{}, invalidSchemaError()
		}
		if _, exists := state.byOID[attribute.oid]; exists {
			return Schema{}, invalidSchemaError()
		}
		state.attributes[attribute] = struct{}{}
		state.byDescription[attribute.description] = attribute
		state.byOID[attribute.oid] = attribute
	}
	return Schema{state: state}, nil
}

// Valid reports whether schema was created by NewSchema.
func (schema Schema) Valid() bool {
	return schema.state != nil && schema.state.byDescription != nil &&
		schema.state.byOID != nil && schema.state.attributes != nil
}

// AttributeCount returns the number of registered typed declarations.
func (schema Schema) AttributeCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.state.attributes)
}

func (state *schemaState) contains(attribute *attributeState) bool {
	if state == nil || attribute == nil {
		return false
	}
	_, ok := state.attributes[attribute]
	return ok
}

func (state *schemaState) lookup(description string) (*attributeState, bool) {
	if state == nil {
		return nil, false
	}
	if validNumericOID(description) {
		attribute, ok := state.byOID[description]
		return attribute, ok
	}
	attribute, ok := state.byDescription[strings.ToLower(description)]
	return attribute, ok
}

func invalidSchemaError() error {
	return fmt.Errorf(
		"ldap: invalid or duplicate schema attribute: %w",
		weave.ErrInvalidField,
	)
}
