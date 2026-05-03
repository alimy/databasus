package tools

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

type MysqlVersion string

const (
	MysqlVersion57 MysqlVersion = "5.7"
	MysqlVersion80 MysqlVersion = "8.0"
	MysqlVersion84 MysqlVersion = "8.4"
	MysqlVersion9  MysqlVersion = "9"
)

type MysqlExecutable string

const (
	MysqlExecutableMysqldump MysqlExecutable = "mysqldump"
	MysqlExecutableMysql     MysqlExecutable = "mysql"
)

// GetMysqlExecutable returns the absolute path to a MySQL client binary for
// the given version (mysqldump or mysql).
func GetMysqlExecutable(version MysqlVersion, executable MysqlExecutable) string {
	return filepath.Join(getMysqlBinDir(version), withExeOnWindows(string(executable)))
}

func getMysqlBinDir(version MysqlVersion) string {
	return filepath.Join(
		AssetsToolsDir(),
		"mysql",
		fmt.Sprintf("mysql-%s", version),
		"bin",
	)
}

// VerifyMysqlInstallation is non-fatal — MySQL support is optional, so a
// missing version warns rather than exiting.
func VerifyMysqlInstallation(logger *slog.Logger, isShowLogs bool) {
	versions := []MysqlVersion{
		MysqlVersion57,
		MysqlVersion80,
		MysqlVersion84,
		MysqlVersion9,
	}

	required := []string{
		string(MysqlExecutableMysqldump),
		string(MysqlExecutableMysql),
	}

	for _, v := range versions {
		verifyBinDir(logger, "mysql", string(v), getMysqlBinDir(v),
			required, false, isShowLogs)
	}
}

// IsMysqlBackupVersionHigherThanRestoreVersion reports whether a backup
// produced on backupVersion would be downgrade-restoring onto restoreVersion.
func IsMysqlBackupVersionHigherThanRestoreVersion(
	backupVersion, restoreVersion MysqlVersion,
) bool {
	versionOrder := map[MysqlVersion]int{
		MysqlVersion57: 1,
		MysqlVersion80: 2,
		MysqlVersion84: 3,
		MysqlVersion9:  4,
	}
	return versionOrder[backupVersion] > versionOrder[restoreVersion]
}

// EscapeMysqlPassword escapes special characters for the MySQL .my.cnf file
// format (passwords with special chars are double-quoted).
func EscapeMysqlPassword(password string) string {
	password = strings.ReplaceAll(password, "\\", "\\\\")
	password = strings.ReplaceAll(password, "\"", "\\\"")
	return password
}

func GetMysqlVersionEnum(version string) MysqlVersion {
	switch version {
	case "5.7":
		return MysqlVersion57
	case "8.0":
		return MysqlVersion80
	case "8.4":
		return MysqlVersion84
	case "9":
		return MysqlVersion9
	default:
		panic(fmt.Sprintf("invalid mysql version: %s", version))
	}
}
