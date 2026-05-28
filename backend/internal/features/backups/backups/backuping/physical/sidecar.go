package backuping_physical

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	postgresql_executor "databasus-backend/internal/features/backups/backups/backuping/physical/postgresql"
	physical_dto "databasus-backend/internal/features/backups/backups/core/physical/dto"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	"databasus-backend/internal/features/storages"
	"databasus-backend/internal/util/tools"
)

func (n *PhysicalBackuperNode) uploadFullSidecar(
	logger *slog.Logger,
	sourceDB *postgresql_physical.PostgresqlPhysicalDatabase,
	backup *physical_models.PhysicalFullBackup,
	result postgresql_executor.FullResult,
	storage *storages.Storage,
) error {
	if result.FileName == "" {
		return fmt.Errorf("cannot upload sidecar: file_name is empty")
	}

	sysID := parseSystemIdentifier(sourceDB.SystemIdentifier)

	sidecar := physical_dto.PhysicalBackupMetadata{
		BackupID:              backup.ID,
		DatabaseID:            backup.DatabaseID,
		BackupType:            physical_enums.PhysicalBackupTypeFull,
		SystemIdentifier:      sysID,
		PgVersion:             pgVersionFromTag(sourceDB.Version),
		TimelineID:            result.TimelineID,
		StartLSN:              result.StartLSN.String(),
		StopLSN:               result.StopLSN.String(),
		UncompressedSizeBytes: 0,
		CompressedSizeBytes:   int64(result.BackupSizeMb * 1024 * 1024),
		Encryption:            result.EncryptionAlgo,
		EncryptionSalt:        result.EncryptionSalt,
		EncryptionIV:          result.EncryptionIV,
		CreatedAt:             backup.CreatedAt,
		CompletedAt:           result.CompletedAt,
	}

	return uploadSidecarJSON(logger, storage, n.fieldEncryptorAccessor(), result.FileName, sidecar)
}

func (n *PhysicalBackuperNode) uploadIncrSidecar(
	logger *slog.Logger,
	sourceDB *postgresql_physical.PostgresqlPhysicalDatabase,
	backup *physical_models.PhysicalIncrementalBackup,
	result postgresql_executor.IncrResult,
	storage *storages.Storage,
) error {
	if result.FileName == "" {
		return fmt.Errorf("cannot upload sidecar: file_name is empty")
	}

	sysID := parseSystemIdentifier(sourceDB.SystemIdentifier)

	rootID := backup.RootFullBackupID

	sidecar := physical_dto.PhysicalBackupMetadata{
		BackupID:                  backup.ID,
		DatabaseID:                backup.DatabaseID,
		BackupType:                physical_enums.PhysicalBackupTypeIncremental,
		SystemIdentifier:          sysID,
		PgVersion:                 pgVersionFromTag(sourceDB.Version),
		TimelineID:                result.TimelineID,
		StartLSN:                  result.StartLSN.String(),
		StopLSN:                   result.StopLSN.String(),
		UncompressedSizeBytes:     0,
		CompressedSizeBytes:       int64(result.BackupSizeMb * 1024 * 1024),
		RootFullBackupID:          &rootID,
		ParentIncrementalBackupID: backup.ParentIncrementalBackupID,
		Encryption:                result.EncryptionAlgo,
		EncryptionSalt:            result.EncryptionSalt,
		EncryptionIV:              result.EncryptionIV,
		CreatedAt:                 backup.CreatedAt,
		CompletedAt:               result.CompletedAt,
	}

	return uploadSidecarJSON(logger, storage, n.fieldEncryptorAccessor(), result.FileName, sidecar)
}

func uploadSidecarJSON(
	logger *slog.Logger,
	storage *storages.Storage,
	fieldEncryptor fieldEncryptorReader,
	artifactFileName string,
	sidecar physical_dto.PhysicalBackupMetadata,
) error {
	body, err := json.Marshal(sidecar)
	if err != nil {
		return fmt.Errorf("marshal sidecar JSON: %w", err)
	}

	sidecarName := artifactFileName + ".metadata"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := storage.SaveFile(ctx, fieldEncryptor, logger, sidecarName, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("upload sidecar: %w", err)
	}

	return nil
}

func parseSystemIdentifier(s *string) uint64 {
	if s == nil {
		return 0
	}

	v, err := strconv.ParseUint(*s, 10, 64)
	if err != nil {
		return 0
	}

	return v
}

// pgVersionFromTag converts the tools.PostgresqlVersion enum ("17", "18")
// into the canonical server_version_num style (170000, 180000) so the
// sidecar carries a value pg_combinebackup can compare against on restore.
// The exact patch level is unknown at this layer — major.minor is enough
// for the compatibility check.
func pgVersionFromTag(v tools.PostgresqlVersion) int {
	major, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0
	}

	return major * 10000
}

// fieldEncryptorReader keeps the sidecar uploader decoupled from the full
// util_encryption package surface — the helper only ever needs to pass an
// encryptor down to storage.SaveFile, which takes the interface type.
type fieldEncryptorReader interface {
	Encrypt(value string) (string, error)
	Decrypt(value string) (string, error)
}

func (n *PhysicalBackuperNode) fieldEncryptorAccessor() fieldEncryptorReader {
	return n.fieldEncryptor
}
