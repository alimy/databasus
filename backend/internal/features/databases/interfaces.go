package databases

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"databasus-backend/internal/util/encryption"
)

// ErrPhysicalNotSupported is returned by Database.AsPhysicalSource() while no
// database type yet implements PhysicalBackupSource. Lifted once
// PostgresqlPhysicalDatabase lands.
var ErrPhysicalNotSupported = errors.New("physical backup is not supported for this database type")

type DatabaseValidator interface {
	Validate() error
}

// LogicalBackupSource — what a database connection used for logical
// backup (pg_dump, mysqldump, mariadb-dump, mongodump) must expose.
// Implementations: PostgresqlLogicalDatabase, MysqlDatabase, MariadbDatabase,
// MongodbDatabase.
type LogicalBackupSource interface {
	TestConnection(
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
	) error

	GetRawDbSizeMb(
		ctx context.Context,
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
	) (float64, error)

	HideSensitiveData()
}

// PhysicalBackupSource — what a database connection used for physical
// backup (pg_basebackup + WAL streaming, PITR) must expose. No implementations
// yet — populated when PostgresqlPhysicalDatabase lands.
type PhysicalBackupSource interface {
	TestReplicationConnection(
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
	) error

	GetClusterSizeMb(
		ctx context.Context,
		logger *slog.Logger,
		encryptor encryption.FieldEncryptor,
	) (float64, error)

	HideSensitiveData()
}

type DatabaseCreationListener interface {
	OnDatabaseCreated(databaseID uuid.UUID)
}

type DatabaseRemoveListener interface {
	OnBeforeDatabaseRemove(databaseID uuid.UUID) error
}

type DatabaseCopyListener interface {
	OnDatabaseCopied(originalDatabaseID, newDatabaseID uuid.UUID)
}
