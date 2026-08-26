//go:build integration

package gormgen

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/model"
	"github.com/imbrooklyn/weave-adapters/gormgen/internal/fixture/query"
	"github.com/imbrooklyn/weave/compilertest"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gen/field"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type integrationProfile struct {
	name           string
	environmentKey string
	profile        Profile
	open           func(string) gorm.Dialector
	createTableSQL string
}

var postgresqlPlaceholder = regexp.MustCompile(`\$[0-9]+`)

func TestIntegrationProfiles(t *testing.T) {
	profiles := []integrationProfile{
		{
			name:           "mysql",
			environmentKey: "WEAVE_GORMGEN_MYSQL_DSN",
			profile:        MySQL,
			open: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
			createTableSQL: `CREATE TABLE weave_gormgen_records (
id VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
number_value BIGINT NOT NULL,
text_value TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
nullable_number BIGINT NULL,
nullable_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
equality_only_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`,
		},
		{
			name:           "postgresql",
			environmentKey: "WEAVE_GORMGEN_POSTGRES_DSN",
			profile:        PostgreSQL,
			open: func(dsn string) gorm.Dialector {
				return postgres.Open(dsn)
			},
			createTableSQL: `CREATE TABLE weave_gormgen_records (
id VARCHAR(16) COLLATE "C" NOT NULL PRIMARY KEY,
number_value BIGINT NOT NULL,
text_value TEXT COLLATE "C" NOT NULL,
nullable_number BIGINT NULL,
nullable_text TEXT COLLATE "C" NULL,
equality_only_text TEXT COLLATE "C" NOT NULL
)`,
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
		t.Skip("set WEAVE_GORMGEN_MYSQL_DSN and/or WEAVE_GORMGEN_POSTGRES_DSN")
	}
}

func runIntegrationProfile(t *testing.T, profile integrationProfile, dsn string) {
	t.Helper()
	database, err := gorm.Open(profile.open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open %s database: %v", profile.name, err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("obtain %s sql.DB: %v", profile.name, err)
	}
	sqlDatabase.SetMaxOpenConns(40)
	t.Cleanup(func() {
		if err := sqlDatabase.Close(); err != nil {
			t.Errorf("close %s database: %v", profile.name, err)
		}
	})
	if err := sqlDatabase.Ping(); err != nil {
		t.Fatalf("ping %s database: %v", profile.name, err)
	}

	if err := database.Exec("DROP TABLE IF EXISTS weave_gormgen_records").Error; err != nil {
		t.Fatalf("drop stale %s fixture table: %v", profile.name, err)
	}
	if err := database.Exec(profile.createTableSQL).Error; err != nil {
		t.Fatalf("create %s fixture table: %v", profile.name, err)
	}
	t.Cleanup(func() {
		if err := database.Exec("DROP TABLE IF EXISTS weave_gormgen_records").Error; err != nil {
			t.Errorf("drop %s fixture table: %v", profile.name, err)
		}
	})

	logAndCheckBackend(t, profile, database)
	seedIntegrationRecords(t, database)

	queries := query.Use(database)
	equalityOnly, err := NewFieldSpec[string](
		queries.SemanticRecord.EqualityOnlyText,
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorIsNull,
		weave.OperatorNotNull,
	)
	if err != nil {
		t.Fatalf("NewFieldSpec(equality-only): %v", err)
	}
	compiler, err := NewCompiler(profile.profile, WithFieldSpecs(equalityOnly))
	if err != nil {
		t.Fatalf("NewCompiler(%s): %v", profile.name, err)
	}
	factory := weave.NewFactory[Conditions, Expression](compiler)

	var inspectionCalls atomic.Int64
	execute := func(conditions Conditions) ([]string, error) {
		return executeIntegrationQuery(database, conditions)
	}
	harness := compilertest.Harness[Conditions, Expression]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           queries.SemanticRecord.Number,
			Text:             queries.SemanticRecord.Text,
			NullableNumber:   queries.SemanticRecord.NullableNumber,
			NullableText:     queries.SemanticRecord.NullableText,
			EqualityOnlyText: queries.SemanticRecord.EqualityOnlyText,
		},
		Resolver: compiler,
		Execute:  execute,
		InspectCondition: func(caseName string, conditions Conditions) error {
			inspectionCalls.Add(1)
			return inspectIntegrationCondition(
				profile.profile,
				database,
				caseName,
				conditions,
			)
		},
		NativeCondition: func(ids []string) Conditions {
			values := append([]string(nil), ids...)
			return ConditionsOf(queries.SemanticRecord.ID.In(values...))
		},
		NativeExpression: func(ids []string) Expression {
			values := append([]string(nil), ids...)
			return queries.SemanticRecord.ID.In(values...)
		},
		NilLikeNativeExpression: func() Expression {
			var expression *field.String
			return expression
		},
		DistinguishesMissing: false,
	}

	compilertest.Run(t, harness)
	if inspectionCalls.Load() == 0 {
		t.Fatal("compilertest did not inspect compiled SQL conditions")
	}
	runSQLStorageCases(t, factory, execute)
	runInjectionProbe(t, profile.profile, factory, queries, database)
}

func seedIntegrationRecords(t *testing.T, database *gorm.DB) {
	t.Helper()
	shared := compilertest.Records()
	records := make([]*model.SemanticRecord, len(shared))
	for index := range shared {
		records[index] = &model.SemanticRecord{
			ID:               shared[index].ID,
			Number:           shared[index].Number,
			Text:             shared[index].Text,
			NullableNumber:   shared[index].NullableNumber,
			NullableText:     shared[index].NullableText,
			EqualityOnlyText: shared[index].Text,
		}
	}
	if err := query.Use(database).SemanticRecord.Create(records...); err != nil {
		t.Fatalf("seed generated DAO fixture: %v", err)
	}
}

func executeIntegrationQuery(
	database *gorm.DB,
	conditions Conditions,
) ([]string, error) {
	records := query.Use(database).SemanticRecord
	matched, err := records.Where(conditions...).Order(records.ID).Find()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(matched))
	for index := range matched {
		ids[index] = matched[index].ID
	}
	return ids, nil
}

func inspectIntegrationCondition(
	profile Profile,
	database *gorm.DB,
	caseName string,
	conditions Conditions,
) error {
	dryRun := database.Session(&gorm.Session{DryRun: true, NewDB: true})
	records := query.Use(dryRun).SemanticRecord
	statement := records.Where(conditions...).UnderlyingDB().Find(
		&[]model.SemanticRecord{},
	).Statement
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
		if _, ok := value.(field.Expr); ok {
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
	case "constant true root", "constant false empty any", "explicit null only", "not null means value":
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
	execute func(Conditions) ([]string, error),
) {
	t.Helper()
	allIDs := []string{"r01", "r02", "r03", "r04", "r05", "r06"}
	for _, test := range []struct {
		name  string
		build func(*weave.Builder[Conditions, Expression])
		want  []string
	}{
		{
			name: "empty AllOf is true",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.AllOf(func(*Group) {})
			},
			want: allIDs,
		},
		{
			name: "empty AnyOf is false",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.AnyOf(func(*Group) {})
			},
		},
		{
			name: "empty NoneOf is true",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.NoneOf(func(*Group) {})
			},
			want: allIDs,
		},
		{
			name: "empty NotAllOf is false",
			build: func(builder *weave.Builder[Conditions, Expression]) {
				builder.NotAllOf(func(*Group) {})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := factory.New()
			test.build(builder)
			conditions, err := builder.Build()
			if err != nil {
				t.Fatalf("Build(empty group): %v", err)
			}
			assertIntegrationIDs(t, execute, conditions, test.want)
		})
	}
}

func runInjectionProbe(
	t *testing.T,
	profile Profile,
	factory *Factory,
	queries *query.Query,
	database *gorm.DB,
) {
	t.Helper()
	const value = "x' OR 1=1 -- %_!"
	conditions, err := factory.New().Contains(
		queries.SemanticRecord.Text,
		value,
	).Build()
	if err != nil {
		t.Fatalf("Build(injection probe): %v", err)
	}
	if err := inspectIntegrationCondition(
		profile,
		database,
		"literal injection probe",
		conditions,
	); err != nil {
		t.Fatalf("inspect injection probe: %v", err)
	}
	dryRun := database.Session(&gorm.Session{DryRun: true, NewDB: true})
	records := query.Use(dryRun).SemanticRecord
	statement := records.Where(conditions...).UnderlyingDB().Find(
		&[]model.SemanticRecord{},
	).Statement
	if statement.Error != nil {
		t.Fatalf("render injection probe: %v", statement.Error)
	}
	if strings.Contains(statement.SQL.String(), value) {
		t.Fatalf("injection probe SQL contains the query value")
	}
	const wantPattern = "%x' OR 1=1 -- !%!_!!%"
	if len(statement.Vars) != 1 || statement.Vars[0] != wantPattern {
		t.Fatalf("injection probe Vars = %#v, want one escaped bound pattern", statement.Vars)
	}
	ids, err := executeIntegrationQuery(database, conditions)
	if err != nil {
		t.Fatalf("execute injection probe: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("injection probe matched IDs %v, want none", ids)
	}
	t.Log("parameterized injection-shaped literal matched no records")
}

func assertIntegrationIDs(
	t *testing.T,
	execute func(Conditions) ([]string, error),
	conditions Conditions,
	want []string,
) {
	t.Helper()
	got, err := execute(conditions)
	if err != nil {
		t.Fatalf("execute conditions: %v", err)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("matching IDs = %v, want %v", got, want)
	}
}

func logAndCheckBackend(
	t *testing.T,
	profile integrationProfile,
	database *gorm.DB,
) {
	t.Helper()
	var version string
	if err := database.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatalf("read %s server version: %v", profile.name, err)
	}
	t.Logf("real backend version: %s", version)

	var collation string
	var result *gorm.DB
	if profile.profile == MySQL {
		result = database.Raw(`SELECT TABLE_COLLATION
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'weave_gormgen_records'`).Scan(&collation)
		if result.Error == nil && collation != "utf8mb4_bin" {
			result.Error = fmt.Errorf("table collation = %q, want utf8mb4_bin", collation)
		}
	} else {
		result = database.Raw(`SELECT collation_name
FROM information_schema.columns
WHERE table_schema = current_schema()
AND table_name = 'weave_gormgen_records'
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
