package tests_physical

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"databasus-backend/internal/config"
	backuping_physical "databasus-backend/internal/features/backups/backups/backuping/physical"
	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/logger"
)

const (
	restoreContainerName = "test-physical-postgres-17-restore-target"
	restoreWorkDir       = "/restore"
	restoredPgUser       = "testuser"
	restoredPgPassword   = "testpassword"
	restoredPgDatabase   = "testdb"
)

func Test_RestorePhysicalBackup_FullAndIncrementalViaPgCombinebackup_AllDataPresent(t *testing.T) {
	_, lookErr := exec.LookPath("docker")
	require.NoError(t, lookErr, "docker CLI must be on PATH")

	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)
	encryptor := encryption.GetFieldEncryptor()

	cleanupRestoreContainer(t)
	t.Cleanup(func() { cleanupRestoreContainer(t) })

	dockerExec(t, "mkdir", "-p", restoreWorkDir+"/full", restoreWorkDir+"/incr")

	sourceConn := postgresql_executor.OpenAdminConn(t, fixture)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	_, err := sourceConn.Exec(ctx, `DROP TABLE IF EXISTS restore_marker`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sourceConn.Exec(context.Background(), `DROP TABLE IF EXISTS restore_marker`)
	})

	_, err = sourceConn.Exec(ctx,
		`CREATE TABLE restore_marker (phase TEXT PRIMARY KEY, payload TEXT NOT NULL)`)
	require.NoError(t, err)

	_, err = sourceConn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ('before-full', $1)`,
		"row-inserted-before-full-backup")
	require.NoError(t, err)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	fullRow, err := physical_repositories.GetFullBackupRepository().FindByID(fixture.BackupID)
	require.NoError(t, err)
	require.NotNil(t, fullRow)
	require.NotNil(t, fullRow.FileName)
	fullFileName := *fullRow.FileName

	fullHostPath := streamArtifactToHost(t, fixture, encryptor, fullFileName, "full.tar.zst")
	extractArtifactInsideContainer(t, fullHostPath, restoreWorkDir+"/full.tar.zst", restoreWorkDir+"/full")

	// pg_basebackup --incremental needs the parent's backup_manifest as an
	// on-disk file. Production never persists a `.manifest` sidecar today —
	// the manifest is only emitted inline as the final tar entry (because
	// --no-manifest is not passed). Read it out of the just-extracted FULL
	// tree and upload to the path incremental.go:downloadParentManifest
	// expects.
	uploadParentManifestSidecar(t, fixture, encryptor, fullFileName)

	_, err = sourceConn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ('after-full', $1)`,
		"row-inserted-between-full-and-incr")
	require.NoError(t, err)

	// pg_basebackup --incremental needs WAL summaries covering FULL.stop_lsn.
	// Generate enough WAL to cross at least one segment boundary, then
	// CHECKPOINT + pg_switch_wal so the summarizer flushes a summary file
	// past stop_lsn (the summarizer never summarizes the active segment, so
	// the segment containing stop_lsn has to be closed first).
	_, err = postgresql_executor.GenerateWalActivity(ctx, sourceConn, 32*1024*1024)
	require.NoError(t, err)

	_, err = sourceConn.Exec(ctx, "CHECKPOINT")
	require.NoError(t, err)

	_, err = sourceConn.Exec(ctx, "SELECT pg_switch_wal()")
	require.NoError(t, err)

	require.NotNil(t, fullRow.StopLSN, "FULL backup must have stop_lsn")
	require.NoError(t, postgresql_executor.WaitForWalSummaries(ctx, sourceConn, *fullRow.StopLSN, 2*time.Minute))

	incrID := buildAndClaimIncrementalBackup(t, fixture)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(incrID, false)
	postgresql_executor.WaitForBackupStatus(t, incrID, physical_enums.PhysicalBackupTypeIncremental,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	incrRow, err := physical_repositories.GetIncrementalBackupRepository().FindByID(incrID)
	require.NoError(t, err)
	require.NotNil(t, incrRow)
	require.NotNil(t, incrRow.FileName)
	incrFileName := *incrRow.FileName

	incrHostPath := streamArtifactToHost(t, fixture, encryptor, incrFileName, "incr.tar.zst")
	extractArtifactInsideContainer(t, incrHostPath, restoreWorkDir+"/incr.tar.zst", restoreWorkDir+"/incr")

	combineAndStartCluster(t)

	port, err := strconv.Atoi(config.GetEnv().TestPhysicalPostgres17RestoreTargetPort)
	require.NoError(t, err)

	restoredPhases := queryRestoredMarkerRows(t, port)

	assert.ElementsMatch(t,
		[]string{"before-full", "after-full"},
		restoredPhases,
		"restored cluster must contain both pre-FULL and between-FULL-and-INCR rows")
}

func uploadParentManifestSidecar(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	encryptor encryption.FieldEncryptor,
	fullFileName string,
) {
	t.Helper()

	manifestBytes := dockerExec(t, "cat", restoreWorkDir+"/full/backup_manifest")
	require.NotEmpty(t, manifestBytes,
		"backup_manifest must be present inside the FULL tar (pg_basebackup --no-manifest is not set)")

	require.NoError(t, os.WriteFile("C:/tmp/pginc/manifest_from_test.bin", manifestBytes, 0o644))
	t.Logf("test parent manifest: %d bytes, starts with %q, ends with %q",
		len(manifestBytes),
		string(manifestBytes[:min(80, len(manifestBytes))]),
		string(manifestBytes[max(0, len(manifestBytes)-80):]))

	require.NoError(t, fixture.Storage.SaveFile(
		t.Context(),
		encryptor,
		logger.GetLogger(),
		fullFileName+".manifest",
		bytes.NewReader(manifestBytes),
	))
}

func buildAndClaimIncrementalBackup(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
) uuid.UUID {
	t.Helper()

	incrID := uuid.New()
	incrRow := &physical_models.PhysicalIncrementalBackup{
		ID:               incrID,
		DatabaseID:       fixture.DB.ID,
		StorageID:        fixture.Storage.ID,
		RootFullBackupID: fixture.BackupID,
		TimelineID:       1,
		Status:           physical_enums.PhysicalBackupStatusInProgress,
		Encryption:       physical_enums.PhysicalBackupEncryptionNone,
		CreatedAt:        time.Now().UTC(),
	}

	require.NoError(t, physical_repositories.GetIncrementalBackupRepository().Save(incrRow))
	t.Cleanup(func() {
		_ = physical_repositories.GetIncrementalBackupRepository().DeleteByID(incrID)
	})

	claimed, err := physical_repositories.GetInFlightBackupRepository().Claim(
		storage.GetDb(),
		fixture.DB.ID,
		physical_enums.PhysicalBackupTypeIncremental,
		incrID,
	)
	require.NoError(t, err)
	require.True(t, claimed, "INCR in-flight claim must succeed after FULL released the slot")
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)
	})

	return incrID
}

func streamArtifactToHost(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	encryptor encryption.FieldEncryptor,
	storageFileName string,
	localName string,
) string {
	t.Helper()

	reader, err := fixture.Storage.GetFile(encryptor, storageFileName)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	hostPath := filepath.Join(t.TempDir(), localName)

	out, err := os.Create(hostPath)
	require.NoError(t, err)

	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	require.NoError(t, copyErr)
	require.NoError(t, closeErr)

	return hostPath
}

func extractArtifactInsideContainer(t *testing.T, hostPath, containerArtifactPath, containerExtractDir string) {
	t.Helper()

	dockerCp(t, hostPath, restoreContainerName+":"+containerArtifactPath)
	dockerExec(t, "sh", "-c",
		fmt.Sprintf("zstd -d < %s | tar -xC %s", containerArtifactPath, containerExtractDir))
}

func combineAndStartCluster(t *testing.T) {
	t.Helper()

	dockerExec(t, "pg_combinebackup",
		"-o", restoreWorkDir+"/combined",
		restoreWorkDir+"/full", restoreWorkDir+"/incr")

	dockerExec(t, "sh", "-c",
		"chown -R postgres:postgres "+restoreWorkDir+"/combined && chmod 0700 "+restoreWorkDir+"/combined && "+
			"touch "+restoreWorkDir+"/pg.log && chown postgres:postgres "+restoreWorkDir+"/pg.log")

	dockerExecAs(t, "postgres",
		"pg_ctl", "-D", restoreWorkDir+"/combined", "-l", restoreWorkDir+"/pg.log", "-w", "start")

	t.Cleanup(func() {
		_ = exec.Command("docker", "exec", "--user", "postgres", restoreContainerName,
			"pg_ctl", "-D", restoreWorkDir+"/combined", "-m", "immediate", "stop").Run()
	})
}

func queryRestoredMarkerRows(t *testing.T, hostPort int) []string {
	t.Helper()

	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=%s password=%s dbname=%s sslmode=disable",
		hostPort, restoredPgUser, restoredPgPassword, restoredPgDatabase)

	conn := connectWithRetry(t, dsn, 30*time.Second)
	defer func() { _ = conn.Close(t.Context()) }()

	rows, err := conn.Query(t.Context(), `SELECT phase FROM restore_marker ORDER BY phase`)
	require.NoError(t, err)
	defer rows.Close()

	var phases []string
	for rows.Next() {
		var phase string
		require.NoError(t, rows.Scan(&phase))
		phases = append(phases, phase)
	}
	require.NoError(t, rows.Err())

	return phases
}

func connectWithRetry(t *testing.T, dsn string, timeout time.Duration) *pgx.Conn {
	t.Helper()

	deadline := time.Now().UTC().Add(timeout)
	var lastErr error

	for time.Now().UTC().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)

		conn, err := pgx.Connect(ctx, dsn)
		cancel()

		if err == nil {
			return conn
		}

		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("could not connect to restored PG within %s: %v", timeout, lastErr)

	return nil
}

func dockerExec(t *testing.T, args ...string) []byte {
	t.Helper()

	cmd := exec.Command("docker", append([]string{"exec", restoreContainerName}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec %v failed: %v\noutput: %s", args, err, out)
	}

	return out
}

func dockerExecAs(t *testing.T, user string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command("docker",
		append([]string{"exec", "--user", user, restoreContainerName}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec --user %s %v failed: %v\noutput: %s", user, args, err, out)
	}

	return out
}

func dockerCp(t *testing.T, src, dst string) {
	t.Helper()

	cmd := exec.Command("docker", "cp", src, dst)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker cp %s -> %s failed: %v\noutput: %s", src, dst, err, out)
	}
}

// cleanupRestoreContainer wipes /restore inside the persistent target
// container so a previous run's leftovers don't poison this one (and so
// the next run starts clean). pg_ctl stop here is best-effort because
// the cluster may not be running when called pre-test.
func cleanupRestoreContainer(_ *testing.T) {
	_ = exec.Command("docker", "exec", "--user", "postgres", restoreContainerName,
		"pg_ctl", "-D", restoreWorkDir+"/combined", "-m", "immediate", "stop").Run()

	_ = exec.Command("docker", "exec", restoreContainerName,
		"sh", "-c", "rm -rf "+restoreWorkDir+"/*").Run()
}
