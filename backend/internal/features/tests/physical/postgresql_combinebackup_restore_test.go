package tests_physical

import (
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
	backups_core_enums "databasus-backend/internal/features/backups/backups/core/enums"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	postgresql_executor "databasus-backend/internal/features/backups/backups/usecases/physical/postgresql"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/walmath"
)

const (
	restoreWorkDir     = "/restore"
	restoredPgUser     = "testuser"
	restoredPgPassword = "testpassword"
	restoredPgDatabase = "testdb"
)

// restoreTarget is the idle container the chain is combined and restored in. Its
// major must match the source backup's (a PG 18 PGDATA cannot start under PG 17),
// and only the PG 18 target can run pg_verifybackup against a compressed tar.
type restoreTarget struct {
	container string
	hostPort  string
	verify    bool
}

func restoreTargetForVersion(version string) restoreTarget {
	if version == "18" {
		return restoreTarget{
			container: "test-physical-postgres-18-restore-target",
			hostPort:  config.GetEnv().TestPhysicalPostgres18RestoreTargetPort,
			verify:    true,
		}
	}

	return restoreTarget{
		container: "test-physical-postgres-17-restore-target",
		hostPort:  config.GetEnv().TestPhysicalPostgres17RestoreTargetPort,
		verify:    false,
	}
}

// Test_RestorePhysicalBackup_FullAndTwoIncrementalsViaPgCombinebackup_AllDataPresent
// runs the production flow (server-zstd + --no-manifest + reconstructed manifest)
// against both supported source majors. The PG 18 leg additionally gates manifest
// correctness with pg_verifybackup -F tar, which only PG 18 can run.
func Test_RestorePhysicalBackup_FullAndTwoIncrementalsViaPgCombinebackup_AllDataPresent(t *testing.T) {
	_, lookErr := exec.LookPath("docker")
	require.NoError(t, lookErr, "docker CLI must be on PATH")

	for _, sourceVersion := range []string{"17", "18"} {
		t.Run("pg"+sourceVersion, func(t *testing.T) {
			runRestoreChain(t, sourceVersion)
		})
	}
}

func runRestoreChain(t *testing.T, sourceVersion string) {
	fixture := postgresql_executor.SetupPhysicalDBForBackupVersion(t, sourceVersion)
	target := restoreTargetForVersion(sourceVersion)
	encryptor := encryption.GetFieldEncryptor()

	cleanupRestoreContainer(target.container)
	t.Cleanup(func() { cleanupRestoreContainer(target.container) })

	dockerExec(t, target.container, "mkdir", "-p",
		restoreWorkDir+"/full", restoreWorkDir+"/incr1", restoreWorkDir+"/incr2")

	// openSourceTestDBConn (not OpenAdminConn) so restore_marker lands in
	// `testdb` — the same database queryRestoredMarkerRows reads on the
	// restored cluster.
	sourceConn := openSourceTestDBConn(t, fixture)

	// Two incremental rounds each run GenerateWalActivity + WaitForWalSummaries
	// (up to 2m apiece); 10m is a ceiling, not a sleep — a fast run finishes fast.
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
	require.NotNil(t, fullRow.ManifestFileName, "production must persist the reconstructed manifest sidecar")
	require.NotNil(t, fullRow.StopLSN, "FULL backup must have stop_lsn")

	fullExtractDir := stageExtractAndVerify(t, fixture, encryptor, target, "full",
		*fullRow.FileName, *fullRow.ManifestFileName, fullRow.Compression)

	fullLink := physicalChainLink{
		IncrID:     nil,
		StopLSN:    *fullRow.StopLSN,
		ExtractDir: fullExtractDir,
	}

	// INCR1 is a delta against the FULL; 'after-full' proves data written between
	// FULL and INCR1 is captured by INCR1.
	incr1Link := createNextIncremental(t, ctx, sourceConn, target, nextIncrementalSpec{
		Fixture:   fixture,
		Encryptor: encryptor,
		Parent:    fullLink,
		Phase:     "after-full",
		Payload:   "row-inserted-between-full-and-incr1",
		ChildName: "incr1",
	})

	// INCR2 is a delta against INCR1; 'after-incr1' proves data written between the
	// two incrementals is captured by INCR2 — the chaining this test exists to verify.
	incr2Link := createNextIncremental(t, ctx, sourceConn, target, nextIncrementalSpec{
		Fixture:   fixture,
		Encryptor: encryptor,
		Parent:    incr1Link,
		Phase:     "after-incr1",
		Payload:   "row-inserted-between-incr1-and-incr2",
		ChildName: "incr2",
	})

	combineAndStartCluster(t, target, fullLink.ExtractDir, incr1Link.ExtractDir, incr2Link.ExtractDir)

	port, err := strconv.Atoi(target.hostPort)
	require.NoError(t, err)

	restoredPhases := queryRestoredMarkerRows(t, port)

	assert.ElementsMatch(t,
		[]string{"before-full", "after-full", "after-incr1"},
		restoredPhases,
		"restored cluster must contain the pre-FULL row plus rows written between FULL→INCR1 and INCR1→INCR2")
}

// physicalChainLink is one completed node in the FULL → INCR1 → INCR2 chain.
type physicalChainLink struct {
	IncrID     *uuid.UUID  // nil ⇒ this link is the ROOT FULL; non-nil ⇒ a completed INCR
	StopLSN    walmath.LSN // the next child waits for WAL summaries covering this
	ExtractDir string      // container dir holding this link's extracted tar (+ backup_manifest)
}

type nextIncrementalSpec struct {
	Fixture   *postgresql_executor.PhysicalDBFixture
	Encryptor encryption.FieldEncryptor
	Parent    physicalChainLink
	Phase     string // restore_marker phase inserted right before this INCR
	Payload   string // restore_marker payload for that phase
	ChildName string // dir/stem id under restoreWorkDir
}

// createNextIncremental builds, runs, extracts and verifies the next incremental
// rooted on spec.Parent. Production now writes the parent's manifest sidecar, so
// (unlike before) nothing republishes it here — resolveParentManifest fetches the
// parent row's ManifestFileName directly. It still owns the chain ordering: drive
// WAL summaries past the parent's stop_lsn before the backup runs.
func createNextIncremental(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	target restoreTarget,
	spec nextIncrementalSpec,
) physicalChainLink {
	t.Helper()

	_, err := conn.Exec(ctx,
		`INSERT INTO restore_marker (phase, payload) VALUES ($1, $2)`, spec.Phase, spec.Payload)
	require.NoError(t, err)

	// pg_basebackup --incremental needs WAL summaries covering the parent's
	// stop_lsn. Generate enough WAL to cross a segment boundary, then CHECKPOINT +
	// pg_switch_wal so the summarizer flushes a summary past stop_lsn (it never
	// summarizes the active segment, so the segment holding stop_lsn must close).
	_, err = postgresql_executor.GenerateWalActivity(ctx, conn, 32*1024*1024)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, "CHECKPOINT")
	require.NoError(t, err)

	_, err = conn.Exec(ctx, "SELECT pg_switch_wal()")
	require.NoError(t, err)

	require.NoError(t, postgresql_executor.WaitForWalSummaries(ctx, conn, spec.Parent.StopLSN, 2*time.Minute))

	childID := buildAndClaimIncrementalBackup(t, spec.Fixture, spec.Parent.IncrID)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(childID, false)
	postgresql_executor.WaitForBackupStatus(t, childID, physical_enums.PhysicalBackupTypeIncremental,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	childRow, err := physical_repositories.GetIncrementalBackupRepository().FindByID(childID)
	require.NoError(t, err)
	require.NotNil(t, childRow)
	require.NotNil(t, childRow.FileName)
	require.NotNil(t, childRow.ManifestFileName, "completed INCR must persist its manifest sidecar")
	require.NotNil(t, childRow.StopLSN, "completed INCR must have stop_lsn for the next child to wait on")

	childExtractDir := stageExtractAndVerify(t, spec.Fixture, spec.Encryptor, target, spec.ChildName,
		*childRow.FileName, *childRow.ManifestFileName, childRow.Compression)

	return physicalChainLink{
		IncrID:     &childID,
		StopLSN:    *childRow.StopLSN,
		ExtractDir: childExtractDir,
	}
}

func buildAndClaimIncrementalBackup(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	parentIncrID *uuid.UUID,
) uuid.UUID {
	t.Helper()

	incrID := uuid.New()
	incrRow := &physical_models.PhysicalIncrementalBackup{
		ID:                        incrID,
		DatabaseID:                fixture.DB.ID,
		StorageID:                 fixture.Storage.ID,
		RootFullBackupID:          fixture.BackupID,
		ParentIncrementalBackupID: parentIncrID,
		TimelineID:                1,
		Status:                    physical_enums.PhysicalBackupStatusInProgress,
		Encryption:                backups_core_enums.BackupEncryptionNone,
		CreatedAt:                 time.Now().UTC(),
	}

	require.NoError(t, physical_repositories.GetIncrementalBackupRepository().Save(incrRow))
	t.Cleanup(func() {
		_ = physical_repositories.GetIncrementalBackupRepository().DeleteByID(incrID)
	})

	claimed, err := physical_repositories.GetInFlightBackupRepository().Claim(
		storage.GetDb(), physical_repositories.ClaimSpec{
			DatabaseID: fixture.DB.ID,
			BackupType: physical_enums.PhysicalBackupTypeIncremental,
			BackupID:   incrID,
		})
	require.NoError(t, err)
	require.True(t, claimed, "INCR in-flight claim must succeed after the previous backup released the slot")
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)
	})

	return incrID
}

// decompressorFor maps the recorded codec to the in-container decompressor and the
// staged-file suffix pg_verifybackup -F tar keys off (base.tar.zst/.gz/.tar).
func decompressorFor(codec physical_enums.PhysicalBackupCompression) (decompress, suffix string) {
	switch codec {
	case physical_enums.PhysicalBackupCompressionGzip:
		return "gzip -d", ".tar.gz"

	case physical_enums.PhysicalBackupCompressionNone:
		return "cat", ".tar"

	default:
		return "zstd -d", ".tar.zst"
	}
}

// stageExtractAndVerify streams this backup's compressed artifact and its
// reconstructed manifest to the host, extracts the artifact into the target
// container, materializes the manifest as backup_manifest (pg_combinebackup needs
// one per input), and on a PG 18 target runs pg_verifybackup -F tar against the
// produced artifact + manifest. Returns the container extract dir.
func stageExtractAndVerify(
	t *testing.T,
	fixture *postgresql_executor.PhysicalDBFixture,
	encryptor encryption.FieldEncryptor,
	target restoreTarget,
	name, fileName, manifestFileName string,
	codec physical_enums.PhysicalBackupCompression,
) string {
	t.Helper()

	decompress, suffix := decompressorFor(codec)

	hostArtifact := streamArtifactToHost(t, fixture, encryptor, fileName, name+suffix)
	hostManifest := streamArtifactToHost(t, fixture, encryptor, manifestFileName, name+".manifest")

	extractDir := restoreWorkDir + "/" + name
	containerArtifact := extractDir + suffix

	dockerCp(t, hostArtifact, target.container+":"+containerArtifact)
	dockerExec(t, target.container, "sh", "-c",
		fmt.Sprintf("%s < %s | tar -xC %s", decompress, containerArtifact, extractDir))

	// Production no longer embeds backup_manifest in the tar (--no-manifest);
	// pg_combinebackup reads one per input, so place our reconstructed sidecar.
	dockerCp(t, hostManifest, target.container+":"+extractDir+"/backup_manifest")

	if target.verify {
		verifyReconstructedManifest(t, target, name, hostArtifact, hostManifest, suffix)
	}

	return extractDir
}

// verifyReconstructedManifest runs PG 18's pg_verifybackup against the produced
// compressed-tar artifact plus our reconstructed manifest. It recomputes every
// (non-pg_wal) file checksum, the file set, sizes and the manifest self-checksum,
// so it passes iff the reconstruction is correct. -n skips WAL parsing and
// -i pg_wal ignores the WAL segments --wal-method=fetch embeds (our manifest, like
// PG's, omits them).
func verifyReconstructedManifest(
	t *testing.T,
	target restoreTarget,
	name, hostArtifact, hostManifest, suffix string,
) {
	t.Helper()

	verifyDir := restoreWorkDir + "/verify-" + name
	dockerExec(t, target.container, "mkdir", "-p", verifyDir)

	dockerCp(t, hostArtifact, target.container+":"+verifyDir+"/base"+suffix)
	dockerCp(t, hostManifest, target.container+":"+verifyDir+"/backup_manifest")

	dockerExec(t, target.container,
		"pg_verifybackup", "-F", "tar", "-n", "-i", "pg_wal",
		"-m", verifyDir+"/backup_manifest", verifyDir)
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

// combineAndStartCluster reconstructs a data directory from the chain and starts
// it. inputDirs must be passed oldest→newest (FULL first); pg_combinebackup
// rejects out-of-order or gapped inputs.
func combineAndStartCluster(t *testing.T, target restoreTarget, inputDirs ...string) {
	t.Helper()

	combineArgs := append([]string{"pg_combinebackup", "-o", restoreWorkDir + "/combined"}, inputDirs...)
	dockerExec(t, target.container, combineArgs...)

	dockerExec(t, target.container, "sh", "-c",
		"chown -R postgres:postgres "+restoreWorkDir+"/combined && chmod 0700 "+restoreWorkDir+"/combined && "+
			"touch "+restoreWorkDir+"/pg.log && chown postgres:postgres "+restoreWorkDir+"/pg.log")

	dockerExecAs(t, target.container, "postgres",
		"pg_ctl", "-D", restoreWorkDir+"/combined", "-l", restoreWorkDir+"/pg.log", "-w", "start")

	t.Cleanup(func() {
		_ = exec.Command("docker", "exec", "--user", "postgres", target.container,
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

func openSourceTestDBConn(t *testing.T, fixture *postgresql_executor.PhysicalDBFixture) *pgx.Conn {
	t.Helper()

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		fixture.DB.PostgresqlPhysical.Host,
		fixture.DB.PostgresqlPhysical.Port,
		restoredPgUser,
		restoredPgPassword,
		restoredPgDatabase,
	)

	conn, err := pgx.Connect(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	return conn
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

func dockerExec(t *testing.T, container string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command("docker", append([]string{"exec", container}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec %v failed: %v\noutput: %s", args, err, out)
	}

	return out
}

func dockerExecAs(t *testing.T, container, user string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command("docker",
		append([]string{"exec", "--user", user, container}, args...)...)

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

// cleanupRestoreContainer wipes /restore inside the persistent target container
// so a previous run's leftovers don't poison this one. pg_ctl stop is best-effort
// because the cluster may not be running when called pre-test.
func cleanupRestoreContainer(container string) {
	_ = exec.Command("docker", "exec", "--user", "postgres", container,
		"pg_ctl", "-D", restoreWorkDir+"/combined", "-m", "immediate", "stop").Run()

	_ = exec.Command("docker", "exec", container,
		"sh", "-c", "rm -rf "+restoreWorkDir+"/*").Run()
}
