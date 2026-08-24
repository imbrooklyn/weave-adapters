package gorm

// Profile identifies the SQL dialect semantics used by a Compiler. A profile
// is fixed for the Compiler's lifetime and is not inferred from a database or
// Dialector.
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
