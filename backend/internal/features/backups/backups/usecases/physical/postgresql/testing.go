package usecases_physical_postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	backups_core_enums "databasus-backend/internal/features/backups/backups/core/enums"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/features/intervals"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	users_enums "databasus-backend/internal/features/users/enums"
	users_testing "databasus-backend/internal/features/users/testing"
	workspaces_controllers "databasus-backend/internal/features/workspaces/controllers"
	workspaces_models "databasus-backend/internal/features/workspaces/models"
	workspaces_testing "databasus-backend/internal/features/workspaces/testing"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/logger"
	"databasus-backend/internal/util/walmath"
)

// PhysicalDBFixture is a fully-wired physical DB ready for FULL backup
// dispatch: workspace, storage, notifier, DB row, backup config, in-flight
// claim, and a backup row in IN_PROGRESS. BackupID + DB are the two handles
// most tests need; the rest are kept so tests can mutate config or hit the
// source PG.
type PhysicalDBFixture struct {
	Workspace *workspaces_models.Workspace
	Storage   *storages.Storage
	Notifier  *notifiers.Notifier
	DB        *databases.Database
	BackupID  uuid.UUID
}

// SetupPhysicalDBForBackup builds a PG17 fixture. Skips the test if the PG17
// container env var is unset.
func SetupPhysicalDBForBackup(t *testing.T) *PhysicalDBFixture {
	return SetupPhysicalDBForBackupVersion(t, "17")
}

// SetupPhysicalDBForBackupVersion builds a fixture against a specific PG
// major version. The container env vars are enforced by config validation at
// startup, so this helper does not re-check them.
func SetupPhysicalDBForBackupVersion(t *testing.T, version string) *PhysicalDBFixture {
	t.Helper()

	router := workspaces_testing.CreateTestRouter(
		workspaces_controllers.GetWorkspaceController(),
		workspaces_controllers.GetMembershipController(),
		databases.GetDatabaseController(),
		backups_config_logical.GetBackupConfigController(),
	)

	owner := users_testing.CreateTestUser(users_enums.UserRoleAdmin)

	workspace := workspaces_testing.CreateTestWorkspace("ws "+uuid.New().String(), owner, router)
	t.Cleanup(func() { workspaces_testing.RemoveTestWorkspace(workspace, router) })

	testStorage := storages.CreateTestStorage(workspace.ID)
	t.Cleanup(func() { storages.RemoveTestStorage(testStorage.ID) })

	notifier := notifiers.CreateTestNotifier(workspace.ID)
	t.Cleanup(func() { notifiers.RemoveTestNotifier(notifier) })

	db := databases.CreateTestPhysicalPostgresDatabase(workspace.ID, notifier, version)
	t.Cleanup(func() { databases.RemoveTestDatabase(db) })

	encryptor := encryption.GetFieldEncryptor()
	log := logger.GetLogger()

	require.NoError(t, db.PostgresqlPhysical.PopulateDbData(log, encryptor))

	// CreateTestPhysicalPostgresDatabase saved the row before PopulateDbData ran,
	// so the captured system_identifier lives only on the in-memory object. The
	// backuper reloads the DB from the repo and the manifest's System-Identifier
	// comes from the stored row, so persist it here — mirroring production, where
	// the database service populates before saving.
	require.NoError(t, storage.GetDb().
		Model(db.PostgresqlPhysical).
		Update("system_identifier", db.PostgresqlPhysical.SystemIdentifier).Error)

	cfgService := backups_config_physical.GetBackupConfigService()

	cfg, err := cfgService.GetBackupConfigByDbId(db.ID)
	require.NoError(t, err)

	cfg.IsBackupsEnabled = true
	cfg.StorageID = &testStorage.ID
	cfg.Storage = testStorage
	cfg.Encryption = backups_core_enums.BackupEncryptionNone
	cfg.PostgresqlPhysical = db.PostgresqlPhysical
	cfg.FullBackupInterval = intervals.Interval{
		Type:      intervals.IntervalDaily,
		TimeOfDay: new("04:00"),
	}

	_, err = cfgService.SaveBackupConfig(cfg)
	require.NoError(t, err)

	backupID := uuid.New()
	backupRow := &physical_models.PhysicalFullBackup{
		ID:         backupID,
		DatabaseID: db.ID,
		StorageID:  testStorage.ID,
		TimelineID: 1,
		Status:     physical_enums.PhysicalBackupStatusInProgress,
		Encryption: backups_core_enums.BackupEncryptionNone,
		CreatedAt:  time.Now().UTC(),
	}

	require.NoError(t, physical_repositories.GetFullBackupRepository().Save(backupRow))
	t.Cleanup(func() {
		_ = physical_repositories.GetFullBackupRepository().DeleteByID(backupID)
	})

	claimed, err := physical_repositories.GetInFlightBackupRepository().Claim(
		storage.GetDb(),
		db.ID,
		physical_enums.PhysicalBackupTypeFull,
		backupID,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(db.ID)
	})

	return &PhysicalDBFixture{
		Workspace: workspace,
		Storage:   testStorage,
		Notifier:  notifier,
		DB:        db,
		BackupID:  backupID,
	}
}

// OpenAdminConn returns an inspection connection to the source PG with
// automatic close on test cleanup.
func OpenAdminConn(t *testing.T, fixture *PhysicalDBFixture) *pgx.Conn {
	t.Helper()

	conn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(context.Background(), encryption.GetFieldEncryptor())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	return conn
}

// SlotExists reports whether the named slot is present in pg_replication_slots.
func SlotExists(t *testing.T, conn *pgx.Conn, slotName string) bool {
	t.Helper()

	var exists bool
	err := conn.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)",
		slotName,
	).Scan(&exists)
	require.NoError(t, err)

	return exists
}

// RunBackupAndPoll spawns runBackup in a goroutine while a second goroutine
// polls pg_replication_slots for slotName on its own connection. Returns
// once runBackup completes. The bool is set when the slot was seen at least
// once during the run.
func RunBackupAndPoll(
	t *testing.T,
	fixture *PhysicalDBFixture,
	slotName string,
	runBackup func(),
) bool {
	t.Helper()

	pollConn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(
		context.Background(), encryption.GetFieldEncryptor(),
	)
	require.NoError(t, err)
	defer func() { _ = pollConn.Close(context.Background()) }()

	var observed atomic.Bool

	backupDone := make(chan struct{})
	pollerReady := make(chan struct{})
	pollDone := make(chan struct{})

	go func() {
		defer close(pollDone)
		close(pollerReady)

		for {
			select {
			case <-backupDone:
				return
			default:
			}

			var exists bool
			queryErr := pollConn.QueryRow(context.Background(),
				"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)",
				slotName,
			).Scan(&exists)
			if queryErr == nil && exists {
				observed.Store(true)
				return
			}
		}
	}()

	<-pollerReady

	go func() {
		defer close(backupDone)
		runBackup()
	}()

	<-backupDone
	<-pollDone

	return observed.Load()
}

// GenerateWalActivity inserts rows into a throwaway LOGGED table until the
// cluster's pg_current_wal_lsn() advances by at least minBytes. The table is
// dropped on return. UNLOGGED/TEMP would skip WAL — LOGGED guarantees the
// WAL advance the test is asserting on.
func GenerateWalActivity(
	ctx context.Context,
	conn *pgx.Conn,
	minBytes int64,
) (int64, error) {
	tableName := "wal_activity_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]

	if _, err := conn.Exec(ctx,
		fmt.Sprintf(`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, payload TEXT)`, tableName),
	); err != nil {
		return 0, fmt.Errorf("create wal activity table: %w", err)
	}

	defer func() {
		_, _ = conn.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tableName))
	}()

	var startLSN walmath.LSN
	if err := conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&startLSN); err != nil {
		return 0, fmt.Errorf("read start LSN: %w", err)
	}

	const rowsPerBatch = 1000

	const payloadSize = 1024

	for {
		_, err := conn.Exec(ctx,
			fmt.Sprintf(
				`INSERT INTO %s (payload) SELECT repeat('x', %d) FROM generate_series(1, %d)`,
				tableName, payloadSize, rowsPerBatch,
			),
		)
		if err != nil {
			return 0, fmt.Errorf("insert wal activity rows: %w", err)
		}

		var currentLSN walmath.LSN
		if err := conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&currentLSN); err != nil {
			return 0, fmt.Errorf("read current LSN: %w", err)
		}

		delta := int64(currentLSN) - int64(startLSN)
		if delta >= minBytes {
			return delta, nil
		}

		if ctx.Err() != nil {
			return delta, ctx.Err()
		}
	}
}

// WaitForWalSummaries polls pg_available_wal_summaries() until the summarizer
// covers untilLSN or the timeout expires.
func WaitForWalSummaries(
	ctx context.Context,
	conn *pgx.Conn,
	untilLSN walmath.LSN,
	timeout time.Duration,
) error {
	deadline := time.Now().UTC().Add(timeout)

	for time.Now().UTC().Before(deadline) {
		var exists bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_available_wal_summaries()
				WHERE start_lsn <= $1::pg_lsn AND end_lsn >= $1::pg_lsn
			)
		`, untilLSN.String()).Scan(&exists)
		if err != nil {
			return fmt.Errorf("query pg_available_wal_summaries: %w", err)
		}

		if exists {
			return nil
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("wal summary coverage of %s not reached within %s", untilLSN.String(), timeout)
}

// WaitForBackupStatus polls the typed table for backupID until it reaches
// the expected status (and optional error_reason) or the timeout expires.
// kind selects which table (FULL or INCREMENTAL).
func WaitForBackupStatus(
	t *testing.T,
	backupID uuid.UUID,
	kind physical_enums.PhysicalBackupType,
	expectedStatus physical_enums.PhysicalBackupStatus,
	expectedReason *physical_enums.PhysicalBackupErrorReason,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().UTC().Add(timeout)

	for time.Now().UTC().Before(deadline) {
		status, reason, found := readBackupStatus(t, backupID, kind)
		if found && status == expectedStatus && reasonsMatch(reason, expectedReason) {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	status, reason, _ := readBackupStatus(t, backupID, kind)

	t.Fatalf(
		"backup %s did not reach status=%s reason=%s within %s (observed status=%s reason=%s)",
		backupID, expectedStatus, reasonString(expectedReason), timeout,
		status, reasonString(reason),
	)
}

func readBackupStatus(
	t *testing.T,
	backupID uuid.UUID,
	kind physical_enums.PhysicalBackupType,
) (physical_enums.PhysicalBackupStatus, *physical_enums.PhysicalBackupErrorReason, bool) {
	t.Helper()

	switch kind {
	case physical_enums.PhysicalBackupTypeFull:
		var row physical_models.PhysicalFullBackup
		if err := storage.GetDb().Where("id = ?", backupID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", nil, false
			}

			t.Fatalf("read full backup row: %v", err)
		}

		return row.Status, row.ErrorReason, true

	case physical_enums.PhysicalBackupTypeIncremental:
		var row physical_models.PhysicalIncrementalBackup
		if err := storage.GetDb().Where("id = ?", backupID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", nil, false
			}

			t.Fatalf("read incr backup row: %v", err)
		}

		return row.Status, row.ErrorReason, true
	}

	t.Fatalf("unsupported backup kind: %s", kind)

	return "", nil, false
}

func reasonsMatch(a, b *physical_enums.PhysicalBackupErrorReason) bool {
	if b == nil {
		return true
	}

	if a == nil {
		return false
	}

	return *a == *b
}

func reasonString(r *physical_enums.PhysicalBackupErrorReason) string {
	if r == nil {
		return "<nil>"
	}

	return string(*r)
}

// SetSummarizerEnabled toggles summarize_wal at runtime via ALTER SYSTEM +
// pg_reload_conf. Auto-restores on t.Cleanup so tests can flip the
// summarizer mid-test without leaking state across cases.
func SetSummarizerEnabled(t *testing.T, conn *pgx.Conn, enabled bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var previous string
	if err := conn.QueryRow(ctx,
		"SELECT setting FROM pg_settings WHERE name = 'summarize_wal'",
	).Scan(&previous); err != nil {
		t.Fatalf("read summarize_wal: %v", err)
	}

	desired := "off"
	if enabled {
		desired = "on"
	}

	if previous == desired {
		return
	}

	if _, err := conn.Exec(ctx,
		fmt.Sprintf("ALTER SYSTEM SET summarize_wal = '%s'", desired),
	); err != nil {
		t.Fatalf("ALTER SYSTEM summarize_wal=%s: %v", desired, err)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_reload_conf()"); err != nil {
		t.Fatalf("pg_reload_conf: %v", err)
	}

	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer restoreCancel()

		if _, err := conn.Exec(restoreCtx,
			fmt.Sprintf("ALTER SYSTEM SET summarize_wal = '%s'", previous),
		); err != nil {
			t.Logf("restore summarize_wal=%s: %v", previous, err)

			return
		}

		_, _ = conn.Exec(restoreCtx, "SELECT pg_reload_conf()")
	})
}

// ExpireWalSummaries forces summary file expiry by temporarily dropping
// wal_summary_keep_time to 1 minute and waiting one sweep cycle. Auto-
// restores the original value on t.Cleanup.
func ExpireWalSummaries(t *testing.T, conn *pgx.Conn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var previous string
	if err := conn.QueryRow(ctx,
		"SELECT setting FROM pg_settings WHERE name = 'wal_summary_keep_time'",
	).Scan(&previous); err != nil {
		t.Fatalf("read wal_summary_keep_time: %v", err)
	}

	if _, err := conn.Exec(ctx,
		"ALTER SYSTEM SET wal_summary_keep_time = '1min'",
	); err != nil {
		t.Fatalf("ALTER SYSTEM wal_summary_keep_time: %v", err)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_reload_conf()"); err != nil {
		t.Fatalf("pg_reload_conf: %v", err)
	}

	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer restoreCancel()

		if _, err := conn.Exec(restoreCtx,
			fmt.Sprintf("ALTER SYSTEM SET wal_summary_keep_time = '%s'", previous),
		); err != nil {
			t.Logf("restore wal_summary_keep_time: %v", err)

			return
		}

		_, _ = conn.Exec(restoreCtx, "SELECT pg_reload_conf()")
	})
}
