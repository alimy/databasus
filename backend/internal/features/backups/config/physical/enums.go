package backups_config_physical

type WalFallbackStrategy string

const (
	WalFallbackStrategyIncrementalWithLoss WalFallbackStrategy = "INCREMENTAL_WITH_LOSS"
	WalFallbackStrategyForceFull           WalFallbackStrategy = "FORCE_FULL"
)

type BackupNotificationType string

const (
	NotificationBackupSuccess BackupNotificationType = "BACKUP_SUCCESS"
	NotificationBackupFailed  BackupNotificationType = "BACKUP_FAILED"
	NotificationChainBroken   BackupNotificationType = "CHAIN_BROKEN"
)
