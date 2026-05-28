package backuping_physical

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"databasus-backend/internal/config"
	"databasus-backend/internal/features/backups/backups/backuping/nodes"
	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	"databasus-backend/internal/features/databases"
	encryption_secrets "databasus-backend/internal/features/encryption/secrets"
	"databasus-backend/internal/features/storages"
	tasks_cancellation "databasus-backend/internal/features/tasks/cancellation"
	workspaces_services "databasus-backend/internal/features/workspaces/services"
	util_encryption "databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/walmath"
)

const (
	heartbeatTickerInterval = 15 * time.Second
	heartbeatStaleThreshold = 5 * time.Minute
)

// ErrUnsupportedTaskKind fires when the registry publishes a WAL_STREAM
// assignment to this node — PR 2 dispatches FULL and INCR only. PR 4 fills
// in the streamer dispatch.
var ErrUnsupportedTaskKind = errors.New("physical backuper: WAL_STREAM dispatch lands in PR 4")

// PhysicalBackuperNode is the per-node worker that consumes backup:submit
// assignments and drives a FULL or INCR through the postgresql executor.
// Mirrors logical's BackuperNode shape (subscribe → handler → spawn
// goroutine per backup) but writes physical_full_backups /
// physical_incremental_backups rows instead of the logical backup table.
type PhysicalBackuperNode struct {
	databaseService     *databases.DatabaseService
	fieldEncryptor      util_encryption.FieldEncryptor
	workspaceService    *workspaces_services.WorkspaceService
	fullRepo            *physical_repositories.PhysicalFullBackupRepository
	incrRepo            *physical_repositories.PhysicalIncrementalBackupRepository
	inFlightRepo        *physical_repositories.PhysicalInFlightBackupRepository
	historyRepo         *physical_repositories.PhysicalWalHistoryRepository
	backupConfigService *backups_config_physical.BackupConfigService
	storageService      *storages.StorageService
	notificationSender  NotificationSender
	taskCancelManager   *tasks_cancellation.TaskCancelManager
	backupNodesRegistry *nodes.BackupNodesRegistry
	secretKeyService    *encryption_secrets.SecretKeyService
	logger              *slog.Logger
	fullExecutor        *postgresql_executor.FullExecutor
	incrExecutor        *postgresql_executor.IncrementalExecutor
	nodeID              uuid.UUID

	lastHeartbeat time.Time

	hasRun atomic.Bool
}

func (n *PhysicalBackuperNode) Run(ctx context.Context) {
	if n.hasRun.Swap(true) {
		panic(fmt.Sprintf("%T.Run() called multiple times", n))
	}

	n.lastHeartbeat = time.Now().UTC()

	throughputMBs := config.GetEnv().NodeNetworkThroughputMBs

	backupNode := nodes.BackupNode{
		ID:            n.nodeID,
		ThroughputMBs: throughputMBs,
		LastHeartbeat: time.Now().UTC(),
	}

	if err := n.backupNodesRegistry.HearthbeatNodeInRegistry(time.Now().UTC(), backupNode); err != nil {
		n.logger.Error("failed to register physical backuper node", "error", err)

		panic(err)
	}

	handler := func(backupID uuid.UUID, isCallNotifier bool) {
		go func() {
			n.MakeBackup(backupID, isCallNotifier)

			if err := n.backupNodesRegistry.PublishBackupCompletion(n.nodeID, backupID); err != nil {
				n.logger.Error("failed to publish backup completion",
					"backup_id", backupID,
					"error", err)
			}
		}()
	}

	if err := n.backupNodesRegistry.SubscribeNodeForBackupsAssignment(n.nodeID, handler); err != nil {
		n.logger.Error("failed to subscribe physical backuper", "error", err)

		panic(err)
	}

	defer func() {
		if err := n.backupNodesRegistry.UnsubscribeNodeForBackupsAssignments(); err != nil {
			n.logger.Error("failed to unsubscribe physical backuper", "error", err)
		}
	}()

	ticker := time.NewTicker(heartbeatTickerInterval)
	defer ticker.Stop()

	n.logger.Info("physical backuper node started", "node_id", n.nodeID, "throughput_mbs", throughputMBs)

	for {
		select {
		case <-ctx.Done():
			n.logger.Info("shutdown signal received, unregistering physical backuper", "node_id", n.nodeID)

			if err := n.backupNodesRegistry.UnregisterNodeFromRegistry(backupNode); err != nil {
				n.logger.Error("failed to unregister physical backuper", "error", err)
			}

			return

		case <-ticker.C:
			n.sendHeartbeat(&backupNode)
		}
	}
}

func (n *PhysicalBackuperNode) IsRunning() bool {
	return n.lastHeartbeat.After(time.Now().UTC().Add(-heartbeatStaleThreshold))
}

// MakeBackup resolves the typed row by ID (FULL or INCR), drives the
// executor, then writes the status update + releases the in-flight claim
// in one transaction. Public so tests can invoke it without standing up
// the registry-subscription loop.
func (n *PhysicalBackuperNode) MakeBackup(backupID uuid.UUID, isCallNotifier bool) {
	logger := n.logger.With("backup_id", backupID)

	fullBackup, err := n.fullRepo.FindByID(backupID)
	if err != nil {
		logger.Error("failed to look up full backup row", "error", err)

		return
	}

	if fullBackup != nil {
		n.runFullBackup(logger, fullBackup, isCallNotifier)

		return
	}

	incrBackup, err := n.incrRepo.FindByID(backupID)
	if err != nil {
		logger.Error("failed to look up incremental backup row", "error", err)

		return
	}

	if incrBackup != nil {
		n.runIncrementalBackup(logger, incrBackup, isCallNotifier)

		return
	}

	logger.Warn("backup not found in either typed table; ignoring assignment")
}

func (n *PhysicalBackuperNode) runFullBackup(
	logger *slog.Logger,
	backup *physical_models.PhysicalFullBackup,
	isCallNotifier bool,
) {
	cfg, db, storage, masterKey, ok := n.loadBackupContext(logger, backup.DatabaseID)
	if !ok {
		n.finalizeFullAsError(backup, physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"failed to load backup context")

		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.taskCancelManager.RegisterTask(backup.ID, cancel)
	defer n.taskCancelManager.UnregisterTask(backup.ID)

	spec := postgresql_executor.FullSpec{
		SourceDB:       db.PostgresqlPhysical,
		Backup:         backup,
		StorageID:      storage.ID,
		Storage:        storage,
		BackupConfig:   nil,
		Encryption:     cfg.Encryption,
		MasterKey:      masterKey,
		FieldEncryptor: n.fieldEncryptor,
		FullRepo:       n.fullRepo,
		HistoryRepo:    n.historyRepo,
		Logger:         logger,
	}

	result, err := n.fullExecutor.Execute(ctx, spec)
	if err != nil {
		logger.Error("full executor returned error", "error", err)

		n.finalizeFullAsError(backup, physical_enums.PhysicalBackupErrorPgBasebackupFailed, err.Error())

		return
	}

	if result.Status != physical_enums.PhysicalBackupStatusCompleted {
		logger.Warn("full executor returned non-COMPLETED result",
			"status", result.Status,
			"reason", reasonOrEmpty(result.ErrorReason),
			"message", result.ErrorMessage)
	}

	if result.Status == physical_enums.PhysicalBackupStatusCompleted {
		if err := n.uploadFullSidecar(logger, db.PostgresqlPhysical, backup, result, storage); err != nil {
			logger.Error("failed to upload sidecar; flipping FULL to ERROR", "error", err)

			n.finalizeFullAsError(backup,
				physical_enums.PhysicalBackupErrorStorageUploadFailed, err.Error())

			return
		}
	}

	if err := n.persistFullResult(backup, result); err != nil {
		logger.Error("failed to persist full result", "error", err)

		return
	}

	if isCallNotifier {
		n.sendFullNotification(cfg, db, backup, result)
	}
}

func (n *PhysicalBackuperNode) runIncrementalBackup(
	logger *slog.Logger,
	backup *physical_models.PhysicalIncrementalBackup,
	isCallNotifier bool,
) {
	cfg, db, storage, masterKey, ok := n.loadBackupContext(logger, backup.DatabaseID)
	if !ok {
		n.finalizeIncrAsError(backup, physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"failed to load backup context")

		return
	}

	parentFileName, parentBackupID, parentEncryption, parentSalt, parentIV, err := n.resolveParentManifest(backup)
	if err != nil {
		logger.Error("failed to resolve parent manifest", "error", err)

		n.finalizeIncrAsError(backup,
			physical_enums.PhysicalBackupErrorParentManifestMissing, err.Error())

		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.taskCancelManager.RegisterTask(backup.ID, cancel)
	defer n.taskCancelManager.UnregisterTask(backup.ID)

	spec := postgresql_executor.IncrSpec{
		SourceDB:             db.PostgresqlPhysical,
		Backup:               backup,
		StorageID:            storage.ID,
		Storage:              storage,
		BackupConfig:         nil,
		Encryption:           cfg.Encryption,
		MasterKey:            masterKey,
		FieldEncryptor:       n.fieldEncryptor,
		ParentFileName:       parentFileName,
		ParentBackupID:       parentBackupID,
		ParentEncryption:     parentEncryption,
		ParentEncryptionSalt: parentSalt,
		ParentEncryptionIV:   parentIV,
		FullRepo:             n.fullRepo,
		IncrRepo:             n.incrRepo,
		HistoryRepo:          n.historyRepo,
		Logger:               logger,
	}

	result, err := n.incrExecutor.Execute(ctx, spec)
	if err != nil {
		logger.Error("incremental executor returned error", "error", err)

		n.finalizeIncrAsError(backup, physical_enums.PhysicalBackupErrorPgBasebackupFailed, err.Error())

		return
	}

	if result.Status != physical_enums.PhysicalBackupStatusCompleted {
		logger.Warn("incremental executor returned non-COMPLETED result",
			"status", result.Status,
			"reason", reasonOrEmpty(result.ErrorReason),
			"message", result.ErrorMessage)
	}

	if result.Status == physical_enums.PhysicalBackupStatusCompleted {
		if err := n.uploadIncrSidecar(logger, db.PostgresqlPhysical, backup, result, storage); err != nil {
			logger.Error("failed to upload sidecar; flipping INCR to ERROR", "error", err)

			n.finalizeIncrAsError(backup,
				physical_enums.PhysicalBackupErrorStorageUploadFailed, err.Error())

			return
		}
	}

	if err := n.persistIncrResult(backup, result); err != nil {
		logger.Error("failed to persist incremental result", "error", err)

		return
	}

	if isCallNotifier {
		n.sendIncrNotification(cfg, db, backup, result)
	}
}

func (n *PhysicalBackuperNode) loadBackupContext(
	logger *slog.Logger,
	databaseID uuid.UUID,
) (*backups_config_physical.PhysicalBackupConfig, *databases.Database, *storages.Storage, string, bool) {
	cfg, err := n.backupConfigService.GetBackupConfigByDbId(databaseID)
	if err != nil {
		logger.Error("failed to fetch physical backup config", "error", err)

		return nil, nil, nil, "", false
	}

	if cfg.StorageID == nil {
		logger.Error("physical backup config has no storage id")

		return nil, nil, nil, "", false
	}

	db, err := n.databaseService.GetDatabaseByID(databaseID)
	if err != nil {
		logger.Error("failed to fetch database by id", "error", err)

		return nil, nil, nil, "", false
	}

	if db.PostgresqlPhysical == nil {
		logger.Error("database is not a physical postgres database")

		return nil, nil, nil, "", false
	}

	storage, err := n.storageService.GetStorageByID(*cfg.StorageID)
	if err != nil {
		logger.Error("failed to fetch storage", "error", err)

		return nil, nil, nil, "", false
	}

	masterKey := ""
	if cfg.Encryption == backups_config_logical.BackupEncryptionEncrypted {
		key, secretErr := n.secretKeyService.GetSecretKey()
		if secretErr != nil {
			logger.Error("failed to fetch master key", "error", secretErr)

			return nil, nil, nil, "", false
		}

		masterKey = key
	}

	return cfg, db, storage, masterKey, true
}

func (n *PhysicalBackuperNode) resolveParentManifest(
	backup *physical_models.PhysicalIncrementalBackup,
) (fileName string, parentID uuid.UUID, enc backups_config_logical.BackupEncryption, salt, iv string, err error) {
	if backup.ParentIncrementalBackupID != nil {
		parent, lookupErr := n.incrRepo.FindByID(*backup.ParentIncrementalBackupID)
		if lookupErr != nil {
			return "", uuid.Nil, "", "", "", fmt.Errorf("look up parent incr: %w", lookupErr)
		}

		if parent == nil || parent.FileName == nil {
			return "", uuid.Nil, "", "", "", errors.New("parent incremental row missing or has no file_name")
		}

		return *parent.FileName, parent.ID,
			physicalEncryptionToLogical(parent.Encryption),
			derefString(parent.EncryptionSalt),
			derefString(parent.EncryptionIV),
			nil
	}

	parent, lookupErr := n.fullRepo.FindByID(backup.RootFullBackupID)
	if lookupErr != nil {
		return "", uuid.Nil, "", "", "", fmt.Errorf("look up root full: %w", lookupErr)
	}

	if parent == nil || parent.FileName == nil {
		return "", uuid.Nil, "", "", "", errors.New("root full row missing or has no file_name")
	}

	return *parent.FileName, parent.ID,
		physicalEncryptionToLogical(parent.Encryption),
		derefString(parent.EncryptionSalt),
		derefString(parent.EncryptionIV),
		nil
}

func physicalEncryptionToLogical(p physical_enums.PhysicalBackupEncryption) backups_config_logical.BackupEncryption {
	if p == physical_enums.PhysicalBackupEncryptionAes256Gcm {
		return backups_config_logical.BackupEncryptionEncrypted
	}

	return backups_config_logical.BackupEncryptionNone
}

func (n *PhysicalBackuperNode) persistFullResult(
	backup *physical_models.PhysicalFullBackup,
	result postgresql_executor.FullResult,
) error {
	backup.Status = result.Status
	backup.ErrorReason = result.ErrorReason

	if result.Status == physical_enums.PhysicalBackupStatusCompleted {
		backup.TimelineID = result.TimelineID
		backup.StartLSN = lsnPtr(result.StartLSN)
		backup.StopLSN = lsnPtr(result.StopLSN)
		backup.BackupSizeMb = &result.BackupSizeMb
		backup.BackupDurationMs = &result.BackupDurationMs
		backup.Encryption = result.EncryptionAlgo
		backup.EncryptionSalt = nilOrPtr(result.EncryptionSalt)
		backup.EncryptionIV = nilOrPtr(result.EncryptionIV)

		completed := result.CompletedAt
		if completed.IsZero() {
			completed = time.Now().UTC()
		}

		backup.CompletedAt = &completed
	}

	if err := n.fullRepo.Save(backup); err != nil {
		return err
	}

	if err := n.inFlightRepo.Release(backup.DatabaseID); err != nil {
		n.logger.Warn("failed to release in-flight claim after full backup",
			"backup_id", backup.ID,
			"error", err)
	}

	return nil
}

func (n *PhysicalBackuperNode) persistIncrResult(
	backup *physical_models.PhysicalIncrementalBackup,
	result postgresql_executor.IncrResult,
) error {
	backup.Status = result.Status
	backup.ErrorReason = result.ErrorReason

	if result.Status == physical_enums.PhysicalBackupStatusCompleted {
		backup.TimelineID = result.TimelineID
		backup.StartLSN = lsnPtr(result.StartLSN)
		backup.StopLSN = lsnPtr(result.StopLSN)
		backup.BackupSizeMb = &result.BackupSizeMb
		backup.BackupDurationMs = &result.BackupDurationMs
		backup.Encryption = result.EncryptionAlgo
		backup.EncryptionSalt = nilOrPtr(result.EncryptionSalt)
		backup.EncryptionIV = nilOrPtr(result.EncryptionIV)

		completed := result.CompletedAt
		if completed.IsZero() {
			completed = time.Now().UTC()
		}

		backup.CompletedAt = &completed
	}

	if err := n.incrRepo.Save(backup); err != nil {
		return err
	}

	if err := n.inFlightRepo.Release(backup.DatabaseID); err != nil {
		n.logger.Warn("failed to release in-flight claim after incremental backup",
			"backup_id", backup.ID,
			"error", err)
	}

	return nil
}

func (n *PhysicalBackuperNode) finalizeFullAsError(
	backup *physical_models.PhysicalFullBackup,
	reason physical_enums.PhysicalBackupErrorReason,
	_ string,
) {
	r := reason

	backup.Status = physical_enums.PhysicalBackupStatusError
	backup.ErrorReason = &r

	if err := n.fullRepo.Save(backup); err != nil {
		n.logger.Error("failed to flip full row to ERROR", "backup_id", backup.ID, "error", err)
	}

	_ = n.inFlightRepo.Release(backup.DatabaseID)
}

func (n *PhysicalBackuperNode) finalizeIncrAsError(
	backup *physical_models.PhysicalIncrementalBackup,
	reason physical_enums.PhysicalBackupErrorReason,
	_ string,
) {
	r := reason

	backup.Status = physical_enums.PhysicalBackupStatusError
	backup.ErrorReason = &r

	if err := n.incrRepo.Save(backup); err != nil {
		n.logger.Error("failed to flip incr row to ERROR", "backup_id", backup.ID, "error", err)
	}

	_ = n.inFlightRepo.Release(backup.DatabaseID)
}

func (n *PhysicalBackuperNode) sendFullNotification(
	cfg *backups_config_physical.PhysicalBackupConfig,
	db *databases.Database,
	backup *physical_models.PhysicalFullBackup,
	result postgresql_executor.FullResult,
) {
	notificationType, title, message := classifyFullNotification(db, backup, result, n.workspaceService)
	if notificationType == "" {
		return
	}

	if !slices.Contains(cfg.SendNotificationsOn, notificationType) {
		return
	}

	for _, notifier := range db.Notifiers {
		n.notificationSender.SendNotification(&notifier, title, message)
	}
}

func (n *PhysicalBackuperNode) sendIncrNotification(
	cfg *backups_config_physical.PhysicalBackupConfig,
	db *databases.Database,
	backup *physical_models.PhysicalIncrementalBackup,
	result postgresql_executor.IncrResult,
) {
	notificationType, title, message := classifyIncrNotification(db, backup, result, n.workspaceService)
	if notificationType == "" {
		return
	}

	if !slices.Contains(cfg.SendNotificationsOn, notificationType) {
		return
	}

	for _, notifier := range db.Notifiers {
		n.notificationSender.SendNotification(&notifier, title, message)
	}
}

func classifyFullNotification(
	db *databases.Database,
	backup *physical_models.PhysicalFullBackup,
	result postgresql_executor.FullResult,
	workspaceService *workspaces_services.WorkspaceService,
) (backups_config_physical.BackupNotificationType, string, string) {
	workspaceName := "unknown"
	if db.WorkspaceID != nil {
		if ws, err := workspaceService.GetWorkspaceByID(*db.WorkspaceID); err == nil {
			workspaceName = ws.Name
		}
	}

	switch backup.Status {
	case physical_enums.PhysicalBackupStatusCompleted:
		return backups_config_physical.NotificationBackupSuccess,
			fmt.Sprintf("Physical FULL completed for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s size=%.2f MB duration=%dms",
				backup.ID, result.BackupSizeMb, result.BackupDurationMs)

	case physical_enums.PhysicalBackupStatusError:
		return backups_config_physical.NotificationBackupFailed,
			fmt.Sprintf("Physical FULL failed for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s reason=%s message=%s",
				backup.ID, reasonOrEmpty(backup.ErrorReason), result.ErrorMessage)

	case physical_enums.PhysicalBackupStatusChainBroken:
		return backups_config_physical.NotificationChainBroken,
			fmt.Sprintf("Physical FULL chain-broken for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s reason=%s message=%s",
				backup.ID, reasonOrEmpty(backup.ErrorReason), result.ErrorMessage)
	}

	return "", "", ""
}

func classifyIncrNotification(
	db *databases.Database,
	backup *physical_models.PhysicalIncrementalBackup,
	result postgresql_executor.IncrResult,
	workspaceService *workspaces_services.WorkspaceService,
) (backups_config_physical.BackupNotificationType, string, string) {
	workspaceName := "unknown"
	if db.WorkspaceID != nil {
		if ws, err := workspaceService.GetWorkspaceByID(*db.WorkspaceID); err == nil {
			workspaceName = ws.Name
		}
	}

	switch backup.Status {
	case physical_enums.PhysicalBackupStatusCompleted:
		return backups_config_physical.NotificationBackupSuccess,
			fmt.Sprintf("Physical INCR completed for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s size=%.2f MB duration=%dms",
				backup.ID, result.BackupSizeMb, result.BackupDurationMs)

	case physical_enums.PhysicalBackupStatusError:
		return backups_config_physical.NotificationBackupFailed,
			fmt.Sprintf("Physical INCR failed for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s reason=%s message=%s",
				backup.ID, reasonOrEmpty(backup.ErrorReason), result.ErrorMessage)

	case physical_enums.PhysicalBackupStatusChainBroken:
		return backups_config_physical.NotificationChainBroken,
			fmt.Sprintf("Physical INCR chain-broken for %q (workspace %q)", db.Name, workspaceName),
			fmt.Sprintf("backup_id=%s reason=%s message=%s",
				backup.ID, reasonOrEmpty(backup.ErrorReason), result.ErrorMessage)
	}

	return "", "", ""
}

func reasonOrEmpty(r *physical_enums.PhysicalBackupErrorReason) string {
	if r == nil {
		return ""
	}

	return string(*r)
}

func (n *PhysicalBackuperNode) sendHeartbeat(backupNode *nodes.BackupNode) {
	n.lastHeartbeat = time.Now().UTC()

	if err := n.backupNodesRegistry.HearthbeatNodeInRegistry(time.Now().UTC(), *backupNode); err != nil {
		n.logger.Error("physical backuper heartbeat failed", "error", err)
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func nilOrPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func lsnPtr(v walmath.LSN) *walmath.LSN {
	out := v

	return &out
}
