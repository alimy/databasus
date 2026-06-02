package tests_physical

import (
	"os"
	"testing"

	backuping_physical "databasus-backend/internal/features/backups/backups/backuping/physical"
	cache_utils "databasus-backend/internal/util/cache"
)

// TestMain runs the single-node production wiring for the whole suite: a physical
// backuper node, the scheduler, and the WAL stream supervisor, coordinating through
// the shared physical node registry. With all three up, a backup requested over the
// HTTP API (config-enable bootstrap for the FULL, the trigger endpoint for
// incrementals) is claimed on the next 1s scheduler tick, and enabling WAL-stream
// backups makes the supervisor claim the database and create its replication slot —
// so the tests drive the real control plane instead of calling MakeBackup or
// starting a streamer directly.
func TestMain(m *testing.M) {
	cache_utils.ClearAllCache()
	backuping_physical.SetupDependencies()

	backuperNode := backuping_physical.CreateTestPhysicalBackuper(nil)
	stopBackuper := backuping_physical.StartPhysicalBackuperForTest(&testing.T{}, backuperNode)
	stopScheduler := backuping_physical.StartPhysicalSchedulerForTest(&testing.T{})
	stopWalStreamSupervisor := backuping_physical.StartPhysicalWalStreamSupervisorForTest(&testing.T{})

	exitCode := m.Run()

	stopWalStreamSupervisor()
	stopScheduler()
	backuping_physical.StopPhysicalBackuperForTest(&testing.T{}, stopBackuper)

	os.Exit(exitCode)
}
