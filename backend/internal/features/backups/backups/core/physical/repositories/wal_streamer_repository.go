package physical_repositories

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	"databasus-backend/internal/storage"
)

type PhysicalWalStreamerRepository struct{}

// Idempotent — supervisor reclaiming a previously-failed streamer is a
// normal flow, so an existing row bumps the heartbeat and flips status back
// to RUNNING instead of returning a conflict.
func (r *PhysicalWalStreamerRepository) Claim(databaseID uuid.UUID) error {
	if databaseID == uuid.Nil {
		return errors.New("database ID is required")
	}

	return storage.
		GetDb().
		Exec(`
			INSERT INTO physical_wal_streamers (database_id, started_at, last_heartbeat_at, status)
			VALUES (?, NOW(), NOW(), ?)
			ON CONFLICT (database_id) DO UPDATE
			    SET started_at        = NOW(),
			        last_heartbeat_at = NOW(),
			        status            = EXCLUDED.status
		`, databaseID, physical_enums.PhysicalWalStreamerStatusRunning).Error
}

func (r *PhysicalWalStreamerRepository) Heartbeat(databaseID uuid.UUID) error {
	return storage.
		GetDb().
		Model(&physical_models.PhysicalWalStreamer{}).
		Where("database_id = ?", databaseID).
		Update("last_heartbeat_at", time.Now().UTC()).Error
}

func (r *PhysicalWalStreamerRepository) MarkFailed(databaseID uuid.UUID) error {
	return storage.
		GetDb().
		Model(&physical_models.PhysicalWalStreamer{}).
		Where("database_id = ?", databaseID).
		Update("status", physical_enums.PhysicalWalStreamerStatusFailed).Error
}

func (r *PhysicalWalStreamerRepository) FindByDatabaseID(
	databaseID uuid.UUID,
) (*physical_models.PhysicalWalStreamer, error) {
	var row physical_models.PhysicalWalStreamer

	if err := storage.GetDb().Where("database_id = ?", databaseID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &row, nil
}

func (r *PhysicalWalStreamerRepository) DeleteByDatabaseID(databaseID uuid.UUID) error {
	return storage.
		GetDb().
		Delete(&physical_models.PhysicalWalStreamer{}, "database_id = ?", databaseID).Error
}
