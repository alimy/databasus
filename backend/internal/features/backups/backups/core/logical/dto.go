package backups_core_logical

import "time"

type BackupFilters struct {
	Statuses   []BackupStatus
	BeforeDate *time.Time
}
