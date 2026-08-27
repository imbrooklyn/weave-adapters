package goqu

import "strconv"

// Profile identifies the SQL dialect semantics used by a Compiler. A profile
// is fixed for the Compiler's lifetime and is not inferred from a database or
// dataset. Its numeric representation is an implementation detail, not a
// persistence, serialization, or interchange protocol. Profile values are
// immutable, comparable, and safe to copy; the zero value is invalid.
type Profile uint8

const (
	// MySQL selects the MySQL semantic profile.
	MySQL Profile = iota + 1
	// PostgreSQL selects the PostgreSQL semantic profile.
	PostgreSQL
)

func (profile Profile) valid() bool {
	return profile == MySQL || profile == PostgreSQL
}

func (profile Profile) dialectName() string {
	if profile == PostgreSQL {
		return "postgres"
	}
	return "mysql"
}

// String returns a stable English diagnostic identifier for profile.
func (profile Profile) String() string {
	switch profile {
	case MySQL:
		return "mysql"
	case PostgreSQL:
		return "postgresql"
	default:
		return "profile(" + strconv.FormatUint(uint64(profile), 10) + ")"
	}
}
