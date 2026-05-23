package databases

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"databasus-backend/internal/features/databases/databases/mariadb"
	"databasus-backend/internal/features/databases/databases/mongodb"
	"databasus-backend/internal/features/databases/databases/mysql"
	"databasus-backend/internal/features/databases/databases/postgresql/logical"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/util/encryption"
)

type Database struct {
	ID uuid.UUID `json:"id" gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`

	// WorkspaceID can be null when a database is created via restore operation
	// outside the context of any workspace
	WorkspaceID *uuid.UUID   `json:"workspaceId" gorm:"column:workspace_id;type:uuid"`
	Name        string       `json:"name"        gorm:"column:name;type:text;not null"`
	Type        DatabaseType `json:"type"        gorm:"column:type;type:text;not null"`

	Postgresql *postgresql_logical.PostgresqlLogicalDatabase `json:"postgresql,omitzero" gorm:"foreignKey:DatabaseID"`
	Mysql      *mysql.MysqlDatabase                          `json:"mysql,omitzero"      gorm:"foreignKey:DatabaseID"`
	Mariadb    *mariadb.MariadbDatabase                      `json:"mariadb,omitzero"    gorm:"foreignKey:DatabaseID"`
	Mongodb    *mongodb.MongodbDatabase                      `json:"mongodb,omitzero"    gorm:"foreignKey:DatabaseID"`

	Notifiers []notifiers.Notifier `json:"notifiers" gorm:"many2many:database_notifiers;"`

	// these fields are not reliable, but
	// they are used for pretty UI
	LastBackupTime         *time.Time `json:"lastBackupTime,omitzero"          gorm:"column:last_backup_time;type:timestamp with time zone"`
	LastBackupErrorMessage *string    `json:"lastBackupErrorMessage,omitempty" gorm:"column:last_backup_error_message;type:text"`

	HealthStatus *HealthStatus `json:"healthStatus" gorm:"column:health_status;type:text;not null"`
}

func (d *Database) Validate() error {
	if d.Name == "" {
		return errors.New("name is required")
	}

	switch d.Type {
	case DatabaseTypePostgres:
		if d.Postgresql == nil {
			return errors.New("postgresql database is required")
		}
		return d.Postgresql.Validate()
	case DatabaseTypeMysql:
		if d.Mysql == nil {
			return errors.New("mysql database is required")
		}
		return d.Mysql.Validate()
	case DatabaseTypeMariadb:
		if d.Mariadb == nil {
			return errors.New("mariadb database is required")
		}
		return d.Mariadb.Validate()
	case DatabaseTypeMongodb:
		if d.Mongodb == nil {
			return errors.New("mongodb database is required")
		}
		return d.Mongodb.Validate()
	default:
		return errors.New("invalid database type: " + string(d.Type))
	}
}

func (d *Database) ValidateUpdate(old, new Database) error {
	// Database type cannot be changed after creation — the entire backup
	// structure (storage files, schedulers, etc.) is tied to the type at
	// creation time. Recreating that state automatically is error-prone;
	// it is safer for the user to create a new database and remove the old.
	if old.Type != new.Type {
		return errors.New("database type cannot be changed; create a new database instead")
	}

	if old.Type == DatabaseTypePostgres && old.Postgresql != nil && new.Postgresql != nil {
		if err := new.Postgresql.ValidateUpdate(old.Postgresql); err != nil {
			return err
		}
	}

	return nil
}

func (d *Database) TestConnection(
	logger *slog.Logger,
	encryptor encryption.FieldEncryptor,
) error {
	source, err := d.AsLogicalSource()
	if err != nil {
		return err
	}

	return source.TestConnection(logger, encryptor)
}

func (d *Database) GetRawDbSizeMb(
	ctx context.Context,
	logger *slog.Logger,
	encryptor encryption.FieldEncryptor,
) (float64, error) {
	source, err := d.AsLogicalSource()
	if err != nil {
		return 0, err
	}

	return source.GetRawDbSizeMb(ctx, logger, encryptor)
}

func (d *Database) IsUserReadOnly(
	ctx context.Context,
	logger *slog.Logger,
	encryptor encryption.FieldEncryptor,
) (bool, []string, error) {
	switch d.Type {
	case DatabaseTypePostgres:
		return d.Postgresql.IsUserReadOnly(ctx, logger, encryptor)
	case DatabaseTypeMysql:
		return d.Mysql.IsUserReadOnly(ctx, logger, encryptor)
	case DatabaseTypeMariadb:
		return d.Mariadb.IsUserReadOnly(ctx, logger, encryptor)
	case DatabaseTypeMongodb:
		return d.Mongodb.IsUserReadOnly(ctx, logger, encryptor)
	default:
		return false, nil, errors.New("read-only check not supported for this database type")
	}
}

func (d *Database) HideSensitiveData() {
	source, err := d.AsLogicalSource()
	if err != nil {
		return
	}

	source.HideSensitiveData()
}

func (d *Database) EncryptSensitiveFields(encryptor encryption.FieldEncryptor) error {
	if d.Postgresql != nil {
		return d.Postgresql.EncryptSensitiveFields(encryptor)
	}
	if d.Mysql != nil {
		return d.Mysql.EncryptSensitiveFields(encryptor)
	}
	if d.Mariadb != nil {
		return d.Mariadb.EncryptSensitiveFields(encryptor)
	}
	if d.Mongodb != nil {
		return d.Mongodb.EncryptSensitiveFields(encryptor)
	}
	return nil
}

func (d *Database) PopulateDbData(
	logger *slog.Logger,
	encryptor encryption.FieldEncryptor,
) error {
	if d.Postgresql != nil {
		return d.Postgresql.PopulateDbData(logger, encryptor)
	}
	if d.Mysql != nil {
		return d.Mysql.PopulateDbData(logger, encryptor)
	}
	if d.Mariadb != nil {
		return d.Mariadb.PopulateDbData(logger, encryptor)
	}
	if d.Mongodb != nil {
		return d.Mongodb.PopulateDbData(logger, encryptor)
	}
	return nil
}

func (d *Database) Update(incoming *Database) {
	d.Name = incoming.Name
	d.Type = incoming.Type
	d.Notifiers = incoming.Notifiers

	switch d.Type {
	case DatabaseTypePostgres:
		if d.Postgresql != nil && incoming.Postgresql != nil {
			d.Postgresql.Update(incoming.Postgresql)
		}
	case DatabaseTypeMysql:
		if d.Mysql != nil && incoming.Mysql != nil {
			d.Mysql.Update(incoming.Mysql)
		}
	case DatabaseTypeMariadb:
		if d.Mariadb != nil && incoming.Mariadb != nil {
			d.Mariadb.Update(incoming.Mariadb)
		}
	case DatabaseTypeMongodb:
		if d.Mongodb != nil && incoming.Mongodb != nil {
			d.Mongodb.Update(incoming.Mongodb)
		}
	}
}

// AsLogicalSource returns the per-DB connector that satisfies LogicalBackupSource.
// Errors if Type has no logical implementation.
func (d *Database) AsLogicalSource() (LogicalBackupSource, error) {
	switch d.Type {
	case DatabaseTypePostgres:
		return d.Postgresql, nil
	case DatabaseTypeMysql:
		return d.Mysql, nil
	case DatabaseTypeMariadb:
		return d.Mariadb, nil
	case DatabaseTypeMongodb:
		return d.Mongodb, nil
	default:
		return nil, errors.New("logical backup not supported for database type: " + string(d.Type))
	}
}

// AsPhysicalSource returns the per-DB connector that satisfies PhysicalBackupSource.
// No database type yet implements physical — always returns ErrPhysicalNotSupported.
// Defined now so callers can be written against a stable signature.
func (d *Database) AsPhysicalSource() (PhysicalBackupSource, error) {
	return nil, ErrPhysicalNotSupported
}
