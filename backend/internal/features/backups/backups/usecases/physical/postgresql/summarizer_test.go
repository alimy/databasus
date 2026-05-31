package usecases_physical_postgresql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	"databasus-backend/internal/features/databases"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/walmath"
)

// Test_CheckSummarizerReadiness_WhenSummarizerOff_FallsBackToFullNewChain proves
// the first WAL-gap fallback: with summarize_wal off the source can never produce
// the summaries an INCR needs, so the readiness check must steer to a fresh FULL
// (new chain) and tag it SUMMARIZER_OFF rather than attempting a doomed INCR.
//
// It targets the dedicated no-summary cluster: summarize_wal is off at the
// postmaster level there, which is the only way to reach this branch — ALTER
// SYSTEM cannot override a command-line GUC on the standard fixture.
func Test_CheckSummarizerReadiness_WhenSummarizerOff_FallsBackToFullNewChain(t *testing.T) {
	sourceDB := databases.GetTestPhysicalPostgresConfigNoSummary("17")

	ctx := context.Background()

	conn, err := sourceDB.OpenInspectionConn(ctx, encryption.GetFieldEncryptor())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	enabled, err := isSummarizerEnabled(ctx, conn)
	require.NoError(t, err)
	require.False(t, enabled, "the no-summary cluster must report summarize_wal off")

	result, err := CheckSummarizerReadiness(ctx, conn, walmath.LSN(0), time.Hour)
	require.NoError(t, err)

	require.Equal(t, DecisionFullNewChain, result.Decision)
	require.NotNil(t, result.Reason)
	require.Equal(t, physical_enums.PhysicalBackupErrorSummarizerOff, *result.Reason)
}

// Test_CheckSummarizerReadiness_WhenSummariesDoNotCoverTarget_FallsBackToFullNewChain
// covers the second WAL-gap fallback: the summarizer is on but no summary covers
// the parent's stop LSN (the real-world cause is summary-file expiry). An LSN that
// predates every available summary is, by definition, uncovered — so the check
// must steer to a fresh FULL and tag it SUMMARIES_EXPIRED. ExpireWalSummaries is
// applied first so the path mirrors production expiry rather than a synthetic LSN
// alone.
func Test_CheckSummarizerReadiness_WhenSummariesDoNotCoverTarget_FallsBackToFullNewChain(t *testing.T) {
	fixture := SetupPhysicalDBForBackup(t)
	conn := OpenAdminConn(t, fixture)

	ExpireWalSummaries(t, conn)

	enabled, err := isSummarizerEnabled(context.Background(), conn)
	require.NoError(t, err)
	require.True(t, enabled, "the standard fixture runs with summarize_wal on")

	// LSN 0/1 sits before any summary the running cluster can hold, so coverage
	// is guaranteed false regardless of where the summarizer currently starts.
	uncoveredTargetLSN := walmath.LSN(1)

	result, err := CheckSummarizerReadiness(context.Background(), conn, uncoveredTargetLSN, time.Hour)
	require.NoError(t, err)

	require.Equal(t, DecisionFullNewChain, result.Decision)
	require.NotNil(t, result.Reason)
	require.Equal(t, physical_enums.PhysicalBackupErrorSummariesExpired, *result.Reason)
}
