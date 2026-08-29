package ldap

import (
	"errors"
	"fmt"
	"unicode/utf8"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/imbrooklyn/weave"
)

const objectClassOID = "2.5.4.0"

const maxFilterTextBytes = 64 * 1024

// A legal Predicate can add one LDAP level per negative Logic, while a
// deepest Expr can itself contain MaxPredicateDepth levels. The extra four
// levels cover the root conjunction and the deepest standard lowering. Raw
// ParseFilter and Expr input remain limited to weave.MaxPredicateDepth.
const maxGeneratedFilterDepth = 3*weave.MaxPredicateDepth + 4

var errInvalidFilter = errors.New("ldap: invalid filter syntax or schema")

// ParseFilter validates source against the fixed filter grammar and schema
// allowlists, canonicalizes it through the locked go-ldap filter codec, and
// returns an immutable Schema-bound Filter. Errors do not include source or
// any assertion value. ParseFilter validates complete filter text; it cannot
// identify caller-controlled assertion values and does not escape them before
// parsing. The returned Filter can contain query data and should not be logged
// blindly.
func ParseFilter(schema Schema, source string) (Filter, error) {
	if !schema.Valid() {
		return Filter{}, fmt.Errorf(
			"ldap: invalid schema state: %w",
			weave.ErrInvalidState,
		)
	}
	filter, err := newFilter(schema.state, source, false)
	if err != nil {
		return Filter{}, fmt.Errorf(
			"ldap: rejected filter: %w",
			weave.ErrInvalidValue,
		)
	}
	return filter, nil
}

func newFilter(schema *schemaState, source string, allowConstantAnchor bool) (Filter, error) {
	canonical, err := canonicalFilter(schema, source, allowConstantAnchor)
	if err != nil {
		return Filter{}, err
	}
	return Filter{state: &filterState{schema: schema, text: canonical}}, nil
}

func canonicalFilter(
	schema *schemaState,
	source string,
	allowConstantAnchor bool,
) (string, error) {
	if err := validateFilterText(schema, source, allowConstantAnchor); err != nil {
		return "", errInvalidFilter
	}
	packet, err := ldapv3.CompileFilter(source)
	if err != nil {
		return "", errInvalidFilter
	}
	canonical, err := ldapv3.DecompileFilter(packet)
	if err != nil || canonical == "" {
		return "", errInvalidFilter
	}
	if err := validateFilterText(schema, canonical, allowConstantAnchor); err != nil {
		return "", errInvalidFilter
	}
	packet, err = ldapv3.CompileFilter(canonical)
	if err != nil {
		return "", errInvalidFilter
	}
	again, err := ldapv3.DecompileFilter(packet)
	if err != nil || again != canonical {
		return "", errInvalidFilter
	}
	return canonical, nil
}

func validateFilterText(
	schema *schemaState,
	source string,
	allowConstantAnchor bool,
) error {
	if schema == nil || source == "" || len(source) > maxFilterTextBytes ||
		!utf8.ValidString(source) {
		return errInvalidFilter
	}
	parser := filterParser{
		schema:              schema,
		source:              source,
		allowConstantAnchor: allowConstantAnchor,
		maximumDepth:        weave.MaxPredicateDepth,
	}
	if allowConstantAnchor {
		parser.maximumDepth = maxGeneratedFilterDepth
	}
	if !parser.parseFilter(0) || parser.position != len(source) {
		return errInvalidFilter
	}
	return nil
}

type filterParser struct {
	schema              *schemaState
	source              string
	position            int
	allowConstantAnchor bool
	maximumDepth        int
}

func (parser *filterParser) parseFilter(depth int) bool {
	if depth > parser.maximumDepth {
		return false
	}
	if !parser.consume('(') || parser.position >= len(parser.source) {
		return false
	}
	switch parser.source[parser.position] {
	case '&', '|':
		parser.position++
		children := 0
		for parser.position < len(parser.source) &&
			parser.source[parser.position] == '(' {
			if !parser.parseFilter(depth + 1) {
				return false
			}
			children++
		}
		return children > 0 && parser.consume(')')
	case '!':
		parser.position++
		if parser.position >= len(parser.source) ||
			parser.source[parser.position] != '(' || !parser.parseFilter(depth+1) {
			return false
		}
		return parser.consume(')')
	default:
		return parser.parseItem() && parser.consume(')')
	}
}

func (parser *filterParser) parseItem() bool {
	start := parser.position
	for parser.position < len(parser.source) {
		switch parser.source[parser.position] {
		case '=', '>', '<', '~', ':':
			attributeText := parser.source[start:parser.position]
			switch parser.source[parser.position] {
			case '=':
				parser.position++
				return parser.parseEquality(attributeText)
			case '>':
				return parser.parseOrdered(attributeText, true)
			case '<':
				return parser.parseOrdered(attributeText, false)
			case '~':
				return false
			case ':':
				return parser.parseExtensible(attributeText)
			}
		case '(', ')', '\\', 0:
			return false
		}
		parser.position++
	}
	return false
}

func (parser *filterParser) parseEquality(attributeText string) bool {
	attribute, anchor, ok := parser.attribute(attributeText)
	if !ok {
		return false
	}
	stars, consecutive, valueLength, ok := parser.scanAssertion(true)
	if !ok || consecutive {
		return false
	}
	if stars == 1 && valueLength == 1 {
		return !anchor || attributeText == objectClassOID
	}
	if anchor {
		return false
	}
	if stars == 0 {
		return attribute.matching.equality != ""
	}
	return attribute.matching.equality != "" && attribute.matching.substring != ""
}

func (parser *filterParser) parseOrdered(attributeText string, greater bool) bool {
	attribute, anchor, ok := parser.attribute(attributeText)
	if !ok || anchor || parser.position+1 >= len(parser.source) ||
		parser.source[parser.position+1] != '=' {
		return false
	}
	parser.position += 2
	_, _, _, ok = parser.scanAssertion(false)
	if !ok || attribute.matching.ordering == "" {
		return false
	}
	return greater || attribute.matching.equality != ""
}

func (parser *filterParser) parseExtensible(attributeText string) bool {
	attribute, anchor, ok := parser.attribute(attributeText)
	if !ok || anchor {
		return false
	}
	tokens := make([]string, 0, 2)
	for {
		if !parser.consume(':') || parser.position >= len(parser.source) {
			return false
		}
		if parser.source[parser.position] == '=' {
			parser.position++
			break
		}
		start := parser.position
		for parser.position < len(parser.source) &&
			parser.source[parser.position] != ':' &&
			parser.source[parser.position] != ')' {
			parser.position++
		}
		if start == parser.position || parser.position >= len(parser.source) ||
			parser.source[parser.position] != ':' {
			return false
		}
		tokens = append(tokens, parser.source[start:parser.position])
	}

	seenDN := false
	matchingRule := ""
	for _, token := range tokens {
		if token == "dn" {
			if seenDN || matchingRule != "" {
				return false
			}
			seenDN = true
			continue
		}
		if matchingRule != "" || !validNumericOID(token) {
			return false
		}
		matchingRule = token
	}
	if matchingRule == "" {
		if attribute.matching.equality == "" {
			return false
		}
	} else if !attribute.matching.contains(matchingRule) {
		return false
	}
	_, _, _, ok = parser.scanAssertion(false)
	return ok
}

func (parser *filterParser) attribute(
	description string,
) (*attributeState, bool, bool) {
	canonical := canonicalDescription(description)
	if canonical == "" {
		return nil, false, false
	}
	if attribute, ok := parser.schema.lookup(canonical); ok {
		return attribute, false, true
	}
	if canonical == objectClassOID && parser.allowConstantAnchor {
		return &attributeState{}, true, true
	}
	return nil, false, false
}

func (parser *filterParser) scanAssertion(
	allowStar bool,
) (stars int, consecutive bool, length int, ok bool) {
	previousStar := false
	for parser.position < len(parser.source) {
		character := parser.source[parser.position]
		switch character {
		case ')':
			return stars, consecutive, length, true
		case '(':
			return 0, false, 0, false
		case 0:
			return 0, false, 0, false
		case '\\':
			if parser.position+2 >= len(parser.source) ||
				!hexByte(parser.source[parser.position+1]) ||
				!hexByte(parser.source[parser.position+2]) {
				return 0, false, 0, false
			}
			parser.position += 3
			length += 3
			previousStar = false
			continue
		case '*':
			if !allowStar {
				return 0, false, 0, false
			}
			stars++
			consecutive = consecutive || previousStar
			previousStar = true
		default:
			previousStar = false
		}
		parser.position++
		length++
	}
	return 0, false, 0, false
}

func (parser *filterParser) consume(want byte) bool {
	if parser.position >= len(parser.source) || parser.source[parser.position] != want {
		return false
	}
	parser.position++
	return true
}

func hexByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}

func filterSchemaMatches(filter Filter, schema *schemaState) bool {
	return filter.Valid() && filter.state.schema == schema
}
