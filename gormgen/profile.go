package gormgen

// Profile identifies the SQL dialect semantics used by a Compiler. A
// Compiler profile is fixed for the lifetime of the Compiler and is not
// inferred from a database handle.
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
