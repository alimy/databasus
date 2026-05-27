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

func (r *PhysicalWalSegmentRepository) Insert(seg *physical_models.PhysicalWalSegment) error {
	if seg.DatabaseID == uuid.Nil || seg.StorageID == uuid.Nil {
		return errors.New("database ID and storage ID are required")
	}

	if seg.ID == uuid.Nil {
		seg.ID = uuid.New()
	}

	now := time.Now().UTC()
	if seg.ReceivedAt.IsZero() {
		seg.ReceivedAt = now
	}
	if seg.ClaimedAt.IsZero() {
		seg.ClaimedAt = now
	}

	return storage.GetDb().Create(seg).Error
}

func (r *PhysicalWalSegmentRepository) FindByID(id uuid.UUID) (*physical_models.PhysicalWalSegment, error) {
	var seg physical_models.PhysicalWalSegment

	if err := storage.GetDb().Where("id = ?", id).First(&seg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &seg, nil
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

// FindOrphans returns WAL segments whose (timeline_id, start_lsn) cannot be
// matched to any chain on that timeline — i.e. no FULL exists on the same
// (database_id, timeline_id) with start_lsn <= this WAL's start_lsn. Used by
// the cleaner orphan pass.
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
