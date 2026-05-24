package backups_config_physical

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"databasus-backend/internal/storage"
)

type BackupConfigRepository struct{}

func (r *BackupConfigRepository) Save(
	backupConfig *PhysicalBackupConfig,
) (*PhysicalBackupConfig, error) {
	if backupConfig.Storage != nil && backupConfig.Storage.ID != uuid.Nil {
		backupConfig.StorageID = &backupConfig.Storage.ID
	}

	if err := storage.GetDb().Save(backupConfig).Omit("Storage", "PostgresqlPhysical").Error; err != nil {
		return nil, err
	}

	return r.FindByDatabaseID(backupConfig.DatabaseID)
}

func (r *BackupConfigRepository) FindByDatabaseID(
	databaseID uuid.UUID,
) (*PhysicalBackupConfig, error) {
	var backupConfig PhysicalBackupConfig

	if err := storage.
		GetDb().
		Preload("PostgresqlPhysical").
		Preload("Storage").
		Preload("Storage.LocalStorage").
		Preload("Storage.S3Storage").
		Preload("Storage.GoogleDriveStorage").
		Preload("Storage.NASStorage").
		Preload("Storage.AzureBlobStorage").
		Preload("Storage.FTPStorage").
		Preload("Storage.SFTPStorage").
		Preload("Storage.RcloneStorage").
		Where("database_id = ?", databaseID).
		First(&backupConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backupConfig, nil
}

func (r *BackupConfigRepository) GetWithEnabledBackups() ([]*PhysicalBackupConfig, error) {
	var backupConfigs []*PhysicalBackupConfig

	if err := storage.
		GetDb().
		Preload("PostgresqlPhysical").
		Preload("Storage").
		Preload("Storage.LocalStorage").
		Preload("Storage.S3Storage").
		Preload("Storage.GoogleDriveStorage").
		Preload("Storage.NASStorage").
		Preload("Storage.AzureBlobStorage").
		Preload("Storage.FTPStorage").
		Preload("Storage.SFTPStorage").
		Preload("Storage.RcloneStorage").
		Where("is_backups_enabled = ?", true).
		Find(&backupConfigs).Error; err != nil {
		return nil, err
	}

	return backupConfigs, nil
}

func (r *BackupConfigRepository) IsStorageUsing(storageID uuid.UUID) (bool, error) {
	var count int64

	if err := storage.
		GetDb().
		Table("physical_backup_configs").
		Where("storage_id = ?", storageID).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func (r *BackupConfigRepository) GetDatabasesIDsByStorageID(
	storageID uuid.UUID,
) ([]uuid.UUID, error) {
	var databasesIDs []uuid.UUID

	if err := storage.
		GetDb().
		Table("physical_backup_configs").
		Where("storage_id = ?", storageID).
		Pluck("database_id", &databasesIDs).Error; err != nil {
		return nil, err
	}

	return databasesIDs, nil
}
