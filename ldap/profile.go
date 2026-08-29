package ldap

import "strconv"

// Profile identifies the LDAP filter semantics used by a Compiler. Its
// numeric representation is an implementation detail and its zero value is
// invalid.
type Profile uint8

const (
	// RFC4515 selects the RFC 4515 string filter grammar together with the
	// RFC 4511 filter evaluation boundary documented by this package.
	RFC4515 Profile = iota + 1
)

func (profile Profile) valid() bool {
	return profile == RFC4515
}

// String returns a stable English diagnostic identifier for profile.
func (profile Profile) String() string {
	switch profile {
	case RFC4515:
		return "rfc4515"
	default:
		return "profile(" + strconv.FormatUint(uint64(profile), 10) + ")"
	}
}
