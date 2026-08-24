package model

import "time"

// User is the stable model used to generate the compile-only DAO fixture.
type User struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	Name      string    `gorm:"column:name"`
	Age       int       `gorm:"column:age"`
	Active    bool      `gorm:"column:active"`
	CreatedAt time.Time `gorm:"column:created_at"`
	Payload   []byte    `gorm:"column:payload"`
}

// TableName returns the fixture table identity.
func (User) TableName() string {
	return "weave_users"
}

// SemanticRecord is the stable generated-DAO model used by the shared
// compilertest fixture against real SQL backends.
type SemanticRecord struct {
	ID               string  `gorm:"column:id;primaryKey"`
	Number           int64   `gorm:"column:number_value"`
	Text             string  `gorm:"column:text_value"`
	NullableNumber   *int64  `gorm:"column:nullable_number"`
	NullableText     *string `gorm:"column:nullable_text"`
	EqualityOnlyText string  `gorm:"column:equality_only_text"`
}

// TableName returns the semantic fixture table identity.
func (SemanticRecord) TableName() string {
	return "weave_gormgen_records"
}
