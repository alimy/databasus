package physical_testing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/walmath"
)

// NewTestCompletedFullBackup returns a fully-populated COMPLETED FULL ready
// for Save. Tests can mutate any field before persisting (e.g. set
// CompletedAt to a fixed past time for retention math).
func NewTestCompletedFullBackup(
	databaseID, storageID uuid.UUID,
	timelineID int,
	startLSN, stopLSN walmath.LSN,
) *physical_models.PhysicalFullBackup {
	now := time.Now().UTC()
	fileName := "test-full-" + uuid.New().String() + ".tar.zst"

	return &physical_models.PhysicalFullBackup{
		DatabaseID:  databaseID,
		StorageID:   storageID,
		TimelineID:  timelineID,
		Status:      physical_enums.PhysicalBackupStatusCompleted,
		FileName:    &fileName,
		StartLSN:    &startLSN,
		StopLSN:     &stopLSN,
		CreatedAt:   now,
		CompletedAt: &now,
	}
}

func NewTestCompletedIncrementalBackup(
	databaseID, storageID, rootFullBackupID uuid.UUID,
	parentIncrementalBackupID *uuid.UUID,
	timelineID int,
	startLSN, stopLSN walmath.LSN,
) *physical_models.PhysicalIncrementalBackup {
	now := time.Now().UTC()
	fileName := "test-incr-" + uuid.New().String() + ".tar.zst"

	return &physical_models.PhysicalIncrementalBackup{
		DatabaseID:                databaseID,
		StorageID:                 storageID,
		TimelineID:                timelineID,
		Status:                    physical_enums.PhysicalBackupStatusCompleted,
		FileName:                  &fileName,
		StartLSN:                  &startLSN,
		StopLSN:                   &stopLSN,
		CreatedAt:                 now,
		CompletedAt:               &now,
		RootFullBackupID:          rootFullBackupID,
		ParentIncrementalBackupID: parentIncrementalBackupID,
	}
}

func NewTestWalSegment(
	databaseID, storageID uuid.UUID,
	timelineID int,
	walFilename string,
	startLSN, endLSN walmath.LSN,
) *physical_models.PhysicalWalSegment {
	now := time.Now().UTC()
	fileName := "test-wal-" + uuid.New().String() + ".zst"

	return &physical_models.PhysicalWalSegment{
		DatabaseID:       databaseID,
		StorageID:        storageID,
		TimelineID:       timelineID,
		FileName:         &fileName,
		WalFilename:      walFilename,
		StartLSN:         startLSN,
		EndLSN:           endLSN,
		CompressedSizeMb: 16,
		ReceivedAt:       now,
		ClaimedAt:        now,
	}
}

func NewTestWalHistoryFile(
	databaseID, storageID uuid.UUID,
	timelineID int,
) *physical_models.PhysicalWalHistoryFile {
	return &physical_models.PhysicalWalHistoryFile{
		DatabaseID:       databaseID,
		StorageID:        storageID,
		TimelineID:       timelineID,
		FileName:         "test-hist-" + uuid.New().String() + ".history.zst",
		HistoryFilename:  walmath.FormatHistoryFilename(uint32(timelineID)),
		CompressedSizeMb: 0.01,
		CreatedAt:        time.Now().UTC(),
	}
}

// CreateTestFullBackup persists a FULL via the repository. Tests use
// NewTestCompletedFullBackup (or build the struct directly) to set the row
// up, then hand it to this helper to save.
func CreateTestFullBackup(
	t *testing.T,
	b *physical_models.PhysicalFullBackup,
) *physical_models.PhysicalFullBackup {
	t.Helper()

	if err := physical_repositories.GetFullBackupRepository().Save(b); err != nil {
		t.Fatalf("save test full backup: %v", err)
	}

	return b
}

func CreateTestIncrementalBackup(
	t *testing.T,
	b *physical_models.PhysicalIncrementalBackup,
) *physical_models.PhysicalIncrementalBackup {
	t.Helper()

	if err := physical_repositories.GetIncrementalBackupRepository().Save(b); err != nil {
		t.Fatalf("save test incremental backup: %v", err)
	}

	return b
}

func CreateTestWalSegment(
	t *testing.T,
	seg *physical_models.PhysicalWalSegment,
) *physical_models.PhysicalWalSegment {
	t.Helper()

	if err := physical_repositories.GetWalSegmentRepository().Insert(seg); err != nil {
		t.Fatalf("save test wal segment: %v", err)
	}

	return seg
}

func CreateTestWalHistoryFile(
	t *testing.T,
	h *physical_models.PhysicalWalHistoryFile,
) *physical_models.PhysicalWalHistoryFile {
	t.Helper()

	if err := physical_repositories.GetWalHistoryRepository().Insert(h); err != nil {
		t.Fatalf("save test wal history: %v", err)
	}

	return h
}

// DeleteAllPhysicalCatalogForDatabase wipes every row physical/* owns for the
// given database, in FK-safe order. Use in t.Cleanup so tests don't leak
// state across runs.
func DeleteAllPhysicalCatalogForDatabase(t *testing.T, databaseID uuid.UUID) {
	t.Helper()

	db := storage.GetDb()

	if err := db.
		Exec(`DELETE FROM physical_wal_segments WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup wal segments: %v", err)
	}

	if err := db.
		Exec(`DELETE FROM physical_wal_history_files WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup wal history: %v", err)
	}

	if err := db.
		Exec(`DELETE FROM physical_in_flight_backups WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup in-flight: %v", err)
	}

	if err := db.
		Exec(`DELETE FROM physical_wal_streamers WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup wal streamers: %v", err)
	}

	if err := db.
		Exec(`DELETE FROM physical_incremental_backups WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup incremental backups: %v", err)
	}

	if err := db.
		Exec(`DELETE FROM physical_full_backups WHERE database_id = ?`, databaseID).Error; err != nil {
		t.Fatalf("cleanup full backups: %v", err)
	}
}
