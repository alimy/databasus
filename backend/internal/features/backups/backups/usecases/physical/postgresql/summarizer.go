package usecases_physical_postgresql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	"databasus-backend/internal/util/walmath"
)

// SummarizerDecision encodes the outcome of a per-tick incremental pre-check.
// The scheduler in PR 3 maps each value to one of: spawn INCR, wait then
// recheck, spawn FULL on same chain, or spawn FULL anchoring a new chain.
type SummarizerDecision int

const (
	DecisionGoIncremental SummarizerDecision = iota
	DecisionWait
	DecisionFullSameChain
	DecisionFullNewChain
)

// SummarizerResult carries the decision plus the inputs the caller needs to
// act on it: wait/poll cadence for DecisionWait, error_reason for the
// chain-killing DecisionFullNewChain branches.
type SummarizerResult struct {
	Decision  SummarizerDecision
	WaitFor   time.Duration
	PollEvery time.Duration
	Reason    *physical_enums.PhysicalBackupErrorReason
}

const (
	// summarizerWaitPollInterval — how often the scheduler in PR 3 will poll
	// after DecisionWait. Five seconds is tight enough that recovery is felt
	// quickly, loose enough that we don't hammer pg_available_wal_summaries.
	summarizerWaitPollInterval = 5 * time.Second

	// summarizerWaitCap — DecisionWait timeout is min(cadence/4, this).
	summarizerWaitCap = 30 * time.Minute

	// summarizerLagThresholdBytes — if the summarizer is falling behind by
	// more than this much WAL between the two samples, we don't bother
	// waiting: spawn FULL on the same chain (no chain-break). 256 MB ≈ 16
	// segments at the default segsize — a clearly degraded summarizer.
	summarizerLagThresholdBytes int64 = 256 * 1024 * 1024
)

// CheckSummarizerReadiness classifies the state of the WAL summarizer relative to
// prevStopLSN (the parent backup's stop_lsn, or — for the WAL-gap fallback
// path — the current LSN). The conn must be an ordinary, non-replication
// connection.
func CheckSummarizerReadiness(
	ctx context.Context,
	conn *pgx.Conn,
	prevStopLSN walmath.LSN,
	incrementalCadence time.Duration,
) (SummarizerResult, error) {
	enabled, err := isSummarizerEnabled(ctx, conn)
	if err != nil {
		return SummarizerResult{}, err
	}

	if !enabled {
		reason := physical_enums.PhysicalBackupErrorSummarizerOff

		return SummarizerResult{
			Decision: DecisionFullNewChain,
			Reason:   &reason,
		}, nil
	}

	covered, err := summarizerCoversLSN(ctx, conn, prevStopLSN)
	if err != nil {
		return SummarizerResult{}, err
	}

	if !covered {
		reason := physical_enums.PhysicalBackupErrorSummariesExpired

		return SummarizerResult{
			Decision: DecisionFullNewChain,
			Reason:   &reason,
		}, nil
	}

	lag, err := measureSummarizerLag(ctx, conn)
	if err != nil {
		return SummarizerResult{}, err
	}

	if lag >= summarizerLagThresholdBytes {
		return SummarizerResult{Decision: DecisionFullSameChain}, nil
	}

	if lag > 0 {
		return SummarizerResult{
			Decision:  DecisionWait,
			WaitFor:   waitWindowForCadence(incrementalCadence),
			PollEvery: summarizerWaitPollInterval,
		}, nil
	}

	return SummarizerResult{Decision: DecisionGoIncremental}, nil
}

func isSummarizerEnabled(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var setting string

	if err := conn.QueryRow(ctx,
		"SELECT setting FROM pg_settings WHERE name = 'summarize_wal'",
	).Scan(&setting); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("read summarize_wal setting: %w", err)
	}

	return setting == "on", nil
}

// summarizerCoversLSN returns true when pg_available_wal_summaries reports a
// summary that contains lsn (start_lsn <= lsn <= end_lsn). Empty result set
// means the LSN has aged out of the kept window — that's
// SUMMARIES_EXPIRED.
func summarizerCoversLSN(ctx context.Context, conn *pgx.Conn, lsn walmath.LSN) (bool, error) {
	var exists bool

	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_available_wal_summaries()
			WHERE start_lsn <= $1::pg_lsn
			  AND end_lsn   >= $1::pg_lsn
		)
	`, lsn.String()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("query pg_available_wal_summaries: %w", err)
	}

	return exists, nil
}

// measureSummarizerLag returns how far the summarizer's coverage trails
// pg_current_wal_lsn. Two samples taken summarizerSampleInterval apart give
// the scheduler the data to distinguish "lagging but catching up" (delta
// shrinking) from "falling behind" (delta growing). For PR 2 we report the
// instantaneous delta only; the rate refinement (delta1 vs delta2) belongs
// to PR 3 where the scheduler maintains the per-DB state.
func measureSummarizerLag(ctx context.Context, conn *pgx.Conn) (int64, error) {
	var lagBytes int64

	err := conn.QueryRow(ctx, `
		SELECT COALESCE(pg_wal_lsn_diff(
			pg_current_wal_lsn(),
			(SELECT MAX(end_lsn) FROM pg_available_wal_summaries())
		), 0)::bigint
	`).Scan(&lagBytes)
	if err != nil {
		return 0, fmt.Errorf("measure summarizer lag: %w", err)
	}

	if lagBytes < 0 {
		return 0, nil
	}

	return lagBytes, nil
}

func waitWindowForCadence(cadence time.Duration) time.Duration {
	quarter := cadence / 4
	if quarter > summarizerWaitCap {
		return summarizerWaitCap
	}

	return quarter
}
