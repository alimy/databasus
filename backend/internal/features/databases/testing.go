package databases

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"databasus-backend/internal/config"
	"databasus-backend/internal/features/databases/databases/mariadb"
	"databasus-backend/internal/features/databases/databases/mongodb"
	"databasus-backend/internal/features/databases/databases/postgresql/logical"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/tools"
)

func GetTestPostgresConfig() *postgresql_logical.PostgresqlLogicalDatabase {
	env := config.GetEnv()
	port, err := strconv.Atoi(env.TestPostgres16Port)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_POSTGRES_16_PORT: %v", err))
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

func GetTestMariadbConfig() *mariadb.MariadbDatabase {
	env := config.GetEnv()
	portStr := env.TestMariadb1011Port
	if portStr == "" {
		portStr = "33111"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_MARIADB_1011_PORT: %v", err))
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
	portStr := env.TestMongodb70Port
	if portStr == "" {
		portStr = "27070"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse TEST_MONGODB_70_PORT: %v", err))
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
		WorkspaceID: &workspaceID,
		Name:        "test " + uuid.New().String(),
		Type:        DatabaseTypePostgres,
		Postgresql:  GetTestPostgresConfig(),
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
	// Delete backups and backup configs associated with this database
	// We hardcode SQL here because we cannot call backups feature due to DI inversion
	// (databases package cannot import backups package as backups already imports databases)
	db := storage.GetDb()

	if err := db.Exec("DELETE FROM logical_backups WHERE database_id = ?", database.ID).Error; err != nil {
		panic(fmt.Sprintf("failed to delete backups: %v", err))
	}

	if err := db.Exec("DELETE FROM logical_backup_configs WHERE database_id = ?", database.ID).Error; err != nil {
		panic(fmt.Sprintf("failed to delete backup config: %v", err))
	}

	err := databaseRepository.Delete(database.ID)
	if err != nil {
		panic(err)
	}
}
