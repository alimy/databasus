package backuping_physical_postgresql

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	chain_view "databasus-backend/internal/features/backups/backups/core/physical/chain_view"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	backup_encryption "databasus-backend/internal/features/backups/backups/encryption"
	backups_config_logical "databasus-backend/internal/features/backups/config/logical"
	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	postgresql_shared "databasus-backend/internal/features/databases/databases/postgresql/shared"
	"databasus-backend/internal/features/storages"
	util_encryption "databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/tools"
	"databasus-backend/internal/util/walmath"
)

// FullSpec carries everything FullExecutor.Execute needs. The executor does
// not touch the catalog — it returns a FullResult that the backuper applies
// (single-writer-of-status invariant per §B "State split").
type FullSpec struct {
	SourceDB       *postgresql_physical.PostgresqlPhysicalDatabase
	Backup         *physical_models.PhysicalFullBackup
	StorageID      uuid.UUID
	Storage        storages.StorageFileSaver
	BackupConfig   *backups_config_logical.LogicalBackupConfig // for Encryption flag — physical reuses logical's enum
	Encryption     backups_config_logical.BackupEncryption
	MasterKey      string
	FieldEncryptor util_encryption.FieldEncryptor
	FullRepo       *physical_repositories.PhysicalFullBackupRepository
	HistoryRepo    *physical_repositories.PhysicalWalHistoryRepository
	Logger         *slog.Logger
}

// FullResult is the typed return of FullExecutor.Execute. The caller
// (backuper.go) maps it to a row UPDATE.
type FullResult struct {
	Status       physical_enums.PhysicalBackupStatus
	ErrorReason  *physical_enums.PhysicalBackupErrorReason
	ErrorMessage string

	FileName string

	TimelineID int
	StartLSN   walmath.LSN
	StopLSN    walmath.LSN

	BackupSizeMb     float64
	BackupDurationMs int64

	EncryptionAlgo physical_enums.PhysicalBackupEncryption
	EncryptionSalt string
	EncryptionIV   string

	CompletedAt time.Time
}

type FullExecutor struct{}

func NewFullExecutor() *FullExecutor { return &FullExecutor{} }

// killAfterCancel is how long we wait between sending SIGTERM (or its
// platform equivalent) and escalating to SIGKILL. The byte-stall watcher
// triggers cancel; pg_basebackup should exit cleanly on SIGTERM but a
// stuck network FD may keep it alive.
const killAfterCancel = 10 * time.Second

// pgBasebackupStartPointRegexp parses lines like
// "pg_basebackup: ... write-ahead log start point: 0/3000060 on timeline 1".
var pgBasebackupStartPointRegexp = regexp.MustCompile(
	`write-ahead log start point:\s+([0-9A-Fa-f]+/[0-9A-Fa-f]+)\s+on timeline\s+(\d+)`,
)

// pgBasebackupStopPointRegexp parses lines like
// "pg_basebackup: ... write-ahead log end point: 0/3000220".
var pgBasebackupStopPointRegexp = regexp.MustCompile(
	`write-ahead log end point:\s+([0-9A-Fa-f]+/[0-9A-Fa-f]+)`,
)

func (e *FullExecutor) Execute(ctx context.Context, spec FullSpec) (FullResult, error) {
	start := time.Now().UTC()

	password, err := decryptIfNeeded(spec.SourceDB.Password, spec.FieldEncryptor)
	if err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed, "decrypt password", err, start), nil
	}

	creds, err := WriteCredentials(spec.SourceDB, password, spec.FieldEncryptor)
	if err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed, "write credentials", err, start), nil
	}
	defer creds.Remove()

	preflightResult, ok := runFullPreflight(ctx, spec, start)
	if !ok {
		return preflightResult, nil
	}

	fileName := buildFullObjectName(spec.SourceDB, spec.Backup.ID, start)

	if err := spec.FullRepo.Save(withInProgressFileName(spec.Backup, fileName)); err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorStorageUploadFailed,
			"persist file_name at upload-start", err, start), nil
	}

	pgBin := tools.GetPostgresqlExecutable(spec.SourceDB.Version, tools.PostgresqlExecutablePgBasebackup)

	cmd, err := newPgBasebackupCommand(ctx, pgBin, spec, creds, password, fileName)
	if err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"build pg_basebackup command", err, start), nil
	}

	var result FullResult

	slotErr := WithBackupSlot(ctx, spec.SourceDB, spec.FieldEncryptor, spec.Logger, func() error {
		streamResult, err := runFullStream(ctx, spec, cmd, fileName, start)
		if err != nil {
			result = errorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
				"pg_basebackup stream", err, start)
			return nil
		}

		if streamResult.Status != physical_enums.PhysicalBackupStatusCompleted {
			result = streamResult
			return nil
		}

		validation, valErr := ValidateStartLsnAgainstHistory(
			spec.SourceDB.ID,
			streamResult.TimelineID,
			streamResult.StartLSN,
			spec.HistoryRepo,
		)
		if valErr != nil {
			spec.Logger.Warn("start-LSN history validation failed",
				"backup_id", spec.Backup.ID,
				"error", valErr)
		}

		if validation.Status == chain_view.ValidationStatusChainBroken {
			removeStorageArtifact(spec.Storage, spec.FieldEncryptor, fileName, spec.Logger)

			reason := physical_enums.PhysicalBackupErrorStartLsnOutsideTimeline

			result = FullResult{
				Status:       physical_enums.PhysicalBackupStatusChainBroken,
				ErrorReason:  &reason,
				ErrorMessage: validation.Message,
				FileName:     fileName,
				TimelineID:   streamResult.TimelineID,
				StartLSN:     streamResult.StartLSN,
				StopLSN:      streamResult.StopLSN,
				CompletedAt:  time.Now().UTC(),
			}

			return nil
		}

		if validation.Status == chain_view.ValidationStatusOKWithWarning {
			spec.Logger.Info(validation.Message,
				"backup_id", spec.Backup.ID,
				"timeline_id", streamResult.TimelineID)
		}

		if streamResult.TimelineID > 1 {
			historyConn, connErr := spec.SourceDB.OpenInspectionConn(ctx, spec.FieldEncryptor)
			if connErr != nil {
				spec.Logger.Warn("could not open connection for history upload; FULL stays COMPLETED",
					"backup_id", spec.Backup.ID,
					"timeline_id", streamResult.TimelineID,
					"error", connErr)
			} else {
				if _, uploadErr := UploadHistoryFile(
					ctx,
					historyConn,
					streamResult.TimelineID,
					spec.Storage,
					spec.SourceDB,
					spec.StorageID,
					spec.HistoryRepo,
					spec.Encryption,
					spec.MasterKey,
					spec.FieldEncryptor,
					spec.Logger,
				); uploadErr != nil {
					spec.Logger.Warn("history upload failed; FULL stays COMPLETED",
						"backup_id", spec.Backup.ID,
						"timeline_id", streamResult.TimelineID,
						"error", uploadErr)
				}

				_ = historyConn.Close(ctx)
			}
		}

		streamResult.BackupDurationMs = time.Since(start).Milliseconds()
		streamResult.CompletedAt = time.Now().UTC()
		streamResult.FileName = fileName

		result = streamResult

		return nil
	})

	if slotErr != nil {
		return errorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"per-backup slot lifecycle", slotErr, start), nil
	}

	return result, nil
}

// runFullPreflight runs CheckTimelineCompatibility and translates each
// refusal kind into the typed FullResult; returns (result, false) when the
// caller must short-circuit, (zero, true) for proceed.
func runFullPreflight(ctx context.Context, spec FullSpec, start time.Time) (FullResult, bool) {
	conn, err := spec.SourceDB.OpenInspectionConn(ctx, spec.FieldEncryptor)
	if err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"open pre-flight connection", err, start), false
	}
	defer func() { _ = conn.Close(ctx) }()

	decision, err := CheckTimelineCompatibility(ctx, conn, spec.SourceDB, spec.FullRepo, spec.HistoryRepo)
	if err != nil {
		return errorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"timeline pre-flight", err, start), false
	}

	switch decision.Kind {
	case TimelineContinue, TimelineFailoverDetected:
		// FailoverDetected is not a refusal in the executor — the executor
		// proceeds with the live TL. The scheduler observes the bumped TL
		// across two FULL rows and treats subsequent ticks accordingly.
		return FullResult{}, true

	case TimelineRegression:
		reason := physical_enums.PhysicalBackupErrorTimelineRegression

		return FullResult{
			Status:      physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason: &reason,
			ErrorMessage: fmt.Sprintf(
				"timeline regression: expected TL %d, live TL %d",
				decision.ExpectedTLI, decision.ActualTLI,
			),
			CompletedAt: time.Now().UTC(),
		}, false

	case TimelineDifferentCluster:
		reason := physical_enums.PhysicalBackupErrorSystemIdentifierMismatch

		return FullResult{
			Status:      physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason: &reason,
			ErrorMessage: fmt.Sprintf(
				"system_identifier mismatch: catalog %s, live %s",
				decision.ExpectedSysID, decision.ActualSysID,
			),
			CompletedAt: time.Now().UTC(),
		}, false
	}

	return FullResult{}, true
}

func newPgBasebackupCommand(
	ctx context.Context,
	pgBin string,
	spec FullSpec,
	creds *Credentials,
	password string,
	label string,
) (*exec.Cmd, error) {
	if _, err := exec.LookPath(pgBin); err != nil {
		return nil, fmt.Errorf("pg_basebackup binary not found at %s: %w", pgBin, err)
	}

	args := []string{
		"--pgdata=-",
		"--format=tar",
		// --compress=client-zstd:5 — server-side compression is incompatible
		// with manifest injection into a tar-to-stdout stream (pg_basebackup
		// errors out with "cannot inject manifest into a compressed tar
		// file"). Client-side compression lets us keep the manifest, which
		// is required for the incremental chain (pg_basebackup --incremental
		// consumes it). The CPU cost lives on the Databasus host, which is
		// the same host that pipes bytes to storage — no extra hop.
		"--compress=client-zstd:5",
		// --wal-method=fetch inlines WAL into the same tar; --wal-method=stream
		// requires a separate output stream which pg_basebackup refuses when
		// writing tar to stdout. Our persistent replication slot keeps the
		// fetched WAL segments retained on the source until the backup
		// completes.
		"--wal-method=fetch",
		"--checkpoint=fast",
		"--label=" + label,
		"--no-password",
		"--verbose",
		"--manifest-checksums=SHA256",
		"-h", spec.SourceDB.Host,
		"-p", strconv.Itoa(spec.SourceDB.Port),
		"-U", spec.SourceDB.Username,
	}

	cmd := exec.CommandContext(ctx, pgBin, args...)

	cmd.Env = append(os.Environ(),
		"PGPASSFILE="+creds.PgpassPath,
		"PGCLIENTENCODING=UTF8",
		"PGCONNECT_TIMEOUT=30",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
	)

	sslMode := spec.SourceDB.SslMode
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

	return cmd, nil
}

func runFullStream(
	ctx context.Context,
	spec FullSpec,
	cmd *exec.Cmd,
	fileName string,
	start time.Time,
) (FullResult, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return FullResult{}, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return FullResult{}, fmt.Errorf("stderr pipe: %w", err)
	}

	storageReader, storageWriter := io.Pipe()

	finalWriter, encryptionWriter, encSalt, encNonce, err := setupEncryption(
		storageWriter, spec.Encryption, spec.MasterKey, spec.Backup.ID,
	)
	if err != nil {
		_ = storageWriter.Close()

		return FullResult{}, err
	}

	counter := NewByteCounter(finalWriter)

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	stopWatcher := WithByteStallWatcher(streamCtx, counter, ByteStallTimeout, func() {
		spec.Logger.Warn("byte-stall timeout tripped; terminating pg_basebackup",
			"backup_id", spec.Backup.ID,
			"file_name", fileName)

		cancelStream()
	})
	defer stopWatcher()

	saveErrCh := make(chan error, 1)

	go func() {
		saveErr := spec.Storage.SaveFile(
			streamCtx,
			spec.FieldEncryptor,
			spec.Logger,
			fileName,
			storageReader,
		)
		if saveErr != nil {
			_ = storageReader.CloseWithError(saveErr)
			cancelStream()
		}

		saveErrCh <- saveErr
	}()

	stderr := newStderrCapture(stderrPipe)

	if err := cmd.Start(); err != nil {
		_ = storageWriter.Close()
		<-saveErrCh

		return FullResult{}, fmt.Errorf("start pg_basebackup: %w", err)
	}

	copyErrCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(counter, stdoutPipe)
		copyErrCh <- err
	}()

	copyErr := <-copyErrCh

	if streamCtx.Err() != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	waitErr := cmd.Wait()

	if encryptionWriter != nil {
		_ = encryptionWriter.Close()
	}

	_ = storageWriter.Close()

	saveErr := <-saveErrCh

	stderr.stop()
	stderrBytes := stderr.contents()

	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return failFromStreamError(copyErr, stderrBytes, streamCtx, start), nil
	}

	if waitErr != nil {
		return failFromStreamError(waitErr, stderrBytes, streamCtx, start), nil
	}

	if saveErr != nil {
		reason := physical_enums.PhysicalBackupErrorStorageUploadFailed

		return FullResult{
			Status:       physical_enums.PhysicalBackupStatusError,
			ErrorReason:  &reason,
			ErrorMessage: fmt.Sprintf("save to storage: %v", saveErr),
		}, nil
	}

	startLSN, stopLSN, timelineID, err := parseLsnsFromStderr(stderrBytes)
	if err != nil {
		reason := physical_enums.PhysicalBackupErrorManifestCorrupted

		return FullResult{
			Status:       physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason:  &reason,
			ErrorMessage: fmt.Sprintf("parse pg_basebackup LSNs: %v", err),
			FileName:     fileName,
		}, nil
	}

	encAlgo := physical_enums.PhysicalBackupEncryptionNone
	if encryptionWriter != nil {
		encAlgo = physical_enums.PhysicalBackupEncryptionAes256Gcm
	}

	return FullResult{
		Status:         physical_enums.PhysicalBackupStatusCompleted,
		FileName:       fileName,
		TimelineID:     timelineID,
		StartLSN:       startLSN,
		StopLSN:        stopLSN,
		BackupSizeMb:   float64(counter.BytesWritten()) / (1024 * 1024),
		EncryptionAlgo: encAlgo,
		EncryptionSalt: encSalt,
		EncryptionIV:   encNonce,
	}, nil
}

func failFromStreamError(
	streamErr error,
	stderrBytes []byte,
	ctx context.Context,
	_ time.Time,
) FullResult {
	if errors.Is(ctx.Err(), context.Canceled) {
		reason := physical_enums.PhysicalBackupErrorNetworkStallTimeout

		return FullResult{
			Status:       physical_enums.PhysicalBackupStatusError,
			ErrorReason:  &reason,
			ErrorMessage: "byte-stall watcher cancelled pg_basebackup",
		}
	}

	reason := physical_enums.PhysicalBackupErrorPgBasebackupFailed

	msg := fmt.Sprintf("%v; stderr: %s", streamErr, truncateStderr(stderrBytes))

	return FullResult{
		Status:       physical_enums.PhysicalBackupStatusError,
		ErrorReason:  &reason,
		ErrorMessage: msg,
	}
}

func parseLsnsFromStderr(stderr []byte) (walmath.LSN, walmath.LSN, int, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(stderr)))

	var (
		startLSN, stopLSN walmath.LSN
		timelineID        int
		gotStart, gotStop bool
	)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := pgBasebackupStartPointRegexp.FindStringSubmatch(line); matches != nil {
			parsed, err := walmath.ParseLSN(matches[1])
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse start LSN: %w", err)
			}

			tli, err := strconv.Atoi(matches[2])
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse timeline ID: %w", err)
			}

			startLSN = parsed
			timelineID = tli
			gotStart = true
		}

		if matches := pgBasebackupStopPointRegexp.FindStringSubmatch(line); matches != nil {
			parsed, err := walmath.ParseLSN(matches[1])
			if err != nil {
				return 0, 0, 0, fmt.Errorf("parse stop LSN: %w", err)
			}

			stopLSN = parsed
			gotStop = true
		}
	}

	if !gotStart || !gotStop {
		return 0, 0, 0, errors.New("pg_basebackup did not emit both start and end points")
	}

	return startLSN, stopLSN, timelineID, nil
}

func buildFullObjectName(
	db *postgresql_physical.PostgresqlPhysicalDatabase,
	backupID uuid.UUID,
	now time.Time,
) string {
	return fmt.Sprintf("%s-FULL-%s-%s.tar.zst",
		db.ID.String(),
		now.Format("20060102-150405"),
		backupID.String(),
	)
}

func withInProgressFileName(
	b *physical_models.PhysicalFullBackup,
	fileName string,
) *physical_models.PhysicalFullBackup {
	b.FileName = &fileName

	return b
}

func errorResult(
	reason physical_enums.PhysicalBackupErrorReason,
	stage string,
	err error,
	_ time.Time,
) FullResult {
	r := reason

	return FullResult{
		Status:       physical_enums.PhysicalBackupStatusError,
		ErrorReason:  &r,
		ErrorMessage: fmt.Sprintf("%s: %v", stage, err),
		CompletedAt:  time.Now().UTC(),
	}
}

func setupEncryption(
	base io.Writer,
	enc backups_config_logical.BackupEncryption,
	masterKey string,
	backupID uuid.UUID,
) (io.Writer, *backup_encryption.EncryptionWriter, string, string, error) {
	if enc != backups_config_logical.BackupEncryptionEncrypted {
		return base, nil, "", "", nil
	}

	encSetup, err := backup_encryption.SetupEncryptionWriter(base, masterKey, backupID)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("setup encryption: %w", err)
	}

	return encSetup.Writer, encSetup.Writer, encSetup.SaltBase64, encSetup.NonceBase64, nil
}

// stderrCapture drains a Reader (pg_basebackup stderr) without blocking the
// caller. The goroutine reads until EOF or stop() is called; contents()
// returns whatever was captured so far.
type stderrCapture struct {
	pipe io.ReadCloser
	mu   sync.Mutex
	buf  []byte
	done chan struct{}
	once sync.Once
}

func newStderrCapture(pipe io.ReadCloser) *stderrCapture {
	c := &stderrCapture{
		pipe: pipe,
		done: make(chan struct{}),
	}

	go func() {
		defer close(c.done)

		out, _ := io.ReadAll(pipe)

		c.mu.Lock()
		c.buf = out
		c.mu.Unlock()
	}()

	return c
}

func (c *stderrCapture) stop() {
	c.once.Do(func() {
		_ = c.pipe.Close()
		<-c.done
	})
}

func (c *stderrCapture) contents() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]byte, len(c.buf))
	copy(out, c.buf)

	return out
}

func truncateStderr(b []byte) string {
	const max = 2048

	if len(b) <= max {
		return string(b)
	}

	return string(b[len(b)-max:])
}

func signalForGracefulCancel(p *os.Process) error {
	if p == nil {
		return nil
	}

	return p.Signal(os.Interrupt)
}

func removeStorageArtifact(
	storage storages.StorageFileSaver,
	encryptor util_encryption.FieldEncryptor,
	fileName string,
	logger *slog.Logger,
) {
	if err := storage.DeleteFile(encryptor, fileName+".metadata"); err != nil {
		logger.Warn("failed to remove sidecar after CHAIN_BROKEN",
			"file_name", fileName+".metadata",
			"error", err)
	}

	if err := storage.DeleteFile(encryptor, fileName); err != nil {
		logger.Warn("failed to remove artifact after CHAIN_BROKEN",
			"file_name", fileName,
			"error", err)
	}
}
