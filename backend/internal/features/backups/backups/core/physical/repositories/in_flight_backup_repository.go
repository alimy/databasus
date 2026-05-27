package physical_repositories

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	"databasus-backend/internal/storage"
)

type PhysicalInFlightBackupRepository struct{}

// Claim atomically reserves the cross-table single-in-flight slot for a
// database. Returns true on success, false on conflict (someone else already
// holds the slot). The caller passes its transaction handle so the claim
// commits together with the typed-table INSERT — that's the whole point of
// the slot existing in DB rather than in memory.
func (r *PhysicalInFlightBackupRepository) Claim(
	db *gorm.DB,
	databaseID uuid.UUID,
	backupType physical_enums.PhysicalBackupType,
	backupID uuid.UUID,
) (bool, error) {
	if databaseID == uuid.Nil || backupID == uuid.Nil {
		return false, errors.New("database ID and backup ID are required")
	}

	result := db.Exec(
		`INSERT INTO physical_in_flight_backups (database_id, backup_type, backup_id, claimed_at)
		 VALUES (?, ?, ?, NOW())
		 ON CONFLICT (database_id) DO NOTHING`,
		databaseID, backupType, backupID,
	)
	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected > 0, nil
}

func (r *PhysicalInFlightBackupRepository) Release(databaseID uuid.UUID) error {
	return storage.
		GetDb().
		Delete(&physical_models.PhysicalInFlightBackup{}, "database_id = ?", databaseID).Error
}

func (r *PhysicalInFlightBackupRepository) FindByDatabaseID(
	databaseID uuid.UUID,
) (*physical_models.PhysicalInFlightBackup, error) {
	var row physical_models.PhysicalInFlightBackup

	if err := storage.GetDb().Where("database_id = ?", databaseID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &row, nil
}
