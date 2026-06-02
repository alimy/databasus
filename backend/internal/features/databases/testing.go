package databases

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/google/uuid"

	"databasus-backend/internal/config"
	"databasus-backend/internal/features/databases/databases/mariadb"
	"databasus-backend/internal/features/databases/databases/mongodb"
	postgresql_logical "databasus-backend/internal/features/databases/databases/postgresql/logical"
	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	postgresql_shared "databasus-backend/internal/features/databases/databases/postgresql/shared"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	"databasus-backend/internal/util/tools"
)

func GetTestPostgresConfig() *postgresql_logical.PostgresqlLogicalDatabase {
	env := config.GetEnv()
	port, err := strconv.Atoi(env.TestLogicalPostgres16Port)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_LOGICAL_POSTGRES_16_PORT: %v", err))
	}

	testDbName := "testdb"
	return &postgresql_logical.PostgresqlLogicalDatabase{
		Version:  tools.PostgresqlVersion16,
		Host:     config.GetEnv().TestLocalhost,
		Port:     port,
		Username: "testuser",
		Password: "testpassword",
		Database: &testDbName,
		CpuCount: 1,
	}
}

// Physical test cluster ports are validated non-empty in config.go at startup
// (it os.Exit(1)s with the exact env var when one is unset), so these helpers
// only parse the value — a missing container DSN fails the run there, never here
// and never as a skip.
func GetTestPhysicalPostgresConfig(versionTag string) *postgresql_physical.PostgresqlPhysicalDatabase {
	return GetTestPhysicalPostgresConfigWithType(versionTag, postgresql_physical.BackupTypeFullOnly)
}

// GetTestPhysicalPostgresConfigWithType is GetTestPhysicalPostgresConfig with an
// explicit BackupType, so scheduler-driven tests can build chains
// (FULL_AND_INCREMENTAL) or stream WAL (FULL_INCREMENTAL_AND_WAL_STREAM) — the
// scheduler's incremental decision keys off this DB-level field, which the
// backup-config API cannot change.
func GetTestPhysicalPostgresConfigWithType(
	versionTag string,
	backupType postgresql_physical.BackupType,
) *postgresql_physical.PostgresqlPhysicalDatabase {
	env := config.GetEnv()

	var portStr string
	var version tools.PostgresqlVersion

	switch versionTag {
	case "17":
		portStr = env.TestPhysicalPostgres17Port
		version = tools.PostgresqlVersion17
	case "18":
		portStr = env.TestPhysicalPostgres18Port
		version = tools.PostgresqlVersion18
	default:
		panic(fmt.Sprintf("unsupported physical postgres version tag: %s (use \"17\" or \"18\")", versionTag))
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse physical postgres %s port: %v", versionTag, err))
	}

	return &postgresql_physical.PostgresqlPhysicalDatabase{
		Version:    version,
		Host:       env.TestLocalhost,
		Port:       port,
		Username:   "testuser",
		Password:   "testpassword",
		BackupType: backupType,
	}
}

func GetTestPhysicalPostgresConfigNoSummary(versionTag string) *postgresql_physical.PostgresqlPhysicalDatabase {
	env := config.GetEnv()

	var portStr string
	var version tools.PostgresqlVersion

	switch versionTag {
	case "17":
		portStr = env.TestPhysicalPostgres17NoSummaryPort
		version = tools.PostgresqlVersion17
	case "18":
		portStr = env.TestPhysicalPostgres18NoSummaryPort
		version = tools.PostgresqlVersion18
	default:
		panic(fmt.Sprintf("unsupported physical postgres version tag: %s (use \"17\" or \"18\")", versionTag))
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse physical postgres %s no-summary port: %v", versionTag, err))
	}

	return &postgresql_physical.PostgresqlPhysicalDatabase{
		Version:    version,
		Host:       env.TestLocalhost,
		Port:       port,
		Username:   "testuser",
		Password:   "testpassword",
		BackupType: postgresql_physical.BackupTypeFullOnly,
	}
}

func GetTestPhysicalPostgresConfigWithTablespace(versionTag string) *postgresql_physical.PostgresqlPhysicalDatabase {
	env := config.GetEnv()

	var portStr string
	var version tools.PostgresqlVersion

	switch versionTag {
	case "17":
		portStr = env.TestPhysicalPostgres17TablespacePort
		version = tools.PostgresqlVersion17
	case "18":
		portStr = env.TestPhysicalPostgres18TablespacePort
		version = tools.PostgresqlVersion18
	default:
		panic(fmt.Sprintf("unsupported physical postgres version tag: %s (use \"17\" or \"18\")", versionTag))
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse physical postgres %s tablespace port: %v", versionTag, err))
	}

	return &postgresql_physical.PostgresqlPhysicalDatabase{
		Version:    version,
		Host:       env.TestLocalhost,
		Port:       port,
		Username:   "testuser",
		Password:   "testpassword",
		BackupType: postgresql_physical.BackupTypeFullOnly,
	}
}

// GetTestPhysicalPostgresConfigMtls builds a physical config pointed at the
// replication-capable mTLS test cluster, with client certs read from the
// physical testdata/mtls fixtures. BackupType is WAL_STREAM so the streamer path
// is exercised over mTLS; callers that only need a FULL can override it.
func GetTestPhysicalPostgresConfigMtls(versionTag string) *postgresql_physical.PostgresqlPhysicalDatabase {
	env := config.GetEnv()

	if versionTag != "17" {
		panic(fmt.Sprintf("physical mTLS test cluster only exists for version 17, got %q", versionTag))
	}

	port, err := strconv.Atoi(env.TestPhysicalPostgres17MtlsPort)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_PHYSICAL_POSTGRES_17_MTLS_PORT: %v", err))
	}

	clientCert, clientKey, rootCert := readPhysicalMtlsCerts()

	// FULL_ONLY keeps the shared fixture's config-save valid (WAL_STREAM would
	// require CHAINS retention + a lag threshold). The WAL-stream-over-mTLS test
	// constructs the streamer spec directly against this DB, so the declared
	// backup type does not gate it.
	return &postgresql_physical.PostgresqlPhysicalDatabase{
		Version:       tools.PostgresqlVersion17,
		Host:          env.TestLocalhost,
		Port:          port,
		Username:      "testuser",
		Password:      "testpassword",
		BackupType:    postgresql_physical.BackupTypeFullOnly,
		SslMode:       postgresql_shared.PostgresSslModeVerifyCA,
		SslClientCert: clientCert,
		SslClientKey:  clientKey,
		SslRootCert:   rootCert,
	}
}

// readPhysicalMtlsCerts reads the committed client cert/key + CA from the
// physical mTLS testdata. Paths are resolved relative to this package so the
// helper works regardless of the calling test's directory.
func readPhysicalMtlsCerts() (clientCert, clientKey, rootCert string) {
	read := func(name string) string {
		content, err := os.ReadFile(filepath.Join(physicalMtlsTestdataDir(), name))
		if err != nil {
			panic(fmt.Sprintf("failed to read physical mTLS test certificate %s: %v", name, err))
		}

		return string(content)
	}

	return read("client.crt"), read("client.key"), read("ca.crt")
}

// physicalMtlsTestdataDir resolves the committed cert directory from this source
// file's location, so it is independent of the test's working directory.
func physicalMtlsTestdataDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve physical mTLS testdata dir: runtime.Caller failed")
	}

	return filepath.Join(
		filepath.Dir(thisFile), "..", "tests", "physical", "testdata", "mtls",
	)
}

// CreateTestPhysicalPostgresDatabaseMtls persists a physical DB row pointed at
// the mTLS replication cluster, for FULL- and WAL-stream-over-mTLS tests.
func CreateTestPhysicalPostgresDatabaseMtls(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
	versionTag string,
) *Database {
	database := &Database{
		WorkspaceID:        &workspaceID,
		Name:               "test-physical-pg-mtls " + uuid.New().String(),
		Type:               DatabaseTypePostgresPhysical,
		PostgresqlPhysical: GetTestPhysicalPostgresConfigMtls(versionTag),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

func GetTestMariadbConfig() *mariadb.MariadbDatabase {
	env := config.GetEnv()
	portStr := env.TestLogicalMariadb1011Port
	if portStr == "" {
		portStr = "33111"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_LOGICAL_MARIADB_1011_PORT: %v", err))
	}

	testDbName := "testdb"
	return &mariadb.MariadbDatabase{
		Version:  tools.MariadbVersion1011,
		Host:     config.GetEnv().TestLocalhost,
		Port:     port,
		Username: "testuser",
		Password: "testpassword",
		Database: &testDbName,
	}
}

func GetTestMongodbConfig() *mongodb.MongodbDatabase {
	env := config.GetEnv()
	portStr := env.TestLogicalMongodb70Port
	if portStr == "" {
		portStr = "27070"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_LOGICAL_MONGODB_70_PORT: %v", err))
	}

	return &mongodb.MongodbDatabase{
		Version:      tools.MongodbVersion7,
		Host:         config.GetEnv().TestLocalhost,
		Port:         &port,
		Username:     "root",
		Password:     "rootpassword",
		Database:     "testdb",
		AuthDatabase: "admin",
		IsHttps:      false,
		IsSrv:        false,
		CpuCount:     1,
	}
}

func CreateTestDatabase(
	workspaceID uuid.UUID,
	storage *storages.Storage,
	notifier *notifiers.Notifier,
) *Database {
	database := &Database{
		WorkspaceID:       &workspaceID,
		Name:              "test " + uuid.New().String(),
		Type:              DatabaseTypePostgresLogical,
		PostgresqlLogical: GetTestPostgresConfig(),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

func CreateTestPhysicalPostgresDatabase(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
	versionTag string,
) *Database {
	return CreateTestPhysicalPostgresDatabaseWithType(
		workspaceID, notifier, versionTag, postgresql_physical.BackupTypeFullOnly)
}

// CreateTestPhysicalPostgresDatabaseWithType is CreateTestPhysicalPostgresDatabase
// with an explicit BackupType, for scheduler-driven incremental / WAL-stream
// chains whose eligibility the scheduler reads from this DB-level field.
func CreateTestPhysicalPostgresDatabaseWithType(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
	versionTag string,
	backupType postgresql_physical.BackupType,
) *Database {
	database := &Database{
		WorkspaceID:        &workspaceID,
		Name:               "test-physical-pg " + uuid.New().String(),
		Type:               DatabaseTypePostgresPhysical,
		PostgresqlPhysical: GetTestPhysicalPostgresConfigWithType(versionTag, backupType),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

// CreateTestPhysicalPostgresDatabaseNoSummary points at the cluster started
// with summarize_wal=off, so incremental pre-checks reach the SUMMARIZER_OFF
// branch deterministically (ALTER SYSTEM cannot override a command-line GUC on
// the standard fixture).
func CreateTestPhysicalPostgresDatabaseNoSummary(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
	versionTag string,
) *Database {
	database := &Database{
		WorkspaceID:        &workspaceID,
		Name:               "test-physical-pg-no-summary " + uuid.New().String(),
		Type:               DatabaseTypePostgresPhysical,
		PostgresqlPhysical: GetTestPhysicalPostgresConfigNoSummary(versionTag),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

func CreateTestMariadbDatabase(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
) *Database {
	database := &Database{
		WorkspaceID: &workspaceID,
		Name:        "test-mariadb " + uuid.New().String(),
		Type:        DatabaseTypeMariadb,
		Mariadb:     GetTestMariadbConfig(),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

func CreateTestMongodbDatabase(
	workspaceID uuid.UUID,
	notifier *notifiers.Notifier,
) *Database {
	database := &Database{
		WorkspaceID: &workspaceID,
		Name:        "test-mongodb " + uuid.New().String(),
		Type:        DatabaseTypeMongodb,
		Mongodb:     GetTestMongodbConfig(),
		Notifiers: []notifiers.Notifier{
			*notifier,
		},
	}

	database, err := databaseRepository.Save(database)
	if err != nil {
		panic(err)
	}

	return database
}

func RemoveTestDatabase(database *Database) {
	if err := databaseService.DeleteForTest(database.ID); err != nil {
		panic(fmt.Sprintf("failed to delete test database: %v", err))
	}
}
