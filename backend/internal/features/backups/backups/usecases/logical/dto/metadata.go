package usecases_logical_dto

import (
	"errors"

	"github.com/google/uuid"

	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
)

type BackupMetadata struct {
	BackupID       uuid.UUID                               `json:"backupId"`
	EncryptionSalt *string                                 `json:"encryptionSalt"`
	EncryptionIV   *string                                 `json:"encryptionIV"`
	Encryption     backups_config_logical.BackupEncryption `json:"encryption"`
}

func (m *BackupMetadata) Validate() error {
	if m.BackupID == uuid.Nil {
		return errors.New("backup ID is required")
	}

	if m.Encryption == "" {
		return errors.New("encryption is required")
	}

	if m.Encryption == backups_config_logical.BackupEncryptionEncrypted {
		if m.EncryptionSalt == nil {
			return errors.New("encryption salt is required when encryption is enabled")
		}

		if m.EncryptionIV == nil {
			return errors.New("encryption IV is required when encryption is enabled")
		}
	}

	return nil
}
