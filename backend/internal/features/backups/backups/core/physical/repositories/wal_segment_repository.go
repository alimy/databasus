package physical_repositories

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/walmath"
)

type PhysicalWalSegmentRepository struct{}

func (r *PhysicalWalSegmentRepository) Insert(segment *physical_models.PhysicalWalSegment) error {
	if segment.DatabaseID == uuid.Nil || segment.StorageID == uuid.Nil {
		return errors.New("database ID and storage ID are required")
	}

	if segment.ID == uuid.Nil {
		segment.ID = uuid.New()
	}

	now := time.Now().UTC()
	if segment.ReceivedAt.IsZero() {
		segment.ReceivedAt = now
	}
	if segment.ClaimedAt.IsZero() {
		segment.ClaimedAt = now
	}

	return storage.GetDb().Create(segment).Error
}

func (r *PhysicalWalSegmentRepository) FindByID(id uuid.UUID) (*physical_models.PhysicalWalSegment, error) {
	var segment physical_models.PhysicalWalSegment

	if err := storage.GetDb().Where("id = ?", id).First(&segment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &segment, nil
}

func (r *PhysicalWalSegmentRepository) FindByChainSpan(
	databaseID uuid.UUID,
	timelineID int,
	startLSN, endLSN walmath.LSN,
) ([]*physical_models.PhysicalWalSegment, error) {
	var segments []*physical_models.PhysicalWalSegment

	if err := storage.
		GetDb().
		Where(
			"database_id = ? AND timeline_id = ? AND start_lsn >= ?::pg_lsn AND start_lsn < ?::pg_lsn",
			databaseID, timelineID, startLSN.String(), endLSN.String(),
		).
		Order("start_lsn ASC").
		Find(&segments).Error; err != nil {
		return nil, err
	}

	return segments, nil
}

// Anti-join not covered by idx_physical_wal_segments_database_id_received_at;
// expect a seq scan on physical_full_backups when invoked in bulk.
func (r *PhysicalWalSegmentRepository) FindOrphans(
	databaseID uuid.UUID,
) ([]*physical_models.PhysicalWalSegment, error) {
	var orphans []*physical_models.PhysicalWalSegment

	if err := storage.
		GetDb().
		Raw(`
			SELECT *
			FROM physical_wal_segments w
			WHERE w.database_id = ?
			  AND NOT EXISTS (
			    SELECT 1
			    FROM physical_full_backups f
			    WHERE f.database_id = w.database_id
			      AND f.timeline_id = w.timeline_id
			      AND f.start_lsn IS NOT NULL
			      AND f.start_lsn <= w.start_lsn
			      AND f.status = ?
			  )
			ORDER BY w.start_lsn ASC
		`, databaseID, physical_enums.PhysicalBackupStatusCompleted).
		Scan(&orphans).Error; err != nil {
		return nil, err
	}

	return orphans, nil
}

func (r *PhysicalWalSegmentRepository) DeleteByID(id uuid.UUID) error {
	return storage.GetDb().Delete(&physical_models.PhysicalWalSegment{}, "id = ?", id).Error
}

// DeleteAbandonedClaims removes insert-first WAL claim rows whose upload never
// finished (file_name still NULL) and that have aged past the grace period. A
// NULL file_name is proof no bytes were ever written under any name, so there
// is no storage object to delete. Returns the number of rows removed.
func (r *PhysicalWalSegmentRepository) DeleteAbandonedClaims(
	databaseID uuid.UUID,
	olderThan time.Time,
) (int64, error) {
	result := storage.
		GetDb().
		Where("database_id = ? AND file_name IS NULL AND claimed_at < ?", databaseID, olderThan).
		Delete(&physical_models.PhysicalWalSegment{})

	return result.RowsAffected, result.Error
}
