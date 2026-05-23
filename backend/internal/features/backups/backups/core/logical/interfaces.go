package backups_core_logical

import (
	"context"

	"github.com/google/uuid"

	usecases_logical_dto "databasus-backend/internal/features/backups/backups/usecases/logical/dto"
	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
)

type NotificationSender interface {
	SendNotification(
		notifier *notifiers.Notifier,
		title string,
		message string,
	)
}

type CreateBackupUsecase interface {
	Execute(
		ctx context.Context,
		backup *LogicalBackup,
		backupConfig *backups_config_logical.LogicalBackupConfig,
		database *databases.Database,
		storage *storages.Storage,
		backupProgressListener func(completedMBs float64),
	) (*usecases_logical_dto.BackupMetadata, error)
}

type BackupRemoveListener interface {
	OnBeforeBackupRemove(backup *LogicalBackup) error
}

type BackupCompletionListener interface {
	OnBackupCompleted(backupID uuid.UUID)
}
