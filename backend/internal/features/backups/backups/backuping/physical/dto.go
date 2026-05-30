package backuping_physical

import (
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/features/storages"
)

type backupContext struct {
	Config    *backups_config_physical.PhysicalBackupConfig
	Database  *databases.Database
	Storage   *storages.Storage
	MasterKey string
}
