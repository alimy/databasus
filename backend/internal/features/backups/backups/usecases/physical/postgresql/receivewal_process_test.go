package usecases_physical_postgresql

import (
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	postgresql_shared "databasus-backend/internal/features/databases/databases/postgresql/shared"
)

func Test_NewReceivewalCommand_SetsParentDeathSignalAndApplicationName(t *testing.T) {
	databaseID := uuid.New()
	sourceDB := &postgresql_physical.PostgresqlPhysicalDatabase{
		DatabaseID:          &databaseID,
		Host:                "localhost",
		Port:                5432,
		Username:            "replicator",
		ReplicationSlotName: "slot",
	}

	cmd, err := newReceivewalCommand(
		t.Context(),
		"sh",
		sourceDB,
		&postgresql_shared.CredentialTempFiles{PgpassPath: "/tmp/pgpass"},
		t.TempDir(),
		"slot",
	)
	require.NoError(t, err)
	require.NotNil(t, cmd.SysProcAttr)
	require.Equal(t, syscall.SIGTERM, cmd.SysProcAttr.Pdeathsig)

	applicationName := "PGAPPNAME=" + receivewalApplicationNamePrefix + databaseID.String()
	require.True(t, strings.Contains(strings.Join(cmd.Env, "\n"), applicationName))
}
