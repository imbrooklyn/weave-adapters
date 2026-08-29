package ldap

import (
	"errors"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"
)

var errInvalidAssertionValue = errors.New("ldap: invalid assertion value")

func encodeAttributeValue(attribute *attributeState, value any) (string, error) {
	if attribute == nil || isNilLike(value) || reflect.TypeOf(value) == nil ||
		!reflect.TypeOf(value).AssignableTo(attribute.valueType) {
		return "", errInvalidAssertionValue
	}
	reflected := reflect.ValueOf(value)
	switch attribute.syntax {
	case SyntaxDirectoryString:
		return validDirectoryString(reflected.String())
	case SyntaxIA5String:
		return validIA5String(reflected.String())
	case SyntaxInteger:
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(reflected.Int(), 10), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return strconv.FormatUint(reflected.Uint(), 10), nil
		default:
			return "", errInvalidAssertionValue
		}
	case SyntaxBoolean:
		if reflected.Bool() {
			return "TRUE", nil
		}
		return "FALSE", nil
	case SyntaxGeneralizedTime:
		instant, ok := value.(time.Time)
		if !ok {
			return "", errInvalidAssertionValue
		}
		instant = instant.UTC()
		if instant.Year() < 0 || instant.Year() > 9999 {
			return "", errInvalidAssertionValue
		}
		return instant.Format("20060102150405.999999999Z"), nil
	case SyntaxOctetString:
		return string(reflected.Bytes()), nil
	default:
		return "", errInvalidAssertionValue
	}
}

func encodeTextValue(attribute *attributeState, value string) (string, error) {
	if attribute == nil {
		return "", errInvalidAssertionValue
	}
	if value == "" && stringSyntax(attribute.syntax) {
		// Empty literal text lowers to presence and is not encoded as an
		// attribute value. Directory String itself remains non-empty.
		return "", nil
	}
	switch attribute.syntax {
	case SyntaxDirectoryString:
		return validDirectoryString(value)
	case SyntaxIA5String:
		return validIA5String(value)
	default:
		return "", errInvalidAssertionValue
	}
}

func validDirectoryString(value string) (string, error) {
	if value == "" || !utf8.ValidString(value) {
		return "", errInvalidAssertionValue
	}
	return value, nil
}

func validIA5String(value string) (string, error) {
	for index := range len(value) {
		if value[index] > 0x7f {
			return "", errInvalidAssertionValue
		}
	}
	return value, nil
}
