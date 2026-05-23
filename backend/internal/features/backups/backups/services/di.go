package backups_services

import (
	"sync"

	audit_logs "databasus-backend/internal/features/audit_logs"
	"databasus-backend/internal/features/backups/backups/backuping/logical"
	backups_core_logical "databasus-backend/internal/features/backups/backups/core/logical"
	backups_download "databasus-backend/internal/features/backups/backups/download"
	"databasus-backend/internal/features/backups/backups/usecases/logical"
	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
	"databasus-backend/internal/features/databases"
	encryption_secrets "databasus-backend/internal/features/encryption/secrets"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	task_cancellation "databasus-backend/internal/features/tasks/cancellation"
	workspaces_services "databasus-backend/internal/features/workspaces/services"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/logger"
)

var taskCancelManager = task_cancellation.GetTaskCancelManager()

var backupService = &BackupService{
	databases.GetDatabaseService(),
	storages.GetStorageService(),
	backups_core_logical.GetBackupRepository(),
	notifiers.GetNotifierService(),
	notifiers.GetNotifierService(),
	backups_config_logical.GetBackupConfigService(),
	encryption_secrets.GetSecretKeyService(),
	encryption.GetFieldEncryptor(),
	usecases_logical.GetCreateBackupUsecase(),
	logger.GetLogger(),
	[]backups_core_logical.BackupRemoveListener{},
	workspaces_services.GetWorkspaceService(),
	audit_logs.GetAuditLogService(),
	taskCancelManager,
	backups_download.GetDownloadTokenService(),
	backuping_logical.GetBackupsScheduler(),
	backuping_logical.GetBackupCleaner(),
}

func GetBackupService() *BackupService {
	return backupService
}

var SetupDependencies = sync.OnceFunc(func() {
	backups_config_logical.
		GetBackupConfigService().
		SetDatabaseStorageChangeListener(backupService)

	databases.GetDatabaseService().AddDbRemoveListener(backupService)
	databases.GetDatabaseService().AddDbCopyListener(backups_config_logical.GetBackupConfigService())
})
