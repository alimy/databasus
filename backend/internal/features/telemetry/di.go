package telemetry

import (
	"sync"

	"databasus-backend/internal/config"
	backups_services "databasus-backend/internal/features/backups/backups/services"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/features/notifiers"
	"databasus-backend/internal/features/storages"
	system_version "databasus-backend/internal/features/system/version"
	verification_agents "databasus-backend/internal/features/verification/agents"
	verification_config "databasus-backend/internal/features/verification/config"
	"databasus-backend/internal/util/logger"
)

const productionEndpoint = "https://metrics.databasus.com/api/anonymous/collect"

var (
	telemetryLogger = logger.GetLogger()

	instanceLoader = NewInstanceFileLoader("", telemetryLogger)
	httpSender     = NewHTTPTelemetrySender(productionEndpoint, system_version.GetAppVersion())

	telemetryService = NewTelemetryService(
		instanceLoader,
		httpSender,
		databases.GetDatabaseService(),
		storages.GetStorageService(),
		notifiers.GetNotifierService(),
		backups_services.GetBackupService(),
		verification_agents.GetAgentService(),
		verification_config.GetVerificationConfigService(),
		system_version.GetAppVersion(),
		telemetryLogger,
	)

	telemetryBackgroundService = NewTelemetryBackgroundService(
		telemetryService,
		telemetryLogger,
	)
)

func GetTelemetryService() *TelemetryService {
	return telemetryService
}

func GetTelemetryBackgroundService() *TelemetryBackgroundService {
	return telemetryBackgroundService
}

var SetupDependencies = sync.OnceFunc(func() {
	instanceLoader.path = config.GetEnv().TelemetryInstancePath
})
