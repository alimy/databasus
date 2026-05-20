package system_healthcheck

import (
	"databasus-backend/internal/features/backups/backups/backuping"
	"databasus-backend/internal/features/disk"
	verification_agents "databasus-backend/internal/features/verification/agents"
)

var healthcheckService = &HealthcheckService{
	disk.GetDiskService(),
	backuping.GetBackupsScheduler(),
	backuping.GetBackuperNode(),
	verification_agents.GetAgentService(),
}

var healthcheckController = &HealthcheckController{
	healthcheckService,
}

func GetHealthcheckController() *HealthcheckController {
	return healthcheckController
}
