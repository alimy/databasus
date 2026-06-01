package tests_physical

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backuping_physical "databasus-backend/internal/features/backups/backups/backuping/physical"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	postgresql_executor "databasus-backend/internal/features/backups/backups/usecases/physical/postgresql"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/walmath"
)

// walSpanUpperBound is an LSN ceiling well above any value a test produces, used
// to fetch every archived segment for a database regardless of position.
const walSpanUpperBound = walmath.LSN(1) << 62

// pitrWalArchiveDir holds the decompressed segments the recovering cluster pulls
// via restore_command. PostgreSQL refuses archive recovery (recovery.signal)
// without a restore_command even when the WAL already sits in pg_wal, so PITR
// always feeds WAL through an archive.
const pitrWalArchiveDir = restoreWorkDir + "/walarchive"

// Test_RestorePhysicalBackup_PITR_TargetTimeMidWalRange_ReplaysToTarget proves
// point-in-time recovery against streamed WAL: a FULL is taken, rows are written
// before and after a captured target timestamp, the streamer archives the
// covering WAL segments, and the FULL + segments are replayed to that target on
// a fresh cluster. The restored cluster must contain every row committed at or
// before the target and none committed after it.
func Test_RestorePhysicalBackup_PITR_TargetTimeMidWalRange_ReplaysToTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("PITR restore runs pg_receivewal + a real recovery; skipped in -short")
	}

	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)
	t.Cleanup(func() {
		_ = physical_repositories.GetWalStreamerRepository().DeleteByDatabaseID(fixture.DB.ID)
	})

	target := restoreTargetForVersion("17")
	encryptor := encryption.GetFieldEncryptor()

	cleanupRestoreContainer(target.container)
	t.Cleanup(func() { cleanupRestoreContainer(target.container) })

	// Stream WAL into the REAL storage so the post-FULL segments are archived
	// where the restore can read them back.
	stopStreamer := postgresql_executor.StartWalStreamerForTest(t, fixture, fixture.Storage)
	t.Cleanup(stopStreamer)

	sourceConn := openSourceTestDBConn(t, fixture)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	_, err := sourceConn.Exec(ctx, `DROP TABLE IF EXISTS restore_marker`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sourceConn.Exec(context.Background(), `DROP TABLE IF EXISTS restore_marker`)
	})

	_, err = sourceConn.Exec(ctx,
		`CREATE TABLE restore_marker (phase TEXT PRIMARY KEY, payload TEXT NOT NULL)`)
	require.NoError(t, err)

	// 'before-full' lands in the base backup itself.
	_, err = sourceConn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ('before-full', $1)`, "row-in-base-backup")
	require.NoError(t, err)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	fullRow, err := physical_repositories.GetFullBackupRepository().FindByID(fixture.BackupID)
	require.NoError(t, err)
	require.NotNil(t, fullRow.FileName)

	// 'before-target' is committed after the FULL but before the captured target,
	// so PITR must replay it from streamed WAL.
	_, err = sourceConn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ('before-target', $1)`, "row-replayed-up-to-target")
	require.NoError(t, err)

	// Capture the target after the before-target commit. clock_timestamp() is the
	// live wall clock, so it is strictly after the row just committed.
	var targetTimestamp string
	require.NoError(t, sourceConn.QueryRow(ctx, `SELECT clock_timestamp()::text`).Scan(&targetTimestamp))

	// Separate the after-target commit from the target so recovery has an
	// unambiguous cut point.
	time.Sleep(2 * time.Second)

	// 'after-target' is committed after the target — PITR must NOT replay it.
	_, err = sourceConn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ('after-target', $1)`, "row-after-target-must-be-absent")
	require.NoError(t, err)

	var afterTargetLSN walmath.LSN
	require.NoError(t, sourceConn.QueryRow(ctx, `SELECT pg_current_wal_lsn()::text`).Scan(&afterTargetLSN))

	// Close the segments holding before-target/after-target so the uploader
	// archives them.
	for range 4 {
		_, rotateErr := postgresql_executor.ForceWalRotation(ctx, sourceConn)
		require.NoError(t, rotateErr)
	}

	waitForArchivedWalThroughLSN(t, fixture.DB.ID, afterTargetLSN, 90*time.Second)

	pgdata := extractFullIntoDatadir(t, fixture, encryptor, target, *fullRow.FileName, fullRow.Compression)
	provisionArchivedWalSegments(t, fixture, encryptor, target)
	configurePitrRecovery(t, target, pgdata, targetTimestamp)
	startDatadirCluster(t, target, pgdata)

	port, err := strconv.Atoi(target.hostPort)
	require.NoError(t, err)

	restoredPhases := queryRestoredMarkerRows(t, port)

	assert.ElementsMatch(t,
		[]string{"before-full", "before-target"},
		restoredPhases,
		"PITR must replay rows committed at/before the target and drop the row committed after it")
}

// waitForArchivedWalThroughLSN blocks until a committed (file_name NOT NULL)
// segment covers throughLSN, i.e. the WAL needed to recover up to the target is
// durably in storage.
func waitForArchivedWalThroughLSN(t *testing.T, databaseID uuid.UUID, throughLSN walmath.LSN, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().UTC().Add(timeout)
	repo := physical_repositories.GetWalSegmentRepository()

	for time.Now().UTC().Before(deadline) {
		segments, err := repo.FindByChainSpan(databaseID, 1, walmath.LSN(0), walSpanUpperBound)
		require.NoError(t, err)

		for _, segment := range segments {
			if segment.FileName != nil && segment.EndLSN >= throughLSN {
				return
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	t.Fatalf("no archived WAL segment covered LSN %s within %s", throughLSN.String(), timeout)
}

// extractFullIntoDatadir streams the FULL artifact to the host and extracts it
// into a fresh datadir inside the restore container, returning the datadir path.
func extractFullIntoDatadir(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	encryptor encryption.FieldEncryptor,
	target restoreTarget,
	fullFileName string,
	codec physical_enums.PhysicalBackupCompression,
) string {
	t.Helper()

	decompress, suffix := decompressorFor(codec)

	hostArtifact := streamArtifactToHost(t, fixture, encryptor, fullFileName, "pitr-full"+suffix)

	pgdata := restoreWorkDir + "/pgdata"
	containerArtifact := pgdata + suffix

	dockerExec(t, target.container, "mkdir", "-p", pgdata)
	dockerCp(t, hostArtifact, target.container+":"+containerArtifact)
	dockerExec(t, target.container, "sh", "-c",
		fmt.Sprintf("%s < %s | tar -xC %s", decompress, containerArtifact, pgdata))

	return pgdata
}

// provisionArchivedWalSegments downloads every committed WAL segment,
// decompresses it (segments are stored zstd-compressed), and lands it in the
// PITR archive dir under its real WAL filename so the cluster's restore_command
// can fetch it during recovery.
func provisionArchivedWalSegments(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	encryptor encryption.FieldEncryptor,
	target restoreTarget,
) {
	t.Helper()

	segments, err := physical_repositories.GetWalSegmentRepository().FindByChainSpan(
		fixture.DB.ID, 1, walmath.LSN(0), walSpanUpperBound)
	require.NoError(t, err)

	dockerExec(t, target.container, "mkdir", "-p", pitrWalArchiveDir)

	provisioned := 0
	for _, segment := range segments {
		if segment.FileName == nil {
			continue
		}

		hostSegment := streamArtifactToHost(t, fixture, encryptor, *segment.FileName, segment.WalFilename+".zst")
		containerSegment := pitrWalArchiveDir + "/" + segment.WalFilename + ".zst"

		dockerCp(t, hostSegment, target.container+":"+containerSegment)
		dockerExec(t, target.container, "sh", "-c",
			fmt.Sprintf("zstd -d -f < %s > %s/%s && rm -f %s",
				containerSegment, pitrWalArchiveDir, segment.WalFilename, containerSegment))

		provisioned++
	}

	require.Positive(t, provisioned, "at least one archived WAL segment must be replayable for PITR")
}

// configurePitrRecovery turns the extracted base backup into a PITR target:
// recovery.signal puts the cluster into archive recovery, recovery_target_time
// bounds the replay, and promote makes it read-write once the target is reached.
func configurePitrRecovery(t *testing.T, target restoreTarget, pgdata, targetTimestamp string) {
	t.Helper()

	recoveryConf := fmt.Sprintf(
		"restore_command = 'cp %s/%%f %%p'\n"+
			"recovery_target_time = '%s'\n"+
			"recovery_target_action = 'promote'\n",
		pitrWalArchiveDir, targetTimestamp)

	dockerExec(t, target.container, "sh", "-c",
		fmt.Sprintf("cat >> %s/postgresql.auto.conf <<'EOF'\n%sEOF", pgdata, recoveryConf))
	dockerExec(t, target.container, "sh", "-c", "touch "+pgdata+"/recovery.signal")
}

// startDatadirCluster fixes ownership/permissions and starts the datadir under
// the postgres user, registering a best-effort stop on cleanup.
func startDatadirCluster(t *testing.T, target restoreTarget, pgdata string) {
	t.Helper()

	dockerExec(t, target.container, "sh", "-c",
		"chown -R postgres:postgres "+pgdata+" "+pitrWalArchiveDir+" && chmod 0700 "+pgdata+" && "+
			"touch "+restoreWorkDir+"/pg.log && chown postgres:postgres "+restoreWorkDir+"/pg.log")

	// Surface the server log only when the test fails — recovery errors are
	// otherwise invisible behind pg_ctl's generic "could not start server".
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}

		out, _ := exec.Command("docker", "exec", target.container, "cat", restoreWorkDir+"/pg.log").CombinedOutput()
		t.Logf("=== pg.log ===\n%s", out)
	})

	dockerExecAs(t, target.container, "postgres",
		"pg_ctl", "-D", pgdata, "-l", restoreWorkDir+"/pg.log", "-w", "start")

	t.Cleanup(func() {
		_ = exec.Command("docker", "exec", "--user", "postgres", target.container,
			"pg_ctl", "-D", pgdata, "-m", "immediate", "stop").Run()
	})
}
