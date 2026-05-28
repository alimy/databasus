package backuping_physical

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	backuping_logical "databasus-backend/internal/features/backups/backups/backuping/logical"
	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	"databasus-backend/internal/features/databases"
	encryption_secrets "databasus-backend/internal/features/encryption/secrets"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	tasks_cancellation "databasus-backend/internal/features/tasks/cancellation"
	workspaces_services "databasus-backend/internal/features/workspaces/services"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/logger"
)

func getNodeID() uuid.UUID {
	return uuid.New()
}

var physicalBackuperNode = &PhysicalBackuperNode{
	databases.GetDatabaseService(),
	encryption.GetFieldEncryptor(),
	workspaces_services.GetWorkspaceService(),
	physical_repositories.GetFullBackupRepository(),
	physical_repositories.GetIncrementalBackupRepository(),
	physical_repositories.GetInFlightBackupRepository(),
	physical_repositories.GetWalHistoryRepository(),
	backups_config_physical.GetBackupConfigService(),
	storages.GetStorageService(),
	notifiers.GetNotifierService(),
	tasks_cancellation.GetTaskCancelManager(),
	backuping_logical.GetBackupNodesRegistry(),
	encryption_secrets.GetSecretKeyService(),
	logger.GetLogger(),
	postgresql_executor.NewFullExecutor(),
	postgresql_executor.NewIncrementalExecutor(),
	getNodeID(),
	time.Time{},
	atomic.Bool{},
}

func GetPhysicalBackuperNode() *PhysicalBackuperNode { return physicalBackuperNode }
