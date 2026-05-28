package backuping_physical

import (
	"context"
	"sync/atomic"
	"testing"
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

// CreateTestPhysicalBackuper returns a fully wired PhysicalBackuperNode for
// tests. The notification sender is parameterized so tests that don't want
// to exercise the notifier stack can inject a counting / no-op stub.
// Pass nil to use the production notifier service.
func CreateTestPhysicalBackuper(notificationSender NotificationSender) *PhysicalBackuperNode {
	sender := notificationSender
	if sender == nil {
		sender = notifiers.GetNotifierService()
	}

	return &PhysicalBackuperNode{
		databases.GetDatabaseService(),
		encryption.GetFieldEncryptor(),
		workspaces_services.GetWorkspaceService(),
		physical_repositories.GetFullBackupRepository(),
		physical_repositories.GetIncrementalBackupRepository(),
		physical_repositories.GetInFlightBackupRepository(),
		physical_repositories.GetWalHistoryRepository(),
		backups_config_physical.GetBackupConfigService(),
		storages.GetStorageService(),
		sender,
		tasks_cancellation.GetTaskCancelManager(),
		backuping_logical.GetBackupNodesRegistry(),
		encryption_secrets.GetSecretKeyService(),
		logger.GetLogger(),
		postgresql_executor.NewFullExecutor(),
		postgresql_executor.NewIncrementalExecutor(),
		uuid.New(),
		time.Time{},
		atomic.Bool{},
	}
}

// StartPhysicalBackuperForTest starts the backuper's Run loop in a goroutine.
// Returns a cancel func the caller defers; the cancel waits for the
// goroutine to drain before returning so tests don't leak the subscription.
func StartPhysicalBackuperForTest(
	t *testing.T,
	backuper *PhysicalBackuperNode,
) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		backuper.Run(ctx)
		close(done)
	}()

	deadline := time.Now().UTC().Add(5 * time.Second)
	for time.Now().UTC().Before(deadline) {
		if backuper.IsRunning() {
			return func() {
				cancel()

				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Log("physical backuper stop timeout")
				}
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("physical backuper failed to start within timeout")

	return nil
}

// StopPhysicalBackuperForTest signals the backuper's Run loop to exit and
// waits for it to drain. The cancel returned from StartPhysicalBackuperForTest
// already does this; StopPhysicalBackuperForTest is the matching named
// wrapper to keep test code symmetric.
func StopPhysicalBackuperForTest(_ *testing.T, cancel context.CancelFunc) {
	cancel()
}
