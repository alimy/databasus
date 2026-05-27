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

type PhysicalIncrementalBackupRepository struct{}

func (r *PhysicalIncrementalBackupRepository) Save(b *physical_models.PhysicalIncrementalBackup) error {
	if b.DatabaseID == uuid.Nil || b.StorageID == uuid.Nil {
		return errors.New("database ID and storage ID are required")
	}
	if b.RootFullBackupID == uuid.Nil {
		return errors.New("root full backup ID is required")
	}

	db := storage.GetDb()

	if b.ID == uuid.Nil {
		b.ID = uuid.New()
		if b.CreatedAt.IsZero() {
			b.CreatedAt = time.Now().UTC()
		}

		return db.Create(b).Error
	}

	return db.Save(b).Error
}

func (r *PhysicalIncrementalBackupRepository) FindByID(
	id uuid.UUID,
) (*physical_models.PhysicalIncrementalBackup, error) {
	var backup physical_models.PhysicalIncrementalBackup

	if err := storage.GetDb().Where("id = ?", id).First(&backup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *PhysicalIncrementalBackupRepository) FindLatestCompletedByRootFull(
	rootFullBackupID uuid.UUID,
) (*physical_models.PhysicalIncrementalBackup, error) {
	var backup physical_models.PhysicalIncrementalBackup

	err := storage.
		GetDb().
		Where("root_full_backup_id = ? AND status = ?",
			rootFullBackupID, physical_enums.PhysicalBackupStatusCompleted).
		Order("start_lsn DESC").
		First(&backup).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &backup, nil
}

func (r *PhysicalIncrementalBackupRepository) FindAllByRootFull(
	rootFullBackupID uuid.UUID,
) ([]*physical_models.PhysicalIncrementalBackup, error) {
	var backups []*physical_models.PhysicalIncrementalBackup

	if err := storage.
		GetDb().
		Where("root_full_backup_id = ?", rootFullBackupID).
		Order("start_lsn ASC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *PhysicalIncrementalBackupRepository) FindByIncrementalParent(
	parentIncrementalBackupID uuid.UUID,
) ([]*physical_models.PhysicalIncrementalBackup, error) {
	var backups []*physical_models.PhysicalIncrementalBackup

	if err := storage.
		GetDb().
		Where("parent_incremental_backup_id = ?", parentIncrementalBackupID).
		Order("start_lsn ASC").
		Find(&backups).Error; err != nil {
		return nil, err
	}

	return backups, nil
}

func (r *PhysicalIncrementalBackupRepository) UpdateStatus(
	id uuid.UUID,
	status physical_enums.PhysicalBackupStatus,
	errorReason *physical_enums.PhysicalBackupErrorReason,
) error {
	updates := map[string]any{"status": status, "error_reason": errorReason}

	if status == physical_enums.PhysicalBackupStatusCompleted {
		now := time.Now().UTC()
		updates["completed_at"] = &now
	}

	return storage.
		GetDb().
		Model(&physical_models.PhysicalIncrementalBackup{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *PhysicalIncrementalBackupRepository) DeleteByID(id uuid.UUID) error {
	return storage.GetDb().Delete(&physical_models.PhysicalIncrementalBackup{}, "id = ?", id).Error
}
