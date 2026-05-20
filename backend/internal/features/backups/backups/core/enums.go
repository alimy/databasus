package backups_core

type BackupStatus string

const (
	BackupStatusInProgress BackupStatus = "IN_PROGRESS"
	BackupStatusCompleted  BackupStatus = "COMPLETED"
	BackupStatusFailed     BackupStatus = "FAILED"
	BackupStatusCanceled   BackupStatus = "CANCELED"
)

type PgWalUploadType string

const (
	PgWalUploadTypeBasebackup PgWalUploadType = "basebackup"
	PgWalUploadTypeWal        PgWalUploadType = "wal"
)

type RestoreVerificationStatus string

const (
	RestoreVerificationStatusNotVerified        RestoreVerificationStatus = "NOT_VERIFIED"
	RestoreVerificationStatusVerifiedSuccessful RestoreVerificationStatus = "VERIFIED_SUCCESSFUL"
	RestoreVerificationStatusVerificationFailed RestoreVerificationStatus = "VERIFICATION_FAILED"
)
