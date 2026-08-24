package gorm

import (
	"reflect"
	"testing"

	"gorm.io/driver/mysql"
	upstream "gorm.io/gorm"
)

func FuzzCompileLiteralText(f *testing.F) {
	for _, seed := range []string{
		"",
		"plain",
		"%_!",
		"\u4e16\u754c\nend",
		"x' OR 1=1 -- %_!",
	} {
		f.Add(seed)
	}
	factory, err := NewFactory(MySQL)
	if err != nil {
		f.Fatal(err)
	}
	field := MustQualifiedField[string]("weave_gorm_records", "name")
	database, err := upstream.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?parseTime=true",
		SkipInitializeWithVersion: true,
	}), &upstream.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
	})
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 4096 {
			t.Skip()
		}
		condition, err := factory.New().Contains(field, value).Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		statement := database.Session(&upstream.Session{NewDB: true}).
			Where(condition).
			Find(&[]benchmarkRecord{}).
			Statement
		if statement.Error != nil {
			t.Fatalf("DryRun error = %v", statement.Error)
		}
		wantVars := []any{"%" + escapeLiteralText(value) + "%"}
		if !reflect.DeepEqual(statement.Vars, wantVars) {
			t.Fatalf("Vars = %#v, want %#v", statement.Vars, wantVars)
		}
	})
}
