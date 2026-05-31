package backuping_physical

import "time"

const (
	// WHY: scheduler tick + failover sweep run on a 15 s cadence. User-configured
	// FULL/INCR intervals are measured in hours-to-days, so sub-minute polling is
	// ample; the tighter bound comes from failover, where 15 s keeps detect-to-fail
	// under ~2 min after a node dies. Each tick is a handful of cheap indexed
	// queries per enabled DB.
	schedulerTickInterval = 15 * time.Second

	// WHY: in many-nodes mode, wait for peer nodes to register in the backup
	// registry before the first tick so node selection has candidates.
	schedulerStartupDelay = 1 * time.Minute

	// WHY: IsSchedulerRunning() reports unhealthy when no tick completed within
	// this window — mirrors logical's healthcheck threshold.
	schedulerHealthcheckThreshold = 5 * time.Minute

	// WHY: isolates the physical backup-node pool from the logical pool in the
	// shared registry — without it, the logical scheduler could assign a logical
	// backup to a physical node (which would drop it) and vice versa.
	physicalNodePoolNamespace = "physical:"

	// WHY: stable log job_name for the scheduler; never the struct type name (a
	// rename would silently break log queries).
	schedulerJobName = "physical_backup_scheduler"

	// WHY: cleaner ticks every 3 s so retention / storage-cap / billing
	// decisions become visible almost immediately, while the per-tick WAL byte
	// budget keeps a single tick bounded even at this cadence. Mirrors logical.
	cleanerTickInterval = 3 * time.Second

	// WHY: never delete a chain whose end timestamp is younger than
	// max(full, incr cadence) × 2 — protects a chain that just completed or is
	// still being extended from premature retention/billing eviction.
	chainGraceIntervalMultiplier = 2

	// WHY: a WAL row with file_name still NULL past 1 h is an abandoned
	// insert-first claim (the live owner finishes in seconds); reap it so the
	// (database_id, timeline_id, start_lsn) slot is free to re-receive.
	walClaimGracePeriod = 1 * time.Hour

	// WHY: stable log job_name for the cleaner.
	cleanerJobName = "physical_backup_retention_cleanup"
)

// WHY: per-tick WAL deletion budget anchors to the latest FULL's size (a cluster
// producing a 10 GB FULL produces O(10 GB) of WAL between FULLs, so "one FULL's
// worth" is a self-scaling chunk), with a 256 MB floor so clusters with tiny
// FULLs but heavy WAL don't crawl.
const minWalDeleteBudgetMB float64 = 256

// recentBackupGracePeriod mirrors logical's 60-minute floor on individual
// backups — used as a conservative fallback in cleaner grace logic.
const recentBackupGracePeriod = 60 * time.Minute
