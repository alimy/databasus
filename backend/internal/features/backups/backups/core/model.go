package backups_core

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	backups_config "databasus-backend/internal/features/backups/config"
	files_utils "databasus-backend/internal/util/files"
)

type Backup struct {
	ID       uuid.UUID `json:"id"       gorm:"column:id;type:uuid;primaryKey"`
	FileName string    `json:"fileName" gorm:"column:file_name;type:text;not null"`

	DatabaseID uuid.UUID `json:"databaseId" gorm:"column:database_id;type:uuid;not null"`
	StorageID  uuid.UUID `json:"storageId"  gorm:"column:storage_id;type:uuid;not null"`

	Status      BackupStatus `json:"status"      gorm:"column:status;not null"`
	FailMessage *string      `json:"failMessage" gorm:"column:fail_message"`
	IsSkipRetry bool         `json:"isSkipRetry" gorm:"column:is_skip_retry;type:boolean;not null"`

	RestoreVerificationStatus RestoreVerificationStatus `json:"restoreVerificationStatus" gorm:"column:restore_verification_status;type:text;not null;default:'NOT_VERIFIED'"`

	BackupSizeMb      float64 `json:"backupSizeMb"      gorm:"column:backup_size_mb;default:0"`
	BackupRawDbSizeMb float64 `json:"backupRawDbSizeMb" gorm:"column:backup_raw_db_size_mb;default:0"`

	BackupDurationMs int64 `json:"backupDurationMs" gorm:"column:backup_duration_ms;default:0"`

	EncryptionSalt *string                         `json:"-"          gorm:"column:encryption_salt"`
	EncryptionIV   *string                         `json:"-"          gorm:"column:encryption_iv"`
	Encryption     backups_config.BackupEncryption `json:"encryption" gorm:"column:encryption;type:text;not null;default:'NONE'"`

	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at"`
}

func (b *Backup) GenerateFilename(dbName string) {
	timestamp := time.Now().UTC()

	b.FileName = fmt.Sprintf(
		"%s-%s-%s",
		files_utils.SanitizeFilename(dbName),
		timestamp.Format("20060102-150405"),
		b.ID.String(),
	)
}
