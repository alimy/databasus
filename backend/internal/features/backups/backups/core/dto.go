package backups_core

import "time"

type BackupFilters struct {
	Statuses   []BackupStatus
	BeforeDate *time.Time
}
