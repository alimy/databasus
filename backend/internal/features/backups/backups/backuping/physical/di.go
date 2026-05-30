package backuping_physical

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	backuping_logical "databasus-backend/internal/features/backups/backups/backuping/logical"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	postgresql_executor "databasus-backend/internal/features/backups/backups/usecases/physical/postgresql"
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
	postgresql_executor.NewCreateFullBackupUsecase(),
	postgresql_executor.NewCreateIncrementalBackupUsecase(),
	getNodeID(),
	time.Time{},
	atomic.Bool{},
}

func GetPhysicalBackuperNode() *PhysicalBackuperNode { return physicalBackuperNode }

var physicalSlotCleanupListener = postgresql_executor.NewPhysicalSlotCleanupListener(
	databases.GetDatabaseService(),
	encryption.GetFieldEncryptor(),
	logger.GetLogger(),
)

var SetupDependencies = sync.OnceFunc(func() {
	databases.GetDatabaseService().AddDbRemoveListener(physicalSlotCleanupListener)
})
