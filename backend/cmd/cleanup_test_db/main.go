// Resets the test database before `make test` runs.
//
// Reads the test DSN through config (config.GetEnv() auto-swaps DatabaseDsn to
// TestDatabaseDsn when IsTesting is true). IsTesting is detected from os.Args
// containing the substring "test" — the binary path "cleanup_test_db" satisfies
// that. Renaming the binary or its directory will break detection.
package main

import (
	"fmt"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	"databasus-backend/internal/config"
	"databasus-backend/internal/util/logger"
)

const systemDb = "postgres"

func main() {
	log := logger.GetLogger()

	env := config.GetEnv()
	if !env.IsTesting {
		log.Error("cleanup_test_db must run with IsTesting=true (binary name must contain 'test')")
		os.Exit(1)
	}

	targetDbName, systemDsn, err := rewriteDbName(env.DatabaseDsn, systemDb)
	if err != nil {
		log.Error("failed to parse TEST_DATABASE_DSN", "error", err)
		os.Exit(1)
	}

	log.Info("resetting test DB", "db", targetDbName)

	db, err := gorm.Open(postgres.Open(systemDsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Silent),
	})
	if err != nil {
		log.Error("failed to connect to system postgres DB", "error", err)
		os.Exit(1)
	}

	quoted := quoteIdentifier(targetDbName)

	if err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoted)).Error; err != nil {
		log.Error("DROP DATABASE failed", "error", err)
		os.Exit(1)
	}

	if err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", quoted)).Error; err != nil {
		log.Error("CREATE DATABASE failed", "error", err)
		os.Exit(1)
	}

	log.Info("test DB reset complete", "db", targetDbName)
}

func rewriteDbName(dsn, newDbName string) (origDbName, rewritten string, err error) {
	parts := strings.Fields(dsn)
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return "", "", fmt.Errorf("invalid DSN token: %q", p)
		}

		if k == "dbname" {
			origDbName = v
			out = append(out, "dbname="+newDbName)
			continue
		}

		out = append(out, p)
	}

	if origDbName == "" {
		return "", "", fmt.Errorf("DSN missing dbname: %q", dsn)
	}

	return origDbName, strings.Join(out, " "), nil
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
