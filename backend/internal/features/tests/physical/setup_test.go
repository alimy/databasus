package tests_physical

import (
	"os"
	"testing"

	backuping_logical "databasus-backend/internal/features/backups/backups/backuping/logical"
	cache_utils "databasus-backend/internal/util/cache"
)

func TestMain(m *testing.M) {
	cache_utils.ClearAllCache()

	logicalBackuperNode := backuping_logical.CreateTestBackuperNode()
	cancelLogical := backuping_logical.StartBackuperNodeForTest(&testing.T{}, logicalBackuperNode)

	exitCode := m.Run()

	backuping_logical.StopBackuperNodeForTest(&testing.T{}, cancelLogical, logicalBackuperNode)

	os.Exit(exitCode)
}
