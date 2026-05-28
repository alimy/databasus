package backuping_physical_postgresql_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backuping_physical "databasus-backend/internal/features/backups/backups/backuping/physical"
	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	"databasus-backend/internal/util/encryption"
)

func Test_FullOnly_ProducesArtifactAndManifest(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	finalRow, err := physical_repositories.GetFullBackupRepository().FindByID(fixture.BackupID)
	require.NoError(t, err)
	require.NotNil(t, finalRow)

	require.NotNil(t, finalRow.FileName, "FileName must be populated post-COMPLETED")
	assert.True(t, strings.HasSuffix(*finalRow.FileName, ".tar.zst"),
		"FileName should end with .tar.zst, got %q", *finalRow.FileName)

	require.NotNil(t, finalRow.StartLSN, "StartLSN must be populated")
	require.NotNil(t, finalRow.StopLSN, "StopLSN must be populated")
	require.NotNil(t, finalRow.BackupSizeMb, "BackupSizeMb must be populated")
	require.NotNil(t, finalRow.CompletedAt, "CompletedAt must be populated")

	assert.GreaterOrEqual(t, *finalRow.BackupSizeMb, float64(0))

	encryptor := encryption.GetFieldEncryptor()

	artifactReader, err := fixture.Storage.GetFile(encryptor, *finalRow.FileName)
	require.NoError(t, err, "artifact %q must be in storage", *finalRow.FileName)
	require.NoError(t, artifactReader.Close())

	sidecarReader, err := fixture.Storage.GetFile(encryptor, *finalRow.FileName+".metadata")
	require.NoError(t, err, "sidecar must be in storage")
	require.NoError(t, sidecarReader.Close())
}
