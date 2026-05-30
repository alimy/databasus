// Resets state before `make test` runs. Two distinct resets:
//
//  1. The Databasus metadata DB: drop and recreate the test schema referenced
//     by TEST_DATABASE_DSN so Goose migrations apply against a clean slate.
//
//  2. Each test PostgreSQL container (versions 12-18): drop every non-system
//     database (leftover backups, restores, and `testdb` itself), recreate
//     `testdb`, and drop every non-system role. This is what makes containers
//     usable across runs - tests like CreateReadOnlyUser issue
//     `REVOKE CREATE ON SCHEMA public FROM PUBLIC` and create per-test roles
//     that otherwise persist across runs and break subsequent pg_dump calls
//     with "permission denied for schema public".
//
// Reads the test DSN through config (config.GetEnv() auto-swaps DatabaseDsn to
// TestDatabaseDsn when IsTesting is true). IsTesting is detected from os.Args
// containing the substring "test" - the binary path "cleanup_test_db" satisfies
// that. Renaming the binary or its directory will break detection.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	"databasus-backend/internal/config"
	cache_utils "databasus-backend/internal/util/cache"
	"databasus-backend/internal/util/logger"
)

const (
	systemDb                = "postgres"
	testPgContainerUser     = "testuser"
	testPgContainerPassword = "testpassword"
	testPgContainerDb       = "testdb"
)

func main() {
	log := logger.GetLogger()
	ctx := context.Background()

	env := config.GetEnv()
	if !env.IsTesting {
		log.Error("cleanup_test_db must run with IsTesting=true (binary name must contain 'test')")
		os.Exit(1)
	}

	if err := resetValkey(log); err != nil {
		log.Error("failed to reset Valkey", "error", err)
		os.Exit(1)
	}

	if err := resetMetadataDb(log, env); err != nil {
		log.Error("failed to reset metadata DB", "error", err)
		os.Exit(1)
	}

	if err := resetTestPostgresContainers(ctx, log, env); err != nil {
		log.Error("failed to reset test PG containers", "error", err)
		os.Exit(1)
	}
}

// resetValkey wipes every key so each `make test` starts from a clean cache.
// Without this, a -failfast'd previous run can leave backup:node:* counters,
// in-flight backup claims, and other state that breaks the next run's
// scheduler/registry assumptions.
func resetValkey(log *slog.Logger) error {
	log.Info("resetting Valkey")

	if err := cache_utils.ClearAllCache(); err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}

	log.Info("Valkey reset complete")
	return nil
}

func resetMetadataDb(log *slog.Logger, env *config.EnvVariables) error {
	targetDbName, systemDsn, err := rewriteDbName(env.DatabaseDsn, systemDb)
	if err != nil {
		return fmt.Errorf("parse TEST_DATABASE_DSN: %w", err)
	}

	log.Info("resetting metadata DB", "db", targetDbName)

	db, err := gorm.Open(postgres.Open(systemDsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("connect to system postgres DB: %w", err)
	}

	quoted := quoteIdentifier(targetDbName)

	if err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoted)).Error; err != nil {
		return fmt.Errorf("drop database %s: %w", targetDbName, err)
	}

	if err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", quoted)).Error; err != nil {
		return fmt.Errorf("create database %s: %w", targetDbName, err)
	}

	log.Info("metadata DB reset complete", "db", targetDbName)
	return nil
}

func resetTestPostgresContainers(ctx context.Context, log *slog.Logger, env *config.EnvVariables) error {
	containers := []struct {
		label string
		port  string
	}{
		{"logical-12", env.TestLogicalPostgres12Port},
		{"logical-13", env.TestLogicalPostgres13Port},
		{"logical-14", env.TestLogicalPostgres14Port},
		{"logical-15", env.TestLogicalPostgres15Port},
		{"logical-16", env.TestLogicalPostgres16Port},
		{"logical-17", env.TestLogicalPostgres17Port},
		{"logical-18", env.TestLogicalPostgres18Port},
		{"physical-17", env.TestPhysicalPostgres17Port},
		{"physical-18", env.TestPhysicalPostgres18Port},
		{"physical-17-no-summary", env.TestPhysicalPostgres17NoSummaryPort},
		{"physical-18-no-summary", env.TestPhysicalPostgres18NoSummaryPort},
		{"physical-17-tablespace", env.TestPhysicalPostgres17TablespacePort},
		{"physical-18-tablespace", env.TestPhysicalPostgres18TablespacePort},
	}

	for _, c := range containers {
		if c.port == "" {
			continue
		}

		if err := resetOnePostgresContainer(ctx, log, env.TestLocalhost, c.label, c.port); err != nil {
			return fmt.Errorf("PG %s on %s:%s: %w", c.label, env.TestLocalhost, c.port, err)
		}
	}

	return nil
}

func resetOnePostgresContainer(ctx context.Context, log *slog.Logger, host, label, port string) error {
	log = log.With("pg_label", label, "port", port)

	systemDsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, testPgContainerUser, testPgContainerPassword, systemDb,
	)

	db, err := sql.Open("postgres", systemDsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	if err := dropDatabasusReplicationSlots(ctx, log, db); err != nil {
		return fmt.Errorf("drop databasus replication slots: %w", err)
	}

	userDbs, err := listUserDatabases(ctx, db)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
	}

	for _, name := range userDbs {
		if _, err := db.ExecContext(ctx, fmt.Sprintf(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid()",
			quoteLiteral(name),
		)); err != nil {
			log.Warn("could not terminate connections to DB", "db", name, "error", err)
		}

		// No WITH (FORCE) - that's PG 13+ only and we support PG 12.
		// pg_terminate_backend above already kicked off other sessions.
		if _, err := db.ExecContext(
			ctx,
			fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(name)),
		); err != nil {
			return fmt.Errorf("drop database %s: %w", name, err)
		}
	}

	if _, err := db.ExecContext(
		ctx,
		fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(testPgContainerDb)),
	); err != nil {
		return fmt.Errorf("create database %s: %w", testPgContainerDb, err)
	}

	userRoles, err := listUserRoles(ctx, db)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	owner := quoteIdentifier(testPgContainerUser)
	for _, role := range userRoles {
		quotedRole := quoteIdentifier(role)
		// REASSIGN/DROP OWNED release any objects still pinned to the role so
		// DROP ROLE can succeed. Failures here are best-effort - the role may
		// own nothing, in which case these are no-ops.
		_, _ = db.ExecContext(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO %s", quotedRole, owner))
		_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP OWNED BY %s", quotedRole))

		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP ROLE IF EXISTS %s", quotedRole)); err != nil {
			log.Warn("could not drop role", "role", role, "error", err)
		}
	}

	log.Info("test PG container reset complete")
	return nil
}

// dropDatabasusReplicationSlots removes every replication slot whose name
// starts with the databasus prefixes. Idempotent: a missing slot returns
// pgx.ErrNoRows from the subquery and the SELECT yields no rows.
func dropDatabasusReplicationSlots(ctx context.Context, log *slog.Logger, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT slot_name FROM pg_replication_slots
		WHERE slot_name LIKE 'databasus_basebackup_%'
		   OR slot_name LIKE 'databasus_slot_%'
	`)
	if err != nil {
		return fmt.Errorf("list slots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var slotNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan slot name: %w", err)
		}

		slotNames = append(slotNames, name)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range slotNames {
		// pg_drop_replication_slot fails if the slot is "active". For tests
		// we want a hard reset, so terminate any active session on the slot
		// first; ignore errors because slot may have just been released.
		_, _ = db.ExecContext(ctx,
			"SELECT pg_terminate_backend(active_pid) FROM pg_replication_slots WHERE slot_name = $1 AND active",
			name,
		)

		if _, err := db.ExecContext(ctx, "SELECT pg_drop_replication_slot($1)", name); err != nil {
			log.Warn("could not drop replication slot", "slot_name", name, "error", err)
			continue
		}

		log.Info("dropped leftover replication slot", "slot_name", name)
	}

	return nil
}

func listUserDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT datname FROM pg_database
		WHERE NOT datistemplate
		  AND datname <> $1
	`, systemDb)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

func listUserRoles(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT rolname FROM pg_roles
		WHERE rolname NOT IN ($1, $2)
		  AND rolname NOT LIKE 'pg\_%' ESCAPE '\'
	`, systemDb, testPgContainerUser)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, rows.Err()
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

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
