package chain_view_test

import (
	"os"
	"testing"

	cache_utils "databasus-backend/internal/util/cache"
)

func TestMain(m *testing.M) {
	cache_utils.ClearAllCache()

	exitCode := m.Run()

	os.Exit(exitCode)
}
