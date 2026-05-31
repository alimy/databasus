package backuping_physical

import (
	"log/slog"

	"github.com/google/uuid"

	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	tasks_cancellation "databasus-backend/internal/features/tasks/cancellation"
)

// PhysicalBackupCancellationListener stands physical backup work down on the two
// events that require it: a config change (backups disabled or BackupType
// demoted from WAL_STREAM) and a database removal. It implements both the
// config-change and db-remove listener seams.
type PhysicalBackupCancellationListener struct {
	inFlightRepo      *physical_repositories.PhysicalInFlightBackupRepository
	walStreamerRepo   *physical_repositories.PhysicalWalStreamerRepository
	taskCancelManager *tasks_cancellation.TaskCancelManager
	logger            *slog.Logger
}

// OnBackupConfigChanged reacts to a config transition. Disabling backups cancels
// any in-flight FULL/INCR; both disabling and demoting BackupType away from
// WAL_STREAM delete the streamer row so the scheduler cannot silently re-spawn a
// streamer the user turned off. The source-PG replication slot is intentionally
// preserved (re-enabling would reuse it).
func (l *PhysicalBackupCancellationListener) OnBackupConfigChanged(
	oldConfig, newConfig *backups_config_physical.PhysicalBackupConfig,
) {
	databaseID := newConfig.DatabaseID
	logger := l.logger.With("database_id", databaseID)

	if oldConfig.IsBackupsEnabled && !newConfig.IsBackupsEnabled {
		l.cancelInFlightBackup(logger, databaseID)
	}

	l.deleteStreamerRow(logger, databaseID)
}

// OnBeforeDatabaseRemove cancels any in-flight backup and removes the streamer
// row before the database (and its cascade-deleted catalog rows) goes away.
func (l *PhysicalBackupCancellationListener) OnBeforeDatabaseRemove(databaseID uuid.UUID) error {
	logger := l.logger.With("database_id", databaseID)

	l.cancelInFlightBackup(logger, databaseID)
	l.deleteStreamerRow(logger, databaseID)

	return nil
}

func (l *PhysicalBackupCancellationListener) cancelInFlightBackup(logger *slog.Logger, databaseID uuid.UUID) {
	claim, err := l.inFlightRepo.FindByDatabaseID(databaseID)
	if err != nil {
		logger.Error("failed to look up in-flight backup for cancel", "error", err)

		return
	}

	if claim == nil {
		return
	}

	if err := l.taskCancelManager.CancelTask(claim.BackupID); err != nil {
		logger.Error("failed to cancel in-flight backup task", "backup_id", claim.BackupID, "error", err)
	}

	if err := l.inFlightRepo.ReleaseOwned(databaseID, claim.BackupID); err != nil {
		logger.Error("failed to release in-flight claim", "error", err)
	}
}

func (l *PhysicalBackupCancellationListener) deleteStreamerRow(logger *slog.Logger, databaseID uuid.UUID) {
	if err := l.walStreamerRepo.DeleteByDatabaseID(databaseID); err != nil {
		logger.Error("failed to delete wal streamer row", "error", err)
	}
}
