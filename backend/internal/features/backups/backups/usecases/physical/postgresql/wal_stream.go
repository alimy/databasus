package usecases_physical_postgresql

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	backups_core_enums "databasus-backend/internal/features/backups/backups/core/enums"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	postgresql_shared "databasus-backend/internal/features/databases/databases/postgresql/shared"
	"databasus-backend/internal/features/storages"
	util_encryption "databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/tools"
	"databasus-backend/internal/util/walmath"
)

const (
	receivewalApplicationNamePrefix = "databasus_wal_receiver_"

	// uploaderPollInterval — the uploader scans watch_dir this often for newly
	// rotated segments. Segments rotate at the source write rate; a tight loop
	// keeps local dwell time low without measurable CPU.
	uploaderPollInterval = 1 * time.Second

	// slotLsnWatcherPollInterval — consumer-side liveness poll. Tighter than the
	// lag monitor because it tests whether OUR pg_receivewal is actually flushing.
	slotLsnWatcherPollInterval = 10 * time.Second

	// slotLsnStallTimeout — restart_lsn unchanged this long on a healthy PG means
	// pg_receivewal is stuck (alive but flushing nothing); restart it locally.
	slotLsnStallTimeout = 60 * time.Second

	// walLocalMinHighWatermarkBytes — pg_receivewal has no
	// kernel-pipe back pressure (it writes files), so the watch dir IS the buffer.
	// Above HIGH we SIGTERM it; we resume only once uploads drain below LOW. The
	// 5x hysteresis prevents flapping on the boundary. HIGH scales up for clusters
	// with non-default wal_segment_size so one segment does not stop the receiver.
	walLocalMinHighWatermarkBytes int64 = 100 * 1024 * 1024

	// receivewalRespawnBackoff — initial pause between a pg_receivewal exit and respawn so
	// a hard-failing source (auth, pg_hba) is not hammered. --no-loop makes
	// pg_receivewal exit on connection loss; this loop is its supervision.
	receivewalRespawnBackoff = 2 * time.Second

	receivewalRespawnMaxBackoff = 30 * time.Minute
)

// WalStreamSpec is the immutable configuration of one database's WAL streamer.
type WalStreamSpec struct {
	DatabaseID     uuid.UUID
	SourceDB       *postgresql_physical.PostgresqlPhysicalDatabase
	StorageID      uuid.UUID
	Storage        storages.StorageFileSaver
	Encryption     backups_core_enums.BackupEncryption
	MasterKey      string
	FieldEncryptor util_encryption.FieldEncryptor
	WalSegmentRepo *physical_repositories.PhysicalWalSegmentRepository
	HistoryRepo    *physical_repositories.PhysicalWalHistoryRepository

	// WatchDirRoot is config.DataFolder; the per-DB queue lives under
	// <root>/wal-queue/<database_id>/. It must survive a process restart so crash
	// recovery can re-process finalized-but-not-uploaded segments.
	WatchDirRoot string

	// WalLagThresholdBytes drives the lag monitor (lag_monitor.go): a slot lag over
	// this many bytes triggers a slot rebuild.
	WalLagThresholdBytes int64

	// OnGapDetected fires once per newly-observed WAL gap (see WalUploader); nil
	// disables notification.
	OnGapDetected func(gapStart, gapEnd walmath.LSN)

	// OnSlotRebuilt fires after the persistent slot has been recreated. Callers use
	// it to request a fresh base backup that anchors the new WAL chain.
	OnSlotRebuilt func(ctx context.Context, reason string) error

	Logger *slog.Logger
}

// WalStreamSupervisor runs and supervises one pg_receivewal process per database:
// it spawns the receiver, archives every fully-rotated segment via the
// insert-first WalUploader, applies disk back pressure, restarts a stalled
// receiver, forwards .history files, and (lag_monitor.go) rebuilds the slot on
// lag/loss. Run blocks until ctx is cancelled.
type WalStreamSupervisor struct {
	spec     WalStreamSpec
	uploader *WalUploader
	watchDir string
	slotName string

	// restartSignal asks the supervision loop to SIGTERM the current
	// pg_receivewal and respawn (sent by the back-pressure monitor and the
	// slot-LSN watcher). Buffered size 1; sends are non-blocking and coalesced.
	restartSignal chan struct{}

	// isPaused holds the supervision loop between pg_receivewal runs so a slot
	// rebuild can drop+recreate the slot without the receiver re-attaching.
	isPaused atomic.Bool

	// rebuildMu serializes slot rebuilds in this process; rebuildTimestamps powers
	// the per-hour loop-protection cap. One supervisor owns a DB at a time (the
	// physical_wal_streamers heartbeat claim), so this is the only guard needed.
	rebuildMu         sync.Mutex
	rebuildTimestamps []time.Time
}

func NewWalStreamSupervisor(spec WalStreamSpec) *WalStreamSupervisor {
	watchDir := filepath.Join(spec.WatchDirRoot, "wal-queue", spec.DatabaseID.String())

	uploader := NewWalUploader(WalUploadDeps{
		DatabaseID:          spec.DatabaseID,
		StorageID:           spec.StorageID,
		Storage:             spec.Storage,
		Encryption:          spec.Encryption,
		MasterKey:           spec.MasterKey,
		FieldEncryptor:      spec.FieldEncryptor,
		WalSegmentRepo:      spec.WalSegmentRepo,
		WalSegmentSizeBytes: walSegmentSizeBytes(spec.SourceDB),
		Logger:              spec.Logger,
		OnGapDetected:       spec.OnGapDetected,
	})

	return &WalStreamSupervisor{
		spec:          spec,
		uploader:      uploader,
		watchDir:      watchDir,
		slotName:      spec.SourceDB.ReplicationSlotName,
		restartSignal: make(chan struct{}, 1),
	}
}

// Run starts the uploader, the back-pressure monitor, the slot-LSN watcher, the
// lag monitor, and the pg_receivewal supervision loop, blocking until ctx is
// cancelled. The persistent slot is created if missing; torn *.partial files are
// cleared before the first spawn.
func (s *WalStreamSupervisor) Run(ctx context.Context) error {
	logger := s.spec.Logger.With("database_id", s.spec.DatabaseID, "slot_name", s.slotName)

	// pg_receivewal finalizes a segment by writing a marker into <dir>/archive_status/
	// and refuses to start (or errors mid-stream) if that subdirectory is absent — it
	// does not create it itself. Create both up front.
	if err := os.MkdirAll(filepath.Join(s.watchDir, "archive_status"), 0o700); err != nil {
		return fmt.Errorf("create wal watch dir: %w", err)
	}

	if err := s.spec.SourceDB.VerifyWalSlot(ctx, logger, s.spec.FieldEncryptor); err != nil {
		return fmt.Errorf("verify persistent replication slot: %w", err)
	}

	var wg sync.WaitGroup

	for _, loop := range []func(context.Context, *slog.Logger){
		s.runUploaderLoop,
		s.runBackpressureMonitor,
		s.runSlotLsnWatcher,
		s.runLagMonitor,
	} {
		wg.Go(func() { loop(ctx, logger) })
	}

	s.runReceivewalSupervision(ctx, logger)

	wg.Wait()

	logger.Info("wal stream supervisor stopped")

	return nil
}

// runReceivewalSupervision is the pg_receivewal lifecycle loop: drain back
// pressure, clear partials, spawn, and wait for exit / restart-signal / ctx.
func (s *WalStreamSupervisor) runReceivewalSupervision(ctx context.Context, logger *slog.Logger) {
	pgBin := tools.GetPostgresqlExecutable(s.spec.SourceDB.Version, tools.PostgresqlExecutablePgReceivewal)
	respawnBackoff := receivewalRespawnBackoff

	for {
		if ctx.Err() != nil {
			return
		}

		if !s.waitWhilePaused(ctx) {
			return
		}

		if !s.waitForBacklogBelowLow(ctx, logger) {
			return
		}

		// Clear any stale restart signal so a spawn does not get cancelled by a
		// signal raised while no process was running.
		s.drainRestartSignal()
		s.removePartials(logger)

		if exitedNormally := s.spawnAndSupervise(ctx, logger, pgBin); !exitedNormally {
			return
		}

		if !sleepCtx(ctx, respawnBackoff) {
			return
		}

		respawnBackoff = min(respawnBackoff*2, receivewalRespawnMaxBackoff)
	}
}

// spawnAndSupervise starts one pg_receivewal process and blocks until it exits,
// the restart signal fires, or ctx is cancelled. Returns false only when ctx was
// cancelled (the caller should stop the loop).
func (s *WalStreamSupervisor) spawnAndSupervise(ctx context.Context, logger *slog.Logger, pgBin string) bool {
	password, err := postgresql_shared.DecryptFieldIfNeeded(s.spec.SourceDB.Password, s.spec.FieldEncryptor)
	if err != nil {
		logger.Error("decrypt source password for pg_receivewal", "error", err)

		return sleepCtx(ctx, receivewalRespawnBackoff)
	}

	creds, err := postgresql_shared.WriteCredentialFilesToTempDir(
		s.spec.SourceDB.CredentialSpec(), password, s.spec.FieldEncryptor,
	)
	if err != nil {
		logger.Error("write pg_receivewal credentials", "error", err)

		return sleepCtx(ctx, receivewalRespawnBackoff)
	}
	defer creds.Remove()

	procCtx, procCancel := context.WithCancel(ctx)
	defer procCancel()

	cmd, err := newReceivewalCommand(procCtx, pgBin, s.spec.SourceDB, creds, s.watchDir, s.slotName)
	if err != nil {
		logger.Error("build pg_receivewal command", "error", err)

		return sleepCtx(ctx, receivewalRespawnBackoff)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.Error("pg_receivewal stderr pipe", "error", err)

		return sleepCtx(ctx, receivewalRespawnBackoff)
	}

	if err := cmd.Start(); err != nil {
		logger.Error("start pg_receivewal", "error", err)

		return sleepCtx(ctx, receivewalRespawnBackoff)
	}

	stderr := newStderrCapture(stderrPipe)

	logger.Info("pg_receivewal started", "watch_dir", s.watchDir)

	exited := make(chan error, 1)

	go func() { exited <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		procCancel()
		<-exited
		stderr.stop()

		return false

	case <-s.restartSignal:
		logger.Info("restarting pg_receivewal on internal signal (back pressure or slot stall)")
		procCancel()
		<-exited
		stderr.stop()

		return true

	case waitErr := <-exited:
		stderr.stop()

		if waitErr != nil && procCtx.Err() == nil {
			logger.Warn("pg_receivewal exited; will respawn",
				"error", waitErr, "stderr", truncateStderr(stderr.contents()))
		}

		return true
	}
}

// runUploaderLoop scans the watch dir on a tight interval and hands each
// finalized segment / .history file to the appropriate handler.
func (s *WalStreamSupervisor) runUploaderLoop(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(uploaderPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.scanAndUpload(ctx, logger)
		}
	}
}

func (s *WalStreamSupervisor) scanAndUpload(ctx context.Context, logger *slog.Logger) {
	entries, err := os.ReadDir(s.watchDir)
	if err != nil {
		logger.Error("read wal watch dir", "error", err)

		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		switch {
		case walmath.IsWalFilename(name):
			if err := s.uploader.ProcessSegment(ctx, filepath.Join(s.watchDir, name), name); err != nil {
				logger.Warn("wal segment upload failed; will retry next tick", "wal_filename", name, "error", err)
			}

		case strings.HasSuffix(name, ".history"):
			s.handleHistoryFile(ctx, logger, name)
		}
	}
}

// handleHistoryFile uploads a .history file the receiver dropped into watch_dir
// (reusing UploadHistoryFile, which reads the body from the source cluster and is
// idempotent on (database_id, timeline_id)), then removes the local copy.
func (s *WalStreamSupervisor) handleHistoryFile(ctx context.Context, logger *slog.Logger, name string) {
	timelineID, err := parseHistoryTimeline(name)
	if err != nil {
		logger.Warn("skip unparseable history file", "name", name, "error", err)

		return
	}

	conn, err := s.spec.SourceDB.OpenInspectionConn(ctx, s.spec.FieldEncryptor)
	if err != nil {
		logger.Warn("could not open connection to upload history file; will retry", "error", err)

		return
	}
	defer func() { _ = conn.Close(context.Background()) }()

	if _, err := UploadHistoryFile(
		ctx, conn, timelineID, s.spec.Storage, s.spec.SourceDB, s.spec.StorageID,
		s.spec.HistoryRepo, s.spec.Encryption, s.spec.MasterKey, s.spec.FieldEncryptor, logger,
	); err != nil {
		logger.Warn("history upload failed; will retry next tick", "timeline_id", timelineID, "error", err)

		return
	}

	logger.Info("timeline switch observed via .history", "timeline_id", timelineID)

	if err := os.Remove(filepath.Join(s.watchDir, name)); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove uploaded history file", "name", name, "error", err)
	}
}

// stallTracker tracks slot restart_lsn advance across watcher ticks. A changed
// LSN (or the very first sample) re-arms the advance clock; an LSN that stays
// frozen past the stall timeout is the signal that pg_receivewal is alive but
// flushing nothing on a reachable source.
type stallTracker struct {
	lastRestartLSN walmath.LSN
	lastAdvanceAt  time.Time
	hasSample      bool
}

// observe records a restart_lsn sample taken at now and reports whether the slot
// has stalled longer than stallTimeout. On a positive result it re-arms the clock
// so the caller restarts at most once per stallTimeout window.
func (t *stallTracker) observe(restartLSN walmath.LSN, now time.Time, stallTimeout time.Duration) bool {
	if !t.hasSample || restartLSN != t.lastRestartLSN {
		t.lastRestartLSN = restartLSN
		t.lastAdvanceAt = now
		t.hasSample = true

		return false
	}

	if now.Sub(t.lastAdvanceAt) > stallTimeout {
		t.lastAdvanceAt = now

		return true
	}

	return false
}

// runSlotLsnWatcher restarts pg_receivewal when the slot's restart_lsn has not
// advanced for slotLsnStallTimeout while the source PG is still reachable — a
// stuck consumer on a healthy server. A stall with an unreachable server is left
// to the lag monitor (slot loss / network down).
func (s *WalStreamSupervisor) runSlotLsnWatcher(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(slotLsnWatcherPollInterval)
	defer ticker.Stop()

	var tracker stallTracker

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			restartLSN, pgReachable := s.sampleSlotRestartLSN(ctx, logger)
			if !pgReachable {
				// Connectivity / slot loss is the lag monitor's job, not ours.
				continue
			}

			if tracker.observe(restartLSN, time.Now().UTC(), slotLsnStallTimeout) {
				logger.Warn("slot restart_lsn stalled on a reachable source; restarting pg_receivewal",
					"restart_lsn", restartLSN.String())

				s.signalRestart()
			}
		}
	}
}

// sampleSlotRestartLSN reads the slot's restart_lsn and confirms the source is
// reachable with a keepalive. pgReachable=false means defer to the lag monitor.
func (s *WalStreamSupervisor) sampleSlotRestartLSN(ctx context.Context, logger *slog.Logger) (walmath.LSN, bool) {
	conn, err := s.spec.SourceDB.OpenInspectionConn(ctx, s.spec.FieldEncryptor)
	if err != nil {
		logger.Debug("slot-lsn watcher: source unreachable, deferring to lag monitor", "error", err)

		return 0, false
	}
	defer func() { _ = conn.Close(context.Background()) }()

	state, err := InspectSlot(ctx, conn, s.slotName)
	if err != nil || state == nil {
		return 0, false
	}

	var alive int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&alive); err != nil {
		return 0, false
	}

	return state.RestartLSN, true
}

// runBackpressureMonitor SIGTERMs pg_receivewal (via the restart signal) when the
// local watch-dir backlog crosses the high watermark; the supervision loop then
// waits for the uploader to drain below the low watermark before respawning.
func (s *WalStreamSupervisor) runBackpressureMonitor(ctx context.Context, _ *slog.Logger) {
	ticker := time.NewTicker(uploaderPollInterval)
	defer ticker.Stop()

	highBytes := s.walBacklogHighWatermarkBytes()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if s.backlogBytes() >= highBytes {
				s.signalRestart()
			}
		}
	}
}

// waitForBacklogBelowLow blocks while the backlog is at/over the high watermark,
// returning once it drains below the low watermark. Returns false if ctx is
// cancelled while waiting.
func (s *WalStreamSupervisor) waitForBacklogBelowLow(ctx context.Context, logger *slog.Logger) bool {
	highBytes := s.walBacklogHighWatermarkBytes()
	lowBytes := s.walBacklogLowWatermarkBytes()

	if s.backlogBytes() < highBytes {
		return true
	}

	logger.Warn("wal backlog over high watermark; pausing pg_receivewal until it drains")

	ticker := time.NewTicker(uploaderPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
			if s.backlogBytes() < lowBytes {
				logger.Info("wal backlog drained below low watermark; resuming pg_receivewal")

				return true
			}
		}
	}
}

// backlogBytes sums the sizes of finalized (not-yet-uploaded) WAL segments in the
// watch dir. Uploaded segments are removed by the uploader, so this is the local
// queue depth; .partial and .history files are excluded.
func (s *WalStreamSupervisor) backlogBytes() int64 {
	entries, err := os.ReadDir(s.watchDir)
	if err != nil {
		return 0
	}

	var total int64

	for _, entry := range entries {
		if entry.IsDir() || !walmath.IsWalFilename(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		total += info.Size()
	}

	return total
}

func (s *WalStreamSupervisor) walBacklogHighWatermarkBytes() int64 {
	return max(walLocalMinHighWatermarkBytes, 4*walSegmentSizeBytes(s.spec.SourceDB))
}

func (s *WalStreamSupervisor) walBacklogLowWatermarkBytes() int64 {
	return s.walBacklogHighWatermarkBytes() / 5
}

func (s *WalStreamSupervisor) removePartials(logger *slog.Logger) {
	entries, err := os.ReadDir(s.watchDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}

		if err := os.Remove(filepath.Join(s.watchDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove partial wal file", "name", entry.Name(), "error", err)
		}
	}
}

// waitWhilePaused blocks the supervision loop while a slot rebuild holds the
// receiver down. Returns false if ctx is cancelled while paused.
func (s *WalStreamSupervisor) waitWhilePaused(ctx context.Context) bool {
	if !s.isPaused.Load() {
		return true
	}

	ticker := time.NewTicker(uploaderPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
			if !s.isPaused.Load() {
				return true
			}
		}
	}
}

func (s *WalStreamSupervisor) signalRestart() {
	select {
	case s.restartSignal <- struct{}{}:
	default:
	}
}

func (s *WalStreamSupervisor) drainRestartSignal() {
	select {
	case <-s.restartSignal:
	default:
	}
}

// newReceivewalCommand builds the pg_receivewal invocation. WAL is left
// uncompressed locally (no --compress) because the uploader re-compresses with
// zstd on upload; --no-loop makes the process exit on connection loss so the
// supervision loop owns retry; --synchronous flushes each segment promptly. SSL
// is supplied through the same PGSSL* env path pg_basebackup uses, so mTLS needs
// no extra handling here.
func newReceivewalCommand(
	ctx context.Context,
	pgBin string,
	sourceDB *postgresql_physical.PostgresqlPhysicalDatabase,
	creds *postgresql_shared.CredentialTempFiles,
	watchDir string,
	slotName string,
) (*exec.Cmd, error) {
	if _, err := exec.LookPath(pgBin); err != nil {
		return nil, fmt.Errorf("pg_receivewal binary not found at %s: %w", pgBin, err)
	}

	args := []string{
		"--directory=" + watchDir,
		"--slot=" + slotName,
		"--no-loop",
		"--synchronous",
		"--verbose",
		"--no-password",
		"-h", sourceDB.Host,
		"-p", strconv.Itoa(sourceDB.Port),
		"-U", sourceDB.Username,
	}

	cmd := exec.CommandContext(ctx, pgBin, args...)

	cmd.Env = append(os.Environ(),
		"PGPASSFILE="+creds.PgpassPath,
		"PGAPPNAME="+receivewalApplicationName(sourceDB),
		"PGCLIENTENCODING=UTF8",
		"PGCONNECT_TIMEOUT=30",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
	)

	sslMode := sourceDB.SslMode
	if sslMode == "" {
		sslMode = postgresql_shared.PostgresSslModeDisable
	}

	cmd.Env = append(cmd.Env,
		"PGSSLMODE="+string(sslMode),
		"PGSSLCERT="+creds.ClientCertPath,
		"PGSSLKEY="+creds.ClientKeyPath,
		"PGSSLROOTCERT="+creds.RootCertPath,
		"PGSSLCRL=",
	)

	cmd.Cancel = func() error {
		return signalForGracefulCancel(cmd.Process)
	}

	cmd.WaitDelay = killAfterCancel
	setReceivewalProcessAttributes(cmd)

	return cmd, nil
}

func receivewalApplicationName(sourceDB *postgresql_physical.PostgresqlPhysicalDatabase) string {
	if sourceDB.DatabaseID == nil {
		return receivewalApplicationNamePrefix + sourceDB.ID.String()
	}

	return receivewalApplicationNamePrefix + sourceDB.DatabaseID.String()
}

// walSegmentSizeBytes returns the source cluster's captured wal_segment_size, or
// the 16 MB default when it has not been captured yet.
func walSegmentSizeBytes(sourceDB *postgresql_physical.PostgresqlPhysicalDatabase) int64 {
	if sourceDB.WalSegmentSizeBytes != nil && *sourceDB.WalSegmentSizeBytes > 0 {
		return *sourceDB.WalSegmentSizeBytes
	}

	return int64(walmath.WalSegmentSize)
}

// parseHistoryTimeline extracts the timeline id from a "%08X.history" filename.
func parseHistoryTimeline(name string) (int, error) {
	trimmed := strings.TrimSuffix(name, ".history")
	if trimmed == name {
		return 0, fmt.Errorf("not a history filename: %q", name)
	}

	timelineID, err := strconv.ParseUint(trimmed, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse timeline from %q: %w", name, err)
	}

	return int(timelineID), nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false

	case <-timer.C:
		return true
	}
}
