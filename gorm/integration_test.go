//go:build integration

package gorm

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave/compilertest"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	upstream "gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const integrationTable = "weave_gorm_records"

type integrationProfile struct {
	name           string
	environmentKey string
	profile        Profile
	open           func(string) upstream.Dialector
	createTableSQL string
	wantVersion    string
}

type integrationRecord struct {
	ID               string  `gorm:"column:id;primaryKey"`
	Number           int64   `gorm:"column:number_value"`
	Text             string  `gorm:"column:text_value"`
	NullableNumber   *int64  `gorm:"column:nullable_number"`
	NullableText     *string `gorm:"column:nullable_text"`
	EqualityOnlyText string  `gorm:"column:equality_only_text"`
}

func (integrationRecord) TableName() string {
	return integrationTable
}

type nilIntegrationExpression struct{}

func (*nilIntegrationExpression) Build(clause.Builder) {}

var postgresqlPlaceholder = regexp.MustCompile(`\$[0-9]+`)

func TestIntegrationProfiles(t *testing.T) {
	profiles := []integrationProfile{
		{
			name:           "mysql",
			environmentKey: "WEAVE_GORM_MYSQL_DSN",
			profile:        MySQL,
			open: func(dsn string) upstream.Dialector {
				return mysql.Open(dsn)
			},
			createTableSQL: `CREATE TABLE weave_gorm_records (
id VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
number_value BIGINT NOT NULL,
text_value TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
nullable_number BIGINT NULL,
nullable_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
equality_only_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`,
			wantVersion: "8.0.40",
		},
		{
			name:           "postgresql",
			environmentKey: "WEAVE_GORM_POSTGRES_DSN",
			profile:        PostgreSQL,
			open: func(dsn string) upstream.Dialector {
				return postgres.Open(dsn)
			},
			createTableSQL: `CREATE TABLE weave_gorm_records (
id VARCHAR(16) COLLATE "C" NOT NULL PRIMARY KEY,
number_value BIGINT NOT NULL,
text_value TEXT COLLATE "C" NOT NULL,
nullable_number BIGINT NULL,
nullable_text TEXT COLLATE "C" NULL,
equality_only_text TEXT COLLATE "C" NOT NULL
)`,
			wantVersion: "PostgreSQL 15.12",
		},
	}

	configured := 0
	for _, profile := range profiles {
		dsn := os.Getenv(profile.environmentKey)
		if dsn == "" {
			continue
		}
		configured++
		t.Run(profile.name, func(t *testing.T) {
			runIntegrationProfile(t, profile, dsn)
		})
	}
	if configured == 0 {
		t.Skip("set WEAVE_GORM_MYSQL_DSN and/or WEAVE_GORM_POSTGRES_DSN")
	}
}

func runIntegrationProfile(
	t *testing.T,
	profile integrationProfile,
	dsn string,
) {
	t.Helper()
	database, err := upstream.Open(profile.open(dsn), &upstream.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open %s database: %v", profile.name, err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("obtain %s sql.DB: %v", profile.name, err)
	}
	sqlDatabase.SetMaxOpenConns(80)
	t.Cleanup(func() {
		if err := sqlDatabase.Close(); err != nil {
			t.Errorf("close %s database: %v", profile.name, err)
		}
	})
	if err := sqlDatabase.Ping(); err != nil {
		t.Fatalf("ping %s database: %v", profile.name, err)
	}

	if err := database.Exec("DROP TABLE IF EXISTS " + integrationTable).Error; err != nil {
		t.Fatalf("drop stale %s fixture table: %v", profile.name, err)
	}
	if err := database.Exec(profile.createTableSQL).Error; err != nil {
		t.Fatalf("create %s fixture table: %v", profile.name, err)
	}
	t.Cleanup(func() {
		if err := database.Exec("DROP TABLE IF EXISTS " + integrationTable).Error; err != nil {
			t.Errorf("drop %s fixture table: %v", profile.name, err)
		}
	})

	logAndCheckIntegrationBackend(t, profile, database)
	seedIntegrationRecords(t, database)

	number := MustQualifiedField[int64](integrationTable, "number_value")
	text := MustQualifiedField[string](integrationTable, "text_value")
	nullableNumber := MustQualifiedField[int64](integrationTable, "nullable_number")
	nullableText := MustQualifiedField[string](integrationTable, "nullable_text")
	equalityOnlyText := MustQualifiedField[string](
		integrationTable,
		"equality_only_text",
		WithOperators(
			weave.OperatorEQ,
			weave.OperatorNEQ,
			weave.OperatorIn,
			weave.OperatorNotIn,
			weave.OperatorIsNull,
			weave.OperatorNotNull,
		),
	)
	compiler, err := NewCompiler(profile.profile)
	if err != nil {
		t.Fatalf("NewCompiler(%s): %v", profile.name, err)
	}
	factory := weave.NewFactory[Condition, Expression](compiler)

	var traditionalCalls atomic.Int64
	var genericCalls atomic.Int64
	execute := func(condition Condition) ([]string, error) {
		return executeBothEntrypoints(
			context.Background(),
			database,
			condition,
			&traditionalCalls,
			&genericCalls,
		)
	}
	var inspectionCalls atomic.Int64
	harness := compilertest.Harness[Condition, Expression]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           number,
			Text:             text,
			NullableNumber:   nullableNumber,
			NullableText:     nullableText,
			EqualityOnlyText: equalityOnlyText,
		},
		Resolver: compiler,
		Execute:  execute,
		InspectCondition: func(caseName string, condition Condition) error {
			inspectionCalls.Add(1)
			return inspectIntegrationCondition(
				profile.profile,
				database,
				caseName,
				condition,
			)
		},
		NativeCondition: func(ids []string) Condition {
			return newIDExpression(ids)
		},
		NativeExpression: func(ids []string) Expression {
			return newIDExpression(ids)
		},
		NilLikeNativeCondition: func() Condition {
			var expression *nilIntegrationExpression
			return expression
		},
		NilLikeNativeExpression: func() Expression {
			var expression *nilIntegrationExpression
			return expression
		},
		DistinguishesMissing: false,
	}

	compilertest.Run(t, harness)
	if inspectionCalls.Load() == 0 {
		t.Fatal("compilertest did not inspect compiled SQL conditions")
	}
	runSQLStorageCases(t, factory, number, nullableNumber, execute)
	runInjectionProbe(t, profile.profile, factory, text, database, execute)
	if traditionalCalls.Load() == 0 || genericCalls.Load() == 0 ||
		traditionalCalls.Load() != genericCalls.Load() {
		t.Fatalf(
			"entrypoint calls = (traditional=%d, generics=%d)",
			traditionalCalls.Load(),
			genericCalls.Load(),
		)
	}
	t.Logf(
		"public GORM entrypoint comparisons: traditional=%d generics=%d",
		traditionalCalls.Load(),
		genericCalls.Load(),
	)
}

func seedIntegrationRecords(t *testing.T, database *upstream.DB) {
	t.Helper()
	shared := compilertest.Records()
	records := make([]integrationRecord, len(shared))
	for index := range shared {
		records[index] = integrationRecord{
			ID:               shared[index].ID,
			Number:           shared[index].Number,
			Text:             shared[index].Text,
			NullableNumber:   shared[index].NullableNumber,
			NullableText:     shared[index].NullableText,
			EqualityOnlyText: shared[index].Text,
		}
	}
	if err := database.Create(&records).Error; err != nil {
		t.Fatalf("seed native GORM fixture: %v", err)
	}
}

func executeBothEntrypoints(
	ctx context.Context,
	database *upstream.DB,
	condition Condition,
	traditionalCalls *atomic.Int64,
	genericCalls *atomic.Int64,
) ([]string, error) {
	var traditional []integrationRecord
	result := database.
		Where(condition).
		Order("id").
		Find(&traditional)
	traditionalCalls.Add(1)
	if result.Error != nil {
		return nil, fmt.Errorf("traditional GORM query: %w", result.Error)
	}

	generic, err := upstream.G[integrationRecord](database).
		Where(condition).
		Order("id").
		Find(ctx)
	genericCalls.Add(1)
	if err != nil {
		return nil, fmt.Errorf("generic GORM query: %w", err)
	}
	traditionalIDs := integrationIDs(traditional)
	genericIDs := integrationIDs(generic)
	if !slices.Equal(traditionalIDs, genericIDs) {
		return nil, fmt.Errorf(
			"traditional and generic GORM ID sets differ: %v != %v",
			traditionalIDs,
			genericIDs,
		)
	}
	return traditionalIDs, nil
}

func integrationIDs(records []integrationRecord) []string {
	ids := make([]string, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	slices.Sort(ids)
	return ids
}

func newIDExpression(ids []string) clause.Expression {
	values := make([]any, len(ids))
	for index := range ids {
		values[index] = ids[index]
	}
	return clause.IN{
		Column: clause.Column{Table: integrationTable, Name: "id"},
		Values: values,
	}
}

func inspectIntegrationCondition(
	profile Profile,
	database *upstream.DB,
	caseName string,
	condition Condition,
) error {
	dryRun := database.Session(&upstream.Session{DryRun: true, NewDB: true})
	statement := dryRun.Where(condition).Find(&[]integrationRecord{}).Statement
	if statement.Error != nil {
		return statement.Error
	}
	sqlText := statement.SQL.String()
	for _, value := range []string{
		"prefix",
		"r01",
		"r02",
		"r03",
		"r04",
		"r06",
		".*",
		"\u4e16\u754c\nend",
		compilertest.LiteralSpecialText,
	} {
		if strings.Contains(sqlText, value) {
			return fmt.Errorf("SQL for %q contains a query value", caseName)
		}
	}
	for _, value := range statement.Vars {
		if _, ok := value.(clause.Column); ok {
			return fmt.Errorf("SQL for %q retained a column as a bound value", caseName)
		}
		if _, ok := value.(clause.Expression); ok {
			return fmt.Errorf("SQL for %q retained an expression as a bound value", caseName)
		}
	}
	placeholderCount := strings.Count(sqlText, "?")
	if profile == PostgreSQL {
		placeholderCount = len(postgresqlPlaceholder.FindAllString(sqlText, -1))
	}
	if placeholderCount != len(statement.Vars) {
		return fmt.Errorf(
			"SQL for %q has %d placeholders and %d bound values",
			caseName,
			placeholderCount,
			len(statement.Vars),
		)
	}
	if strings.HasPrefix(caseName, "literal ") &&
		!strings.Contains(sqlText, "ESCAPE '!'") {
		return fmt.Errorf("SQL for %q has no fixed ESCAPE clause", caseName)
	}
	switch caseName {
	case "constant true root", "constant false empty any", "not null means value":
	default:
		if len(statement.Vars) == 0 {
			return fmt.Errorf("SQL for %q has no bound query value", caseName)
		}
	}
	return nil
}

func runSQLStorageCases(
	t *testing.T,
	factory *Factory,
	number Field[int64],
	nullableNumber Field[int64],
	execute func(Condition) ([]string, error),
) {
	t.Helper()
	allIDs := []string{"r01", "r02", "r03", "r04", "r05", "r06"}
	for _, test := range []struct {
		name  string
		build func(*weave.Builder[Condition, Expression])
		want  []string
	}{
		{
			name: "empty AllOf is true",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.AllOf(func(*Group) {})
			},
			want: allIDs,
		},
		{
			name: "empty AnyOf is false",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.AnyOf(func(*Group) {})
			},
		},
		{
			name: "empty NoneOf is true",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.NoneOf(func(*Group) {})
			},
			want: allIDs,
		},
		{
			name: "empty NotAllOf is false",
			build: func(builder *weave.Builder[Condition, Expression]) {
				builder.NotAllOf(func(*Group) {})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := factory.New()
			test.build(builder)
			condition, err := builder.Build()
			if err != nil {
				t.Fatalf("Build(empty group): %v", err)
			}
			assertIntegrationIDs(t, execute, condition, test.want)
		})
	}
	t.Run("SQL storage null and missing collapse", func(t *testing.T) {
		condition, err := factory.New().IsNull(nullableNumber).Build()
		if err != nil {
			t.Fatalf("Build(IsNull): %v", err)
		}
		assertIntegrationIDs(t, execute, condition, []string{"r03", "r04"})
	})
	t.Run("nullable In includes SQL null", func(t *testing.T) {
		two := int64(2)
		condition, err := factory.New().In(
			nullableNumber,
			[]*int64{&two, nil, &two},
		).Build()
		if err != nil {
			t.Fatalf("Build(nullable In): %v", err)
		}
		assertIntegrationIDs(
			t,
			execute,
			condition,
			[]string{"r02", "r03", "r04", "r06"},
		)
	})
	t.Run("eight-level group nesting", func(t *testing.T) {
		condition, err := factory.New().AllOf(func(group *Group) {
			addDeepAllOf(group, number, 8)
		}).Build()
		if err != nil {
			t.Fatalf("Build(deep group): %v", err)
		}
		assertIntegrationIDs(t, execute, condition, []string{"r05"})
	})
}

func addDeepAllOf(group *Group, number Field[int64], remaining int) {
	if remaining == 0 {
		group.EQ(number, int64(5))
		return
	}
	group.AllOf(func(child *Group) {
		addDeepAllOf(child, number, remaining-1)
	})
}

func runInjectionProbe(
	t *testing.T,
	profile Profile,
	factory *Factory,
	text Field[string],
	database *upstream.DB,
	execute func(Condition) ([]string, error),
) {
	t.Helper()
	const value = "x' OR 1=1 -- %_!"
	condition, err := factory.New().Contains(text, value).Build()
	if err != nil {
		t.Fatalf("Build(injection probe): %v", err)
	}
	if err := inspectIntegrationCondition(
		profile,
		database,
		"literal injection probe",
		condition,
	); err != nil {
		t.Fatalf("inspect injection probe: %v", err)
	}
	dryRun := database.Session(&upstream.Session{DryRun: true, NewDB: true})
	statement := dryRun.Where(condition).Find(&[]integrationRecord{}).Statement
	if statement.Error != nil {
		t.Fatalf("render injection probe: %v", statement.Error)
	}
	if strings.Contains(statement.SQL.String(), value) {
		t.Fatal("injection probe SQL contains the query value")
	}
	const wantPattern = "%x' OR 1=1 -- !%!_!!%"
	if len(statement.Vars) != 1 || statement.Vars[0] != wantPattern {
		t.Fatalf("injection probe Vars = %#v, want one escaped bound pattern", statement.Vars)
	}
	assertIntegrationIDs(t, execute, condition, nil)
	t.Log("parameterized injection-shaped literal matched no records")
}

func assertIntegrationIDs(
	t *testing.T,
	execute func(Condition) ([]string, error),
	condition Condition,
	want []string,
) {
	t.Helper()
	got, err := execute(condition)
	if err != nil {
		t.Fatalf("execute condition: %v", err)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("matching IDs = %v, want %v", got, want)
	}
}

func logAndCheckIntegrationBackend(
	t *testing.T,
	profile integrationProfile,
	database *upstream.DB,
) {
	t.Helper()
	var version string
	if err := database.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatalf("read %s server version: %v", profile.name, err)
	}
	if !strings.HasPrefix(version, profile.wantVersion) {
		t.Fatalf(
			"%s server version = %q, want prefix %q",
			profile.name,
			version,
			profile.wantVersion,
		)
	}
	t.Logf("real backend version: %s", version)

	var collation string
	var result *upstream.DB
	if profile.profile == MySQL {
		result = database.Raw(`SELECT TABLE_COLLATION
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'weave_gorm_records'`).Scan(&collation)
		if result.Error == nil && collation != "utf8mb4_bin" {
			result.Error = fmt.Errorf("table collation = %q, want utf8mb4_bin", collation)
		}
	} else {
		result = database.Raw(`SELECT collation_name
FROM information_schema.columns
WHERE table_schema = current_schema()
AND table_name = 'weave_gorm_records'
AND column_name = 'text_value'`).Scan(&collation)
		if result.Error == nil && collation != "C" {
			result.Error = fmt.Errorf("text collation = %q, want C", collation)
		}
	}
	if result.Error != nil {
		t.Fatalf("check %s fixture collation: %v", profile.name, result.Error)
	}
	t.Logf("controlled fixture collation: %s", collation)
}
