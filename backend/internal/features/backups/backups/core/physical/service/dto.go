package physical_service

import "github.com/google/uuid"

// DeletedSummary reports what a single DeleteFull / DeleteChainDependentsKeepFull
// call removed. The cleaner logs it per tick. ChainFullyDeleted is false when
// the per-tick WAL byte budget capped the call before the FULL row was reached
// (or when the call intentionally keeps the FULL) — the caller resumes on the
// next tick.
type DeletedSummary struct {
	RootFullBackupID  uuid.UUID
	WalSegments       int
	Incrementals      int
	HistoryFiles      int
	BytesDeletedMB    float64
	ChainFullyDeleted bool
}

// DependentsSummary counts a chain's dependents and total on-disk size without
// deleting anything. Powers "what would DeleteFull remove?" UI/audit and the
// billing pass's pre-delete accounting.
type DependentsSummary struct {
	RootFullBackupID uuid.UUID
	WalSegments      int
	Incrementals     int
	HistoryFiles     int
	TotalSizeMB      float64
}
