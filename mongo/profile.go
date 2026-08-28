package mongo

import "strconv"

// Profile identifies the MongoDB server semantics used by a Compiler. A
// profile is fixed for the Compiler's lifetime and is not inferred from a
// client or server. Its numeric representation is an implementation detail,
// not a persistence, serialization, or interchange protocol. Profile values
// are immutable, comparable, and safe to copy; the zero value is invalid.
type Profile uint8

const (
	// MongoDB60Plus selects filter and PCRE semantics shared by MongoDB 6.0 and
	// newer supported server versions. The caller is responsible for using the
	// resulting filters only with a compatible deployment.
	MongoDB60Plus Profile = iota + 1
)

func (profile Profile) valid() bool {
	return profile == MongoDB60Plus
}

// String returns a stable English diagnostic identifier for profile.
func (profile Profile) String() string {
	switch profile {
	case MongoDB60Plus:
		return "mongodb_6_0_plus"
	default:
		return "profile(" + strconv.FormatUint(uint64(profile), 10) + ")"
	}
}
