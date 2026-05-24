package backups_config_physical

import "github.com/google/uuid"

type BackupConfigStorageChangeListener interface {
	OnBeforeBackupsStorageChange(dbID uuid.UUID) error
}
