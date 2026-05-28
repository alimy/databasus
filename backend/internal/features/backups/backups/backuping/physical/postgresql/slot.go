package backuping_physical_postgresql

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"databasus-backend/internal/util/walmath"
)

// ErrSlotRebuildNotImplemented gates the Rebuild call site. Full 7-step body
// lands in PR 4 along with the WAL stream supervisor. PR 2 only ships the
// loop-protection scaffold so PR 4's diff stays focused on the rebuild
// sequence itself.
var ErrSlotRebuildNotImplemented = errors.New(
	"slot.Rebuild is implemented in PR 4 (WAL stream supervisor)",
)

// SlotState captures the snapshot of one physical replication slot. Sourced
// from pg_replication_slots; lag_bytes is computed against pg_current_wal_lsn().
type SlotState struct {
	SlotName   string
	Active     bool
	ActivePID  *int
	WalStatus  string
	RestartLSN walmath.LSN
	FlushLSN   walmath.LSN
	LagBytes   int64
}

// InspectSlot reads the row for slotName from pg_replication_slots and
// computes lag_bytes against the cluster's current WAL position. Returns nil
// when the slot does not exist (caller is responsible for the absence
// branch — typically "create slot" or "refuse" depending on context).
func InspectSlot(ctx context.Context, conn *pgx.Conn, slotName string) (*SlotState, error) {
	state := &SlotState{SlotName: slotName}

	var restartLSN, flushLSN walmath.LSN

	err := conn.QueryRow(ctx, `
		SELECT
			s.active,
			s.active_pid,
			COALESCE(s.wal_status, ''),
			COALESCE(s.restart_lsn, '0/0'::pg_lsn),
			COALESCE(s.confirmed_flush_lsn, '0/0'::pg_lsn),
			COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), s.restart_lsn), 0)::bigint
		FROM pg_replication_slots s
		WHERE s.slot_name = $1
	`, slotName).Scan(
		&state.Active,
		&state.ActivePID,
		&state.WalStatus,
		&restartLSN,
		&flushLSN,
		&state.LagBytes,
	)

	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil

	default:
		return nil, err
	}

	state.RestartLSN = restartLSN
	state.FlushLSN = flushLSN

	return state, nil
}

// SlotRebuilder is the entry point for the WAL stream supervisor's
// slot-rebuild flow. The body of Rebuild() is intentionally a PR 4
// fill-in (7-step sequence — stop supervisor → inspect → terminate stuck
// PID → drop slot → recreate → pg_basebackup with --slot=persistent →
// restart supervisor) plus the per-DB rebuild-attempt window for loop
// protection.
type SlotRebuilder struct{}

func NewSlotRebuilder() *SlotRebuilder {
	return &SlotRebuilder{}
}

// Rebuild is the stub: returns ErrSlotRebuildNotImplemented unconditionally.
func (r *SlotRebuilder) Rebuild(_ context.Context, _ uuid.UUID) error {
	return ErrSlotRebuildNotImplemented
}
