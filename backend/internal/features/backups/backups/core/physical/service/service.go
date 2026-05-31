package physical_service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"databasus-backend/internal/features/backups/backups/core/physical/chain_view"
	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	"databasus-backend/internal/features/storages"
	"databasus-backend/internal/storage"
	util_encryption "databasus-backend/internal/util/encryption"
)

const (
	// Sidecar suffix mirrors the executor's metadata convention; every FULL /
	// INCR / WAL / history artifact has a sibling "<name>.metadata".
	metadataSuffix = ".metadata"

	// One DELETE transaction never removes more than this many WAL rows per
	// batch, so the anchor-FULL lock is not held across an unbounded delete.
	maxWalDeleteBatchRows = 50
)

// PhysicalBackupService owns the transactional, multi-table operations over the
// physical catalog that the cleaner needs: chain-cascade deletion (file before
// row, leaves before parents), usage accounting, and orphan-WAL deletion. It is
// the single seam through which the cleaner mutates catalog rows — the cleaner
// never issues raw DELETEs.
type PhysicalBackupService struct {
	fullBackupRepository        *physical_repositories.PhysicalFullBackupRepository
	incrementalBackupRepository *physical_repositories.PhysicalIncrementalBackupRepository
	walHistoryRepository        *physical_repositories.PhysicalWalHistoryRepository
	chainViewService            *chain_view.ChainViewService
	storageService              *storages.StorageService
	fieldEncryptor              util_encryption.FieldEncryptor
	logger                      *slog.Logger
}

// DeleteFull cascades one chain's deletion bounded by walByteBudgetMB. The
// order is WAL (oldest LSN first) → INCRs (leaves first) → orphaned history →
// the FULL itself, each object deleted from storage before its row. Idempotent:
// a missing FULL is a no-op (a peer already deleted it); a budget-capped call
// returns ChainFullyDeleted=false and the caller resumes next tick.
func (s *PhysicalBackupService) DeleteFull(
	ctx context.Context,
	rootFullBackupID uuid.UUID,
	walByteBudgetMB float64,
) (DeletedSummary, error) {
	return s.cascadeDelete(ctx, rootFullBackupID, walByteBudgetMB, false)
}

// DeleteChainDependentsKeepFull strips a chain's INCRs and WAL but leaves the
// FULL row (and its history) in place — the FULL_BACKUPS LAST_N policy keeps the
// base FULL while shedding everything downstream of it.
func (s *PhysicalBackupService) DeleteChainDependentsKeepFull(
	ctx context.Context,
	rootFullBackupID uuid.UUID,
	walByteBudgetMB float64,
) (DeletedSummary, error) {
	return s.cascadeDelete(ctx, rootFullBackupID, walByteBudgetMB, true)
}

// DeleteWalSegmentsInSpan deletes orphan WAL — segments with no anchoring FULL —
// for one (database, timeline) over an LSN span, budget-bounded. There is no
// FULL to lock, so each batch is SELECT ... FOR UPDATE on the WAL rows directly.
func (s *PhysicalBackupService) DeleteWalSegmentsInSpan(
	ctx context.Context,
	databaseID uuid.UUID,
	timelineID int,
	span chain_view.LSNRange,
	walByteBudgetMB float64,
) (int, float64, error) {
	var deletedRows int
	var deletedMB float64

	txErr := storage.GetDb().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, mb, _, err := s.deleteWalInSpanBudgeted(tx, databaseID, timelineID, span, walByteBudgetMB, true)
		deletedRows = rows
		deletedMB = mb

		return err
	})
	if txErr != nil {
		return 0, 0, txErr
	}

	return deletedRows, deletedMB, nil
}

// GetDependentsSummary counts a chain's dependents and total size without
// deleting anything. Idempotent and read-only.
func (s *PhysicalBackupService) GetDependentsSummary(rootFullBackupID uuid.UUID) (DependentsSummary, error) {
	full, err := s.fullBackupRepository.FindByID(rootFullBackupID)
	if err != nil {
		return DependentsSummary{}, err
	}
	if full == nil {
		return DependentsSummary{}, ErrFullNotFound
	}

	span, err := s.chainViewService.GetChainSpan(rootFullBackupID)
	if err != nil {
		return DependentsSummary{}, err
	}

	segments, err := s.chainViewService.FindWalSegmentsInSpan(full.DatabaseID, full.TimelineID, span.Start, span.End)
	if err != nil {
		return DependentsSummary{}, err
	}

	incrementals, err := s.incrementalBackupRepository.FindAllByRootFull(rootFullBackupID)
	if err != nil {
		return DependentsSummary{}, err
	}

	historyFiles, err := s.walHistoryRepository.FindAllByDatabase(full.DatabaseID)
	if err != nil {
		return DependentsSummary{}, err
	}

	summary := DependentsSummary{
		RootFullBackupID: rootFullBackupID,
		WalSegments:      len(segments),
		Incrementals:     len(incrementals),
	}

	if full.BackupSizeMb != nil {
		summary.TotalSizeMB += *full.BackupSizeMb
	}

	for _, segment := range segments {
		summary.TotalSizeMB += segment.CompressedSizeMb
	}

	for _, incremental := range incrementals {
		if incremental.BackupSizeMb != nil {
			summary.TotalSizeMB += *incremental.BackupSizeMb
		}
	}

	for _, historyFile := range historyFiles {
		if historyFile.TimelineID == full.TimelineID {
			summary.HistoryFiles++
			summary.TotalSizeMB += historyFile.CompressedSizeMb
		}
	}

	return summary, nil
}

// GetTotalUsageMBByDatabase sums every COMPLETED FULL + COMPLETED INCR backup
// size and every WAL segment's compressed size for one database. Failed /
// canceled rows (NULL backup_size_mb) are excluded by the status filter.
func (s *PhysicalBackupService) GetTotalUsageMBByDatabase(databaseID uuid.UUID) (float64, error) {
	var totalMB float64

	err := storage.GetDb().Raw(`
		SELECT
		    COALESCE((SELECT SUM(backup_size_mb) FROM physical_full_backups
		              WHERE database_id = ? AND status = ?), 0)
		  + COALESCE((SELECT SUM(backup_size_mb) FROM physical_incremental_backups
		              WHERE database_id = ? AND status = ?), 0)
		  + COALESCE((SELECT SUM(compressed_size_mb) FROM physical_wal_segments
		              WHERE database_id = ?), 0)
	`,
		databaseID, physical_enums.PhysicalBackupStatusCompleted,
		databaseID, physical_enums.PhysicalBackupStatusCompleted,
		databaseID,
	).Scan(&totalMB).Error

	return totalMB, err
}

func (s *PhysicalBackupService) cascadeDelete(
	ctx context.Context,
	rootFullBackupID uuid.UUID,
	walByteBudgetMB float64,
	keepFull bool,
) (DeletedSummary, error) {
	summary := DeletedSummary{RootFullBackupID: rootFullBackupID}

	txErr := storage.GetDb().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var full physical_models.PhysicalFullBackup

		// FOR UPDATE serializes with concurrent cleaner instances and the WAL
		// uploader (which locks the same anchor FULL before its INSERT).
		lockErr := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", rootFullBackupID).
			First(&full).Error
		if errors.Is(lockErr, gorm.ErrRecordNotFound) {
			s.logger.Debug("chain already deleted by peer", "root_full_backup_id", rootFullBackupID)

			return nil
		}
		if lockErr != nil {
			return lockErr
		}

		span, err := s.chainViewService.GetChainSpan(rootFullBackupID)
		if err != nil {
			return err
		}

		walRows, walBytes, budgetHit, err := s.deleteWalInSpanBudgeted(
			tx, full.DatabaseID, full.TimelineID, span, walByteBudgetMB, false,
		)
		summary.WalSegments = walRows
		summary.BytesDeletedMB = walBytes
		if err != nil {
			return err
		}

		if budgetHit {
			// WAL budget exhausted before the chain emptied — commit the partial
			// deletion; the next tick re-queries the now-oldest rows.
			return nil
		}

		incrementalCount, err := s.deleteIncrementals(tx, rootFullBackupID)
		summary.Incrementals = incrementalCount
		if err != nil {
			return err
		}

		if keepFull {
			return nil
		}

		historyCount, err := s.deleteOrphanedHistoryFiles(tx, &full)
		summary.HistoryFiles = historyCount
		if err != nil {
			return err
		}

		if err := s.deleteFullArtifactAndRow(tx, &full); err != nil {
			return err
		}

		summary.ChainFullyDeleted = true

		return nil
	})
	if txErr != nil {
		return DeletedSummary{RootFullBackupID: rootFullBackupID}, txErr
	}

	return summary, nil
}

// deleteWalInSpanBudgeted removes WAL rows in the span oldest-LSN first, batched,
// stopping when the byte budget is reached or the span is drained. Storage
// deletes are fail-closed: a transient DeleteFile error aborts the batch (the
// rows survive and retry) rather than orphaning an object. lockRows takes a
// row-level FOR UPDATE on each batch (orphan path, no anchor FULL to serialize
// on); when false the caller already holds the anchor FULL lock. Returns rows
// deleted, MB deleted, and whether the budget capped the run (more rows remain).
func (s *PhysicalBackupService) deleteWalInSpanBudgeted(
	tx *gorm.DB,
	databaseID uuid.UUID,
	timelineID int,
	span chain_view.LSNRange,
	walByteBudgetMB float64,
	lockRows bool,
) (deletedRows int, deletedMB float64, budgetHit bool, err error) {
	for {
		var segments []*physical_models.PhysicalWalSegment

		batchQuery := tx.
			Where(
				"database_id = ? AND timeline_id = ? AND start_lsn >= ?::pg_lsn AND start_lsn < ?::pg_lsn",
				databaseID, timelineID, span.Start.String(), span.End.String(),
			).
			Order("start_lsn ASC").
			Limit(maxWalDeleteBatchRows)

		if lockRows {
			batchQuery = batchQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}

		if err = batchQuery.Find(&segments).Error; err != nil {
			return deletedRows, deletedMB, false, err
		}

		if len(segments) == 0 {
			return deletedRows, deletedMB, false, nil
		}

		for _, segment := range segments {
			if segment.FileName != nil {
				if delErr := s.deleteWalObjectFailClosed(segment.StorageID, *segment.FileName); delErr != nil {
					return deletedRows, deletedMB, false, delErr
				}

				if delErr := s.deleteWalObjectFailClosed(
					segment.StorageID,
					*segment.FileName+metadataSuffix,
				); delErr != nil {
					return deletedRows, deletedMB, false, delErr
				}
			}

			if err = tx.Delete(&physical_models.PhysicalWalSegment{}, "id = ?", segment.ID).Error; err != nil {
				return deletedRows, deletedMB, false, err
			}

			deletedRows++
			deletedMB += segment.CompressedSizeMb

			// Soft byte cap, checked per row: stop as soon as the budget is
			// reached so a sub-batch span cannot blow past it. Possibly-more
			// rows remain, so report budgetHit=true.
			if deletedMB >= walByteBudgetMB {
				return deletedRows, deletedMB, true, nil
			}
		}

		if len(segments) < maxWalDeleteBatchRows {
			return deletedRows, deletedMB, false, nil
		}
	}
}

func (s *PhysicalBackupService) deleteIncrementals(tx *gorm.DB, rootFullBackupID uuid.UUID) (int, error) {
	var incrementals []*physical_models.PhysicalIncrementalBackup

	if err := tx.
		Where("root_full_backup_id = ?", rootFullBackupID).
		Find(&incrementals).Error; err != nil {
		return 0, err
	}

	deleted := 0

	for _, incremental := range reverseTopoOrderIncrementals(incrementals) {
		if incremental.FileName != nil {
			s.deleteStorageObjectFailOpen(incremental.StorageID, *incremental.FileName+metadataSuffix)

			if incremental.ManifestFileName != nil {
				s.deleteStorageObjectFailOpen(incremental.StorageID, *incremental.ManifestFileName)
			}

			s.deleteStorageObjectFailOpen(incremental.StorageID, *incremental.FileName)
		}

		if err := tx.Delete(&physical_models.PhysicalIncrementalBackup{}, "id = ?", incremental.ID).Error; err != nil {
			return deleted, err
		}

		deleted++
	}

	return deleted, nil
}

// deleteOrphanedHistoryFiles drops the history files of the FULL's timeline only
// when no other COMPLETED FULL survives on that timeline — otherwise the history
// still anchors a living chain.
func (s *PhysicalBackupService) deleteOrphanedHistoryFiles(
	tx *gorm.DB,
	full *physical_models.PhysicalFullBackup,
) (int, error) {
	var survivingFulls int64

	if err := tx.
		Model(&physical_models.PhysicalFullBackup{}).
		Where("database_id = ? AND timeline_id = ? AND status = ? AND id != ?",
			full.DatabaseID, full.TimelineID, physical_enums.PhysicalBackupStatusCompleted, full.ID).
		Count(&survivingFulls).Error; err != nil {
		return 0, err
	}

	if survivingFulls > 0 {
		return 0, nil
	}

	var historyFiles []*physical_models.PhysicalWalHistoryFile

	if err := tx.
		Where("database_id = ? AND timeline_id = ?", full.DatabaseID, full.TimelineID).
		Find(&historyFiles).Error; err != nil {
		return 0, err
	}

	deleted := 0

	for _, historyFile := range historyFiles {
		s.deleteStorageObjectFailOpen(historyFile.StorageID, historyFile.FileName+metadataSuffix)
		s.deleteStorageObjectFailOpen(historyFile.StorageID, historyFile.FileName)

		if err := tx.Delete(&physical_models.PhysicalWalHistoryFile{}, "id = ?", historyFile.ID).Error; err != nil {
			return deleted, err
		}

		deleted++
	}

	return deleted, nil
}

func (s *PhysicalBackupService) deleteFullArtifactAndRow(
	tx *gorm.DB,
	full *physical_models.PhysicalFullBackup,
) error {
	if full.FileName != nil {
		s.deleteStorageObjectFailOpen(full.StorageID, *full.FileName+metadataSuffix)

		if full.ManifestFileName != nil {
			s.deleteStorageObjectFailOpen(full.StorageID, *full.ManifestFileName)
		}

		s.deleteStorageObjectFailOpen(full.StorageID, *full.FileName)
	}

	return tx.Delete(&physical_models.PhysicalFullBackup{}, "id = ?", full.ID).Error
}

// deleteStorageObjectFailOpen deletes one object, logging and continuing on any
// failure. Used for FULL / INCR / history objects: a transient storage error
// must not block the row delete. storage.DeleteFile is idempotent on not-found.
func (s *PhysicalBackupService) deleteStorageObjectFailOpen(storageID uuid.UUID, fileName string) {
	backupStorage, err := s.storageService.GetStorageByID(storageID)
	if err != nil {
		s.logger.Error("failed to resolve storage for object delete",
			"storage_id", storageID, "file_name", fileName, "error", err)

		return
	}

	if err := backupStorage.DeleteFile(s.fieldEncryptor, fileName); err != nil {
		s.logger.Error("failed to delete storage object", "file_name", fileName, "error", err)
	}
}

// deleteWalObjectFailClosed deletes one WAL object fail-closed: a transient
// DeleteFile error is returned so the caller rolls back the batch and retries,
// never orphaning a WAL object with no catalog row. A permanently-removed
// storage (the storage row itself is gone) is fail-open — the object is
// unreachable forever, so the row may be deleted.
func (s *PhysicalBackupService) deleteWalObjectFailClosed(storageID uuid.UUID, fileName string) error {
	backupStorage, err := s.storageService.GetStorageByID(storageID)
	if err != nil {
		s.logger.Warn("storage not found for WAL object delete; removing row anyway",
			"storage_id", storageID, "file_name", fileName, "error", err)

		return nil
	}

	if err := backupStorage.DeleteFile(s.fieldEncryptor, fileName); err != nil {
		return fmt.Errorf("delete WAL object %s: %w", fileName, err)
	}

	return nil
}

// reverseTopoOrderIncrementals orders incrementals leaves-first: an INCR is
// emitted before any INCR it names as parent, so deleting in this order respects
// the RESTRICT FK on parent_incremental_backup_id. Linear chains collapse to
// newest-first; the explicit walk also covers the rare branched case where two
// INCRs share one parent.
func reverseTopoOrderIncrementals(
	incrementals []*physical_models.PhysicalIncrementalBackup,
) []*physical_models.PhysicalIncrementalBackup {
	present := make(map[uuid.UUID]bool, len(incrementals))
	childCount := make(map[uuid.UUID]int, len(incrementals))

	for _, incremental := range incrementals {
		present[incremental.ID] = true
	}

	for _, incremental := range incrementals {
		if incremental.ParentIncrementalBackupID != nil && present[*incremental.ParentIncrementalBackupID] {
			childCount[*incremental.ParentIncrementalBackupID]++
		}
	}

	ordered := make([]*physical_models.PhysicalIncrementalBackup, 0, len(incrementals))
	remaining := incrementals

	for len(remaining) > 0 {
		var stillBlocked []*physical_models.PhysicalIncrementalBackup
		progressed := false

		for _, incremental := range remaining {
			if childCount[incremental.ID] > 0 {
				stillBlocked = append(stillBlocked, incremental)

				continue
			}

			ordered = append(ordered, incremental)
			progressed = true

			if incremental.ParentIncrementalBackupID != nil && present[*incremental.ParentIncrementalBackupID] {
				childCount[*incremental.ParentIncrementalBackupID]--
			}
		}

		if !progressed {
			// RESTRICT FKs make a cycle impossible; never loop forever anyway.
			ordered = append(ordered, stillBlocked...)

			break
		}

		remaining = stillBlocked
	}

	return ordered
}
