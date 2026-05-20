package verification_runs

import (
	"math"

	backups_core "databasus-backend/internal/features/backups/backups/core"
)

// diskCostPerJobGapMb is the per-job safe gap added on top of the downloaded
// backup file and the restored database. Covers WAL, indexes, sort/temp spills,
// FS slack and small fixed costs that don't scale with backup size.
const diskCostPerJobGapMb = 1024

// IsVerificationFitWithinRemainedDiskCapacity reports whether
// running candidateBackup on this agent alongside runningBackups
// stays within the agent's declared disk capacity.
func IsVerificationFitWithinRemainedDiskCapacity(
	capacity AgentCapacity,
	runningBackups []*backups_core.Backup,
	candidateBackup *backups_core.Backup,
) bool {
	if capacity.MaxDiskGb <= 0 || candidateBackup == nil {
		return false
	}

	totalBudgetMb := int64(capacity.MaxDiskGb) * 1024
	usedMb := sumEstimatedRequiredDiskMb(runningBackups)
	candidateCostMb := EstimateRequiredForRestoreDiskMb(candidateBackup)

	return usedMb+candidateCostMb <= totalBudgetMb
}

// EstimateRequiredForRestoreDiskMb estimates expected job's on-disk cost.
//
// Includes:
// - Space needed for backup file (if archived - decompressed on the fly while streaming)
// - Space needed for restored database
// - Safe gap (WAL, indexes, sort/temp spills, FS slack)
func EstimateRequiredForRestoreDiskMb(backup *backups_core.Backup) int64 {
	archiveSizeMb := backup.BackupSizeMb
	if archiveSizeMb < 0 {
		archiveSizeMb = 0
	}

	restoredSizeMb := backup.BackupRawDbSizeMb
	if restoredSizeMb < 0 {
		restoredSizeMb = 0
	}

	return int64(math.Ceil(archiveSizeMb+restoredSizeMb)) + diskCostPerJobGapMb
}

func sumEstimatedRequiredDiskMb(backups []*backups_core.Backup) int64 {
	var total int64
	for _, backup := range backups {
		total += EstimateRequiredForRestoreDiskMb(backup)
	}

	return total
}
