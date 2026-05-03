package tools

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// withExeOnWindows appends ".exe" when running on Windows.
func withExeOnWindows(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}

	return name
}

// verifyBinDir checks that binDir exists and contains every command in
// requiredCommands. Logs at the requested verbosity. Exits the process if
// isFatal is true and anything is missing; otherwise warns and continues.
func verifyBinDir(
	logger *slog.Logger,
	dbName, versionLabel, binDir string,
	requiredCommands []string,
	isFatal, isShowLogs bool,
) {
	log := logger.With("db", dbName, "version", versionLabel, "path", binDir)

	if isShowLogs {
		log.Info("verifying client tools installation")
	}

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		if isFatal {
			log.Error("client tools bin directory not found")
			os.Exit(1)
		}

		log.Warn("client tools bin directory not found - support disabled")
		return
	}

	for _, cmd := range requiredCommands {
		cmdPath := filepath.Join(binDir, withExeOnWindows(cmd))
		if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
			if isFatal {
				log.Error("client command not found", "command", cmd, "command_path", cmdPath)
				os.Exit(1)
			}

			log.Warn("client command not found - support disabled",
				"command", cmd, "command_path", cmdPath)
			continue
		}

		if isShowLogs {
			log.Info("client command found", "command", cmd)
		}
	}
}

// VerifyAllClientTools runs the per-DB verifications. Postgres is fatal;
// MySQL/MariaDB/MongoDB are non-fatal (warn and continue) so the app stays
// up when only some clients are present.
func VerifyAllClientTools(logger *slog.Logger, isShowLogs bool) {
	VerifyPostgresqlInstallation(logger, isShowLogs)
	VerifyMysqlInstallation(logger, isShowLogs)
	VerifyMariadbInstallation(logger, isShowLogs)
	VerifyMongodbInstallation(logger, isShowLogs)
}
