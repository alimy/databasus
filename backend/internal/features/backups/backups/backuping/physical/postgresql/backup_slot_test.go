package backuping_physical_postgresql_test

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backuping_physical "databasus-backend/internal/features/backups/backups/backuping/physical"
	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backups_config_physical "databasus-backend/internal/features/backups/config/physical"
	"databasus-backend/internal/features/storages"
	tasks_cancellation "databasus-backend/internal/features/tasks/cancellation"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/logger"
)

func Test_SlotName_IsDeterministicForSameDbID(t *testing.T) {
	dbID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	first := postgresql_executor.SlotName(dbID)
	second := postgresql_executor.SlotName(dbID)

	assert.Equal(t, first, second)
}

func Test_SlotName_UsesValidPGIdentifierCharacters(t *testing.T) {
	identifier := regexp.MustCompile(`^[a-z0-9_]+$`)

	for range 100 {
		name := postgresql_executor.SlotName(uuid.New())

		assert.True(t, identifier.MatchString(name),
			"slot name %q must contain only lowercase letters, digits, and underscores", name)
	}
}

func Test_SlotName_HasExpectedPrefix(t *testing.T) {
	name := postgresql_executor.SlotName(uuid.New())

	assert.True(t, strings.HasPrefix(name, "databasus_basebackup_"),
		"slot name %q must start with databasus_basebackup_", name)
}

func Test_SlotName_IsUnderPGMaxNameLength(t *testing.T) {
	const pgMaxNameLength = 63

	for range 100 {
		name := postgresql_executor.SlotName(uuid.New())

		assert.LessOrEqual(t, len(name), pgMaxNameLength,
			"slot name %q exceeds PG NAMEDATALEN-1 (%d bytes)", name, pgMaxNameLength)
	}
}

func Test_BackupSlot_PresentDuringRun_GoneAfterCompleted(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	require.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must not exist before backup")

	pollConn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(
		context.Background(), encryption.GetFieldEncryptor(),
	)
	require.NoError(t, err)
	defer func() { _ = pollConn.Close(context.Background()) }()

	var slotObserved atomic.Bool
	var pollCount atomic.Int64
	var observedSlots sync.Map

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

			pollCount.Add(1)

			rows, queryErr := pollConn.Query(context.Background(),
				"SELECT slot_name FROM pg_replication_slots",
			)
			if queryErr != nil {
				continue
			}

			for rows.Next() {
				var name string
				if scanErr := rows.Scan(&name); scanErr != nil {
					continue
				}

				observedSlots.Store(name, struct{}{})

				if name == slotName {
					slotObserved.Store(true)
				}
			}
			rows.Close()

			if slotObserved.Load() {
				return
			}
		}
	}()

	<-pollerReady

	go func() {
		defer close(backupDone)
		backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	}()

	<-backupDone
	<-pollDone

	t.Logf("poll count during run: %d, slotObserved: %v", pollCount.Load(), slotObserved.Load())

	var slotsSeen []string
	observedSlots.Range(func(k, _ any) bool {
		slotsSeen = append(slotsSeen, k.(string))
		return true
	})
	t.Logf("slots observed during run: %v", slotsSeen)

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.True(t, slotObserved.Load(),
		"per-backup slot %q should have been observed in pg_replication_slots during the run", slotName)

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"per-backup slot %q must be dropped after COMPLETED", slotName)
}

func Test_BackupSlot_AbsentWhenPreflightFails_ChainBroken(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	wrongSysID := "9999999999999999999"
	fixture.DB.PostgresqlPhysical.SystemIdentifier = &wrongSysID
	require.NoError(t, storage.GetDb().Save(fixture.DB.PostgresqlPhysical).Error)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusChainBroken, nil, 30*time.Second)

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must never be created when pre-flight fails (pre-flight runs outside WithBackupSlot)")
}

func Test_BackupSlot_OrphanFromPriorCrash_DropIfExistsRecovers(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	_, err := adminConn.Exec(context.Background(),
		"SELECT pg_create_physical_replication_slot($1, true)", slotName)
	require.NoError(t, err, "pre-create orphan slot")

	require.True(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"orphan slot must exist before backup starts")

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be cleaned up after backup completes (orphan recovered + new slot dropped)")
}

func Test_RunStartupCleanup_DropsOrphanSlots(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	_, err := adminConn.Exec(context.Background(),
		"SELECT pg_create_physical_replication_slot($1, true)", slotName)
	require.NoError(t, err, "pre-create orphan slot")

	require.True(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"orphan slot must exist before cleanup")

	require.NoError(t, postgresql_executor.RunStartupCleanup(t.Context(), logger.GetLogger()))

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"RunStartupCleanup must drop the per-backup orphan slot")
}

func Test_RunStartupCleanup_PreservesStreamerSlot(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	streamerSlot := fixture.DB.PostgresqlPhysical.ReplicationSlotName
	require.NotEmpty(t, streamerSlot, "streamer slot name should be populated by BeforeCreate")

	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	require.NoError(t, fixture.DB.PostgresqlPhysical.VerifyWalSlot(
		t.Context(), logger.GetLogger(), encryption.GetFieldEncryptor(),
	))
	t.Cleanup(func() {
		_ = fixture.DB.PostgresqlPhysical.DropWalSlot(
			context.Background(), logger.GetLogger(), encryption.GetFieldEncryptor(),
		)
	})

	require.True(t, postgresql_executor.SlotExists(t, adminConn, streamerSlot),
		"streamer slot must exist before cleanup")

	require.NoError(t, postgresql_executor.RunStartupCleanup(t.Context(), logger.GetLogger()))

	assert.True(t, postgresql_executor.SlotExists(t, adminConn, streamerSlot),
		"RunStartupCleanup must NOT drop the streamer slot (different prefix)")
}

func Test_RunStartupCleanup_PreservesUnrelatedSlots(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	unrelatedSlot := "test_unrelated_slot_" + uuid.New().String()[:8]
	_, err := adminConn.Exec(context.Background(),
		"SELECT pg_create_physical_replication_slot($1, true)", unrelatedSlot)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminConn.Exec(context.Background(),
			`SELECT pg_drop_replication_slot(slot_name)
			   FROM pg_replication_slots WHERE slot_name = $1`,
			unrelatedSlot)
	})

	require.True(t, postgresql_executor.SlotExists(t, adminConn, unrelatedSlot),
		"unrelated slot must exist before cleanup")

	require.NoError(t, postgresql_executor.RunStartupCleanup(t.Context(), logger.GetLogger()))

	assert.True(t, postgresql_executor.SlotExists(t, adminConn, unrelatedSlot),
		"RunStartupCleanup must NOT drop slots that don't match the per-backup prefix")
}

func Test_RunStartupCleanup_SkipsUnreachableSource_DoesNotFail(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	fixture.DB.PostgresqlPhysical.Host = "127.0.0.1"
	fixture.DB.PostgresqlPhysical.Port = 1
	require.NoError(t, storage.GetDb().Save(fixture.DB.PostgresqlPhysical).Error)

	err := postgresql_executor.RunStartupCleanup(t.Context(), logger.GetLogger())

	assert.NoError(t, err,
		"unreachable source must be logged + skipped, not surfaced as a failure")
}

func Test_BackupSlot_PresentDuringIncrementalRun_GoneAfterFinish(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	fullRow, err := physical_repositories.GetFullBackupRepository().FindByID(fixture.BackupID)
	require.NoError(t, err)
	require.NotNil(t, fullRow)
	require.NotNil(t, fullRow.FileName)

	fakeManifest := "fake-manifest-bytes-pg_basebackup-will-reject-this"
	require.NoError(t, fixture.Storage.SaveFile(
		t.Context(),
		encryption.GetFieldEncryptor(),
		logger.GetLogger(),
		*fullRow.FileName+".manifest",
		strings.NewReader(fakeManifest),
	))

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
	require.True(t, claimed, "INCR in-flight claim must succeed after FULL completes")
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)
	})

	observed := postgresql_executor.RunBackupAndPoll(t, fixture, slotName, func() {
		backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(incrID, false)
	})

	assert.True(t, observed,
		"per-backup slot must be observed during INCR run (slot wrap is the same helper as FULL)")
	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be dropped after INCR ends regardless of pg_basebackup success/failure")
}

func Test_BackupSlot_HasExpectedCharacteristics(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)

	pollConn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(
		context.Background(), encryption.GetFieldEncryptor(),
	)
	require.NoError(t, err)
	defer func() { _ = pollConn.Close(context.Background()) }()

	var (
		observed        atomic.Bool
		capturedType    atomic.Value
		capturedNonZero atomic.Bool
	)

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

			var (
				slotType   string
				restartLSN string
			)

			queryErr := pollConn.QueryRow(context.Background(),
				`SELECT slot_type, COALESCE(restart_lsn::text, '0/0')
				   FROM pg_replication_slots WHERE slot_name = $1`,
				slotName,
			).Scan(&slotType, &restartLSN)
			if queryErr != nil {
				continue
			}

			capturedType.Store(slotType)
			capturedNonZero.Store(restartLSN != "0/0")
			observed.Store(true)
			return
		}
	}()

	<-pollerReady

	go func() {
		defer close(backupDone)
		backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	}()

	<-backupDone
	<-pollDone

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	require.True(t, observed.Load(), "per-backup slot must be observed during run")

	assert.Equal(t, "physical", capturedType.Load(),
		"per-backup slot must be physical type (not logical)")
	assert.True(t, capturedNonZero.Load(),
		"restart_lsn must be set (immediately_reserve=true reserves WAL at creation — fetch retention depends on it)")
}

func Test_BackupSlot_ConcurrentBackupsOnDifferentDBs_NoCollision(t *testing.T) {
	fixtureA := postgresql_executor.SetupPhysicalDBForBackup(t)
	fixtureB := postgresql_executor.SetupPhysicalDBForBackup(t)

	require.NotEqual(t, fixtureA.DB.PostgresqlPhysical.ID, fixtureB.DB.PostgresqlPhysical.ID)

	slotA := postgresql_executor.SlotName(fixtureA.DB.PostgresqlPhysical.ID)
	slotB := postgresql_executor.SlotName(fixtureB.DB.PostgresqlPhysical.ID)
	require.NotEqual(t, slotA, slotB, "different DBs must produce different slot names")

	var observedA, observedB atomic.Bool

	doneA := make(chan struct{})
	doneB := make(chan struct{})

	go func() {
		defer close(doneA)
		observedA.Store(postgresql_executor.RunBackupAndPoll(t, fixtureA, slotA, func() {
			backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixtureA.BackupID, false)
		}))
	}()

	go func() {
		defer close(doneB)
		observedB.Store(postgresql_executor.RunBackupAndPoll(t, fixtureB, slotB, func() {
			backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixtureB.BackupID, false)
		}))
	}()

	<-doneA
	<-doneB

	postgresql_executor.WaitForBackupStatus(t, fixtureA.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)
	postgresql_executor.WaitForBackupStatus(t, fixtureB.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.True(t, observedA.Load(), "slot A should be observed during DB A's backup")
	assert.True(t, observedB.Load(), "slot B should be observed during DB B's backup")

	adminConnA := postgresql_executor.OpenAdminConn(t, fixtureA)
	adminConnB := postgresql_executor.OpenAdminConn(t, fixtureB)

	assert.False(t, postgresql_executor.SlotExists(t, adminConnA, slotA), "slot A must be dropped after DB A's backup")
	assert.False(t, postgresql_executor.SlotExists(t, adminConnB, slotB), "slot B must be dropped after DB B's backup")
}

func Test_RunStartupCleanup_OrphanFromDeletedDB_NotCleaned(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)

	adminConn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(
		context.Background(), encryption.GetFieldEncryptor(),
	)
	require.NoError(t, err)
	defer func() { _ = adminConn.Close(context.Background()) }()

	_, err = adminConn.Exec(context.Background(),
		"SELECT pg_create_physical_replication_slot($1, true)", slotName)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminConn.Exec(context.Background(),
			`SELECT pg_drop_replication_slot(slot_name)
			   FROM pg_replication_slots WHERE slot_name = $1`,
			slotName)
	})

	require.True(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"orphan slot must exist before deletion")

	require.NoError(t, storage.GetDb().Delete(fixture.DB.PostgresqlPhysical).Error,
		"delete the PostgresqlPhysical row — simulates a crashed OnBeforeDatabaseRemove hook "+
			"that removed the physical-DB record but didn't drop the slot on source")

	require.NoError(t, postgresql_executor.RunStartupCleanup(t.Context(), logger.GetLogger()))

	assert.True(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"RunStartupCleanup iterates registered physical DBs only (Preload returns nil for the deleted "+
			"PostgresqlPhysical) — orphan slots from deleted DBs remain. This is a known limitation; "+
			"recovery requires manual cleanup on the source PG.")
}

func Test_BackupSlot_TwoBackupsInRow_SecondReusesNameOk(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	require.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be gone after first backup")

	secondID := uuid.New()
	secondRow := &physical_models.PhysicalFullBackup{
		ID:         secondID,
		DatabaseID: fixture.DB.ID,
		StorageID:  fixture.Storage.ID,
		TimelineID: 1,
		Status:     physical_enums.PhysicalBackupStatusInProgress,
		Encryption: physical_enums.PhysicalBackupEncryptionNone,
		CreatedAt:  time.Now().UTC(),
	}

	require.NoError(t, physical_repositories.GetFullBackupRepository().Save(secondRow))
	t.Cleanup(func() {
		_ = physical_repositories.GetFullBackupRepository().DeleteByID(secondID)
	})

	claimed, err := physical_repositories.GetInFlightBackupRepository().Claim(
		storage.GetDb(),
		fixture.DB.ID,
		physical_enums.PhysicalBackupTypeFull,
		secondID,
	)
	require.NoError(t, err)
	require.True(t, claimed, "second in-flight claim must succeed (first one was released on COMPLETED)")
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)
	})

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(secondID, false)
	postgresql_executor.WaitForBackupStatus(t, secondID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be gone after second backup; drop-if-exists + defer drop are robust to repeat runs")
}

func Test_BackupSlot_PresentDuringRun_PG18(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackupVersion(t, "18")

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	observed := postgresql_executor.RunBackupAndPoll(t, fixture, slotName, func() {
		backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	})

	postgresql_executor.WaitForBackupStatus(t, fixture.BackupID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.True(t, observed,
		"per-backup slot must be observed during run on PG18 (same lifecycle as PG17)")
	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be dropped after COMPLETED on PG18")
}

func Test_BackupSlot_RetryAfterFailure_ReusesName(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	flakyStorage := storages.CreateTestFlakyS3Storage(*fixture.DB.WorkspaceID,
		"http://127.0.0.1:1")
	t.Cleanup(func() { storages.RemoveTestStorage(flakyStorage.ID) })

	cfgService := backups_config_physical.GetBackupConfigService()
	cfg, err := cfgService.GetBackupConfigByDbId(fixture.DB.ID)
	require.NoError(t, err)

	cfg.StorageID = &flakyStorage.ID
	cfg.Storage = flakyStorage
	_, err = cfgService.SaveBackupConfig(cfg)
	require.NoError(t, err)

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)

	finalRow, err := physical_repositories.GetFullBackupRepository().FindByID(fixture.BackupID)
	require.NoError(t, err)
	require.NotEqual(t, physical_enums.PhysicalBackupStatusCompleted, finalRow.Status,
		"first backup must not COMPLETED with flaky storage; got status=%s", finalRow.Status)

	require.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be cleaned up after failed first run (defer-drop must run even when SaveFile fails)")

	cfg, err = cfgService.GetBackupConfigByDbId(fixture.DB.ID)
	require.NoError(t, err)

	cfg.StorageID = &fixture.Storage.ID
	cfg.Storage = fixture.Storage
	_, err = cfgService.SaveBackupConfig(cfg)
	require.NoError(t, err)

	_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)

	retryID := uuid.New()
	retryRow := &physical_models.PhysicalFullBackup{
		ID:         retryID,
		DatabaseID: fixture.DB.ID,
		StorageID:  fixture.Storage.ID,
		TimelineID: 1,
		Status:     physical_enums.PhysicalBackupStatusInProgress,
		Encryption: physical_enums.PhysicalBackupEncryptionNone,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, physical_repositories.GetFullBackupRepository().Save(retryRow))
	t.Cleanup(func() {
		_ = physical_repositories.GetFullBackupRepository().DeleteByID(retryID)
	})

	claimed, err := physical_repositories.GetInFlightBackupRepository().Claim(
		storage.GetDb(), fixture.DB.ID, physical_enums.PhysicalBackupTypeFull, retryID,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() {
		_ = physical_repositories.GetInFlightBackupRepository().Release(fixture.DB.ID)
	})

	backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(retryID, false)
	postgresql_executor.WaitForBackupStatus(t, retryID, physical_enums.PhysicalBackupTypeFull,
		physical_enums.PhysicalBackupStatusCompleted, nil, 3*time.Minute)

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be cleaned up after retry COMPLETED; both runs left no orphan")
}

func Test_BackupSlot_DroppedAfterCancelledMidRun(t *testing.T) {
	fixture := postgresql_executor.SetupPhysicalDBForBackup(t)

	slotName := postgresql_executor.SlotName(fixture.DB.PostgresqlPhysical.ID)
	adminConn := postgresql_executor.OpenAdminConn(t, fixture)

	pollConn, err := fixture.DB.PostgresqlPhysical.OpenInspectionConn(
		context.Background(), encryption.GetFieldEncryptor(),
	)
	require.NoError(t, err)
	defer func() { _ = pollConn.Close(context.Background()) }()

	var slotObserved atomic.Bool

	backupDone := make(chan struct{})
	cancelTriggered := make(chan struct{})

	go func() {
		defer close(backupDone)
		backuping_physical.CreateTestPhysicalBackuper(nil).MakeBackup(fixture.BackupID, false)
	}()

	go func() {
		defer close(cancelTriggered)

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
			if queryErr != nil {
				continue
			}

			if exists {
				slotObserved.Store(true)
				_ = tasks_cancellation.GetTaskCancelManager().CancelTask(fixture.BackupID)
				return
			}
		}
	}()

	<-backupDone
	<-cancelTriggered

	if !slotObserved.Load() {
		t.Skip("backup completed too fast for cancellation to land while slot existed — test is timing-dependent")
	}

	assert.False(t, postgresql_executor.SlotExists(t, adminConn, slotName),
		"slot must be dropped after cancellation (defer uses context.Background() so it survives ctx cancel)")
}
