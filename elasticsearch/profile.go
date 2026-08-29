package elasticsearch

import "strconv"

// Profile identifies one immutable Elasticsearch 9.5 query-policy contract.
// It is selected by application configuration and is never discovered from a
// client or live cluster. Its numeric representation is an implementation
// detail and its zero value is invalid.
type Profile uint8

const (
	// Elasticsearch95ExpensiveQueries selects Elasticsearch 9.5 Query DSL
	// semantics and asserts that search.allow_expensive_queries is true.
	Elasticsearch95ExpensiveQueries Profile = iota + 1
	// Elasticsearch95NoExpensiveQueries selects Elasticsearch 9.5 Query DSL
	// semantics and asserts that search.allow_expensive_queries is false.
	Elasticsearch95NoExpensiveQueries
)

func (profile Profile) valid() bool {
	return profile == Elasticsearch95ExpensiveQueries ||
		profile == Elasticsearch95NoExpensiveQueries
}

func (profile Profile) allowsExpensiveQueries() bool {
	return profile == Elasticsearch95ExpensiveQueries
}

// AllowsExpensiveQueries reports the cluster policy asserted by profile. It
// reports false for the zero value and unknown profiles.
func (profile Profile) AllowsExpensiveQueries() bool {
	return profile.valid() && profile.allowsExpensiveQueries()
}

// String returns a stable English diagnostic identifier for profile.
func (profile Profile) String() string {
	switch profile {
	case Elasticsearch95ExpensiveQueries:
		return "elasticsearch_9_5_expensive_queries"
	case Elasticsearch95NoExpensiveQueries:
		return "elasticsearch_9_5_no_expensive_queries"
	default:
		return "profile(" + strconv.FormatUint(uint64(profile), 10) + ")"
	}
}
