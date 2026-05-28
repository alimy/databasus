package backuping_physical_postgresql

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// IncrSpec is FullSpec plus the parent-manifest source. The caller
// resolves whether the parent is the chain root FULL or a preceding INCR
// — the executor only sees one ParentManifest reference and one
// (Encryption, Salt, IV) triple it can use to decrypt it.
type IncrSpec struct {
	SourceDB       *postgresql_physical.PostgresqlPhysicalDatabase
	Backup         *physical_models.PhysicalIncrementalBackup
	StorageID      uuid.UUID
	Storage        storages.StorageFileSaver
	BackupConfig   *backups_config_logical.LogicalBackupConfig
	Encryption     backups_config_logical.BackupEncryption
	MasterKey      string
	FieldEncryptor util_encryption.FieldEncryptor

	ParentFileName       string
	ParentBackupID       uuid.UUID
	ParentEncryption     backups_config_logical.BackupEncryption
	ParentEncryptionSalt string
	ParentEncryptionIV   string

	FullRepo    *physical_repositories.PhysicalFullBackupRepository
	IncrRepo    *physical_repositories.PhysicalIncrementalBackupRepository
	HistoryRepo *physical_repositories.PhysicalWalHistoryRepository

	Logger *slog.Logger
}

type IncrResult struct {
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

type IncrementalExecutor struct{}

func NewIncrementalExecutor() *IncrementalExecutor { return &IncrementalExecutor{} }

func (e *IncrementalExecutor) Execute(ctx context.Context, spec IncrSpec) (IncrResult, error) {
	start := time.Now().UTC()

	password, err := decryptIfNeeded(spec.SourceDB.Password, spec.FieldEncryptor)
	if err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"decrypt password", err, start), nil
	}

	creds, err := WriteCredentials(spec.SourceDB, password, spec.FieldEncryptor)
	if err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"write credentials", err, start), nil
	}
	defer creds.Remove()

	if preflightResult, ok := runIncrPreflight(ctx, spec, start); !ok {
		return preflightResult, nil
	}

	manifestPath, manifestCleanup, err := downloadParentManifest(ctx, spec)
	if err != nil {
		reason := physical_enums.PhysicalBackupErrorParentManifestMissing

		return IncrResult{
			Status:       physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason:  &reason,
			ErrorMessage: fmt.Sprintf("download parent manifest: %v", err),
			CompletedAt:  time.Now().UTC(),
		}, nil
	}
	defer manifestCleanup()

	fileName := buildIncrObjectName(spec.SourceDB, spec.Backup.ID, start)

	spec.Backup.FileName = &fileName

	if err := spec.IncrRepo.Save(spec.Backup); err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorStorageUploadFailed,
			"persist file_name at upload-start", err, start), nil
	}

	pgBin := tools.GetPostgresqlExecutable(spec.SourceDB.Version, tools.PostgresqlExecutablePgBasebackup)

	cmd, err := newPgBasebackupIncrCommand(ctx, pgBin, spec, creds, manifestPath, fileName)
	if err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
			"build pg_basebackup --incremental command", err, start), nil
	}

	var result IncrResult

	slotErr := WithBackupSlot(ctx, spec.SourceDB, spec.FieldEncryptor, spec.Logger, func() error {
		streamResult, err := runIncrStream(ctx, spec, cmd, fileName, start)
		if err != nil {
			result = incrErrorResult(physical_enums.PhysicalBackupErrorPgBasebackupFailed,
				"pg_basebackup --incremental stream", err, start)
			return nil
		}

		if streamResult.Status != physical_enums.PhysicalBackupStatusCompleted {
			result = streamResult
			return nil
		}

		streamResult.BackupDurationMs = time.Since(start).Milliseconds()
		streamResult.CompletedAt = time.Now().UTC()
		streamResult.FileName = fileName

		result = streamResult

		return nil
	})

	if slotErr != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"per-backup slot lifecycle", slotErr, start), nil
	}

	return result, nil
}

func runIncrPreflight(ctx context.Context, spec IncrSpec, start time.Time) (IncrResult, bool) {
	conn, err := spec.SourceDB.OpenInspectionConn(ctx, spec.FieldEncryptor)
	if err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"open pre-flight connection", err, start), false
	}
	defer func() { _ = conn.Close(ctx) }()

	decision, err := CheckTimelineCompatibility(ctx, conn, spec.SourceDB, spec.FullRepo, spec.HistoryRepo)
	if err != nil {
		return incrErrorResult(physical_enums.PhysicalBackupErrorNetworkFailure,
			"timeline pre-flight", err, start), false
	}

	switch decision.Kind {
	case TimelineContinue:
		return IncrResult{}, true

	case TimelineFailoverDetected:
		// An INCR cannot extend across a timeline switch — the chain must
		// re-anchor on a fresh FULL on the new TL.
		reason := physical_enums.PhysicalBackupErrorTimelineRegression

		return IncrResult{
			Status:      physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason: &reason,
			ErrorMessage: fmt.Sprintf(
				"timeline switch detected (live TL %d > known TL): incremental refused, new FULL required",
				decision.NewTLI,
			),
			CompletedAt: time.Now().UTC(),
		}, false

	case TimelineRegression:
		reason := physical_enums.PhysicalBackupErrorTimelineRegression

		return IncrResult{
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

		return IncrResult{
			Status:      physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason: &reason,
			ErrorMessage: fmt.Sprintf(
				"system_identifier mismatch: catalog %s, live %s",
				decision.ExpectedSysID, decision.ActualSysID,
			),
			CompletedAt: time.Now().UTC(),
		}, false
	}

	return IncrResult{}, true
}

// downloadParentManifest reads the parent backup's manifest from storage,
// optionally decrypts it, and writes it to a temp file pg_basebackup can
// open. Returns the temp file path + a cleanup callback. The parent
// manifest is named "<parent_file_name>.manifest" by convention.
//
// PR 2 ships a minimal best-effort downloader: if the parent had
// encryption ON, we decrypt; otherwise we copy bytes through. The
// parent_manifest path inside the parent's tar artifact is a future
// refinement — for now we expect the manifest stored as a separate
// storage object alongside the tar.
func downloadParentManifest(
	ctx context.Context,
	spec IncrSpec,
) (manifestPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "pgincr_"+uuid.New().String())
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	cleanupAll := func() { _ = os.RemoveAll(tmpDir) }

	storageHandle, ok := spec.Storage.(parentManifestFetcher)
	if !ok {
		cleanupAll()

		return "", func() {}, errors.New("storage does not support GetFile")
	}

	reader, err := storageHandle.GetFile(spec.FieldEncryptor, spec.ParentFileName+".manifest")
	if err != nil {
		cleanupAll()

		return "", func() {}, fmt.Errorf("fetch parent manifest: %w", err)
	}
	defer func() { _ = reader.Close() }()

	decoded := io.Reader(reader)

	if spec.ParentEncryption == backups_config_logical.BackupEncryptionEncrypted {
		decoded, err = decryptParentManifest(reader, spec)
		if err != nil {
			cleanupAll()

			return "", func() {}, err
		}
	}

	manifestPath = filepath.Join(tmpDir, "backup_manifest")

	out, err := os.Create(manifestPath)
	if err != nil {
		cleanupAll()

		return "", func() {}, fmt.Errorf("create manifest temp file: %w", err)
	}

	if _, err := io.Copy(out, decoded); err != nil {
		_ = out.Close()
		cleanupAll()

		return "", func() {}, fmt.Errorf("copy manifest into temp file: %w", err)
	}

	if err := out.Close(); err != nil {
		cleanupAll()

		return "", func() {}, fmt.Errorf("close manifest temp file: %w", err)
	}

	// suppress unused-warning when ctx isn't used by callees yet —
	// future revisions of downloadParentManifest will respect ctx
	// for the GetFile call.
	_ = ctx

	return manifestPath, cleanupAll, nil
}

type parentManifestFetcher interface {
	GetFile(encryptor util_encryption.FieldEncryptor, fileName string) (io.ReadCloser, error)
}

func decryptParentManifest(reader io.Reader, spec IncrSpec) (io.Reader, error) {
	salt, err := base64.StdEncoding.DecodeString(spec.ParentEncryptionSalt)
	if err != nil {
		return nil, fmt.Errorf("decode parent salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(spec.ParentEncryptionIV)
	if err != nil {
		return nil, fmt.Errorf("decode parent nonce: %w", err)
	}

	dec, err := backup_encryption.NewDecryptionReader(reader, spec.MasterKey, spec.ParentBackupID, salt, nonce)
	if err != nil {
		return nil, fmt.Errorf("init decryption reader: %w", err)
	}

	return dec, nil
}

func newPgBasebackupIncrCommand(
	ctx context.Context,
	pgBin string,
	spec IncrSpec,
	creds *Credentials,
	manifestPath, label string,
) (*exec.Cmd, error) {
	if _, err := exec.LookPath(pgBin); err != nil {
		return nil, fmt.Errorf("pg_basebackup binary not found at %s: %w", pgBin, err)
	}

	args := []string{
		"--pgdata=-",
		"--format=tar",
		// --compress=client-zstd:5 — see comment in full.go newPgBasebackupCommand.
		"--compress=client-zstd:5",
		// --wal-method=fetch — see comment in full.go newPgBasebackupCommand.
		"--wal-method=fetch",
		"--checkpoint=fast",
		"--incremental=" + manifestPath,
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

	cmd.Cancel = func() error { return signalForGracefulCancel(cmd.Process) }
	cmd.WaitDelay = killAfterCancel

	return cmd, nil
}

func runIncrStream(
	ctx context.Context,
	spec IncrSpec,
	cmd *exec.Cmd,
	fileName string,
	start time.Time,
) (IncrResult, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return IncrResult{}, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return IncrResult{}, fmt.Errorf("stderr pipe: %w", err)
	}

	storageReader, storageWriter := io.Pipe()

	finalWriter, encryptionWriter, encSalt, encNonce, err := setupEncryption(
		storageWriter, spec.Encryption, spec.MasterKey, spec.Backup.ID,
	)
	if err != nil {
		_ = storageWriter.Close()

		return IncrResult{}, err
	}

	counter := NewByteCounter(finalWriter)

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	stopWatcher := WithByteStallWatcher(streamCtx, counter, ByteStallTimeout, func() {
		spec.Logger.Warn("byte-stall timeout tripped; terminating pg_basebackup --incremental",
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

		return IncrResult{}, fmt.Errorf("start pg_basebackup --incremental: %w", err)
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
		return incrFromStreamError(copyErr, stderrBytes, streamCtx), nil
	}

	if waitErr != nil {
		return incrFromStreamError(waitErr, stderrBytes, streamCtx), nil
	}

	if saveErr != nil {
		reason := physical_enums.PhysicalBackupErrorStorageUploadFailed

		return IncrResult{
			Status:       physical_enums.PhysicalBackupStatusError,
			ErrorReason:  &reason,
			ErrorMessage: fmt.Sprintf("save to storage: %v", saveErr),
		}, nil
	}

	startLSN, stopLSN, timelineID, err := parseLsnsFromStderr(stderrBytes)
	if err != nil {
		reason := physical_enums.PhysicalBackupErrorManifestCorrupted

		return IncrResult{
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

	return IncrResult{
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

func incrFromStreamError(streamErr error, stderr []byte, ctx context.Context) IncrResult {
	if errors.Is(ctx.Err(), context.Canceled) {
		reason := physical_enums.PhysicalBackupErrorNetworkStallTimeout

		return IncrResult{
			Status:       physical_enums.PhysicalBackupStatusError,
			ErrorReason:  &reason,
			ErrorMessage: "byte-stall watcher cancelled pg_basebackup --incremental",
		}
	}

	if isSummariesExpiredError(stderr) {
		reason := physical_enums.PhysicalBackupErrorSummariesExpired

		return IncrResult{
			Status:       physical_enums.PhysicalBackupStatusChainBroken,
			ErrorReason:  &reason,
			ErrorMessage: fmt.Sprintf("%v; stderr: %s", streamErr, truncateStderr(stderr)),
		}
	}

	reason := physical_enums.PhysicalBackupErrorPgBasebackupFailed

	return IncrResult{
		Status:       physical_enums.PhysicalBackupStatusError,
		ErrorReason:  &reason,
		ErrorMessage: fmt.Sprintf("%v; stderr: %s", streamErr, truncateStderr(stderr)),
	}
}

// isSummariesExpiredError detects the post-PreCheck race where the source
// cluster pruned WAL summaries between PreCheck returning OK and
// pg_basebackup --incremental opening the actual range. PG surfaces this
// as "WAL summary file ... not found" or "could not open WAL summary".
func isSummariesExpiredError(stderr []byte) bool {
	msg := string(stderr)

	return containsAny(msg,
		"WAL summary file",
		"could not open WAL summary",
		"WAL summary not found",
	)
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if len(s) >= len(n) && stringIndex(s, n) >= 0 {
			return true
		}
	}

	return false
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}

	return -1
}

func buildIncrObjectName(
	db *postgresql_physical.PostgresqlPhysicalDatabase,
	backupID uuid.UUID,
	now time.Time,
) string {
	return fmt.Sprintf("%s-INCR-%s-%s.tar.zst",
		db.ID.String(),
		now.Format("20060102-150405"),
		backupID.String(),
	)
}

func incrErrorResult(
	reason physical_enums.PhysicalBackupErrorReason,
	stage string,
	err error,
	_ time.Time,
) IncrResult {
	r := reason

	return IncrResult{
		Status:       physical_enums.PhysicalBackupStatusError,
		ErrorReason:  &r,
		ErrorMessage: fmt.Sprintf("%s: %v", stage, err),
		CompletedAt:  time.Now().UTC(),
	}
}

// _ keeps chain_view referenced for future restore validators that share
// the ValidationResult shape.
var _ = chain_view.ValidationStatusOK
