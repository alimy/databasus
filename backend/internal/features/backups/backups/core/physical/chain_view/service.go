package chain_view

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	physical_enums "databasus-backend/internal/features/backups/backups/core/physical/enums"
	physical_models "databasus-backend/internal/features/backups/backups/core/physical/models"
	physical_repositories "databasus-backend/internal/features/backups/backups/core/physical/repositories"
	"databasus-backend/internal/storage"
	"databasus-backend/internal/util/walmath"
)

type ChainViewService struct {
	fullBackupRepository        *physical_repositories.PhysicalFullBackupRepository
	incrementalBackupRepository *physical_repositories.PhysicalIncrementalBackupRepository
	walSegmentRepository        *physical_repositories.PhysicalWalSegmentRepository
	walHistoryRepository        *physical_repositories.PhysicalWalHistoryRepository
}

func (s *ChainViewService) FindLastExtendableChainByDatabase(databaseID uuid.UUID) (*ChainView, error) {
	fulls, err := s.fullBackupRepository.FindCompletedNewestFirstByDatabase(databaseID)
	if err != nil {
		return nil, err
	}

	for _, full := range fulls {
		state, err := s.getChainState(full.ID)
		if err != nil {
			return nil, err
		}

		if state == ChainStateExtendable {
			return s.buildChainView(full)
		}
	}

	return nil, nil
}

func (s *ChainViewService) FindNonExtendableChainsByDatabase(databaseID uuid.UUID) ([]*ChainView, error) {
	fulls, err := s.fullBackupRepository.FindCompletedNewestFirstByDatabase(databaseID)
	if err != nil {
		return nil, err
	}

	views := make([]*ChainView, 0, len(fulls))
	sawActive := false

	for _, full := range fulls {
		state, err := s.getChainState(full.ID)
		if err != nil {
			return nil, err
		}

		if !sawActive && state == ChainStateExtendable {
			sawActive = true
			continue
		}

		view, err := s.buildChainView(full)
		if err != nil {
			return nil, err
		}

		views = append(views, view)
	}

	return views, nil
}

func (s *ChainViewService) GetChainSpan(rootFullBackupID uuid.UUID) (LSNRange, error) {
	full, err := s.fullBackupRepository.FindByID(rootFullBackupID)
	if err != nil {
		return LSNRange{}, err
	}
	if full == nil {
		return LSNRange{}, fmt.Errorf("chain_view: full backup not found: %s", rootFullBackupID)
	}
	if full.StartLSN == nil {
		return LSNRange{}, errors.New("chain_view: cannot compute span — root FULL has no start_lsn")
	}

	start := *full.StartLSN

	var successor physical_models.PhysicalFullBackup

	err = storage.
		GetDb().
		Where(
			"database_id = ? AND timeline_id = ? AND status = ? AND start_lsn > ?::pg_lsn",
			full.DatabaseID,
			full.TimelineID,
			physical_enums.PhysicalBackupStatusCompleted,
			start.String(),
		).
		Order("start_lsn ASC").
		First(&successor).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return LSNRange{Start: start, End: LSNMax}, nil
		}

		return LSNRange{}, err
	}

	end := LSNMax
	if successor.StartLSN != nil {
		end = *successor.StartLSN
	}

	return LSNRange{Start: start, End: end}, nil
}

func (s *ChainViewService) FindWalSegmentsInSpan(
	databaseID uuid.UUID,
	timelineID int,
	startLSN, endLSN walmath.LSN,
) ([]*physical_models.PhysicalWalSegment, error) {
	return s.walSegmentRepository.FindByChainSpan(databaseID, timelineID, startLSN, endLSN)
}

func (s *ChainViewService) FindWalGapsInChain(rootFullBackupID uuid.UUID) ([]LSNRange, error) {
	full, err := s.fullBackupRepository.FindByID(rootFullBackupID)
	if err != nil {
		return nil, err
	}
	if full == nil {
		return nil, fmt.Errorf("chain_view: full backup not found: %s", rootFullBackupID)
	}

	span, err := s.GetChainSpan(rootFullBackupID)
	if err != nil {
		return nil, err
	}

	segments, err := s.walSegmentRepository.FindByChainSpan(
		full.DatabaseID, full.TimelineID, span.Start, span.End,
	)
	if err != nil {
		return nil, err
	}

	if len(segments) < 2 {
		return nil, nil
	}

	gaps := make([]LSNRange, 0)

	for i := 1; i < len(segments); i++ {
		prev := segments[i-1]
		curr := segments[i]

		if curr.StartLSN > prev.EndLSN {
			gaps = append(gaps, LSNRange{
				Start: prev.EndLSN,
				End:   curr.StartLSN,
			})
		}
	}

	return gaps, nil
}

func (s *ChainViewService) CheckHistoryFilePresence(
	databaseID uuid.UUID,
	timelineID int,
) (ValidationResult, error) {
	histories, err := s.walHistoryRepository.FindAllByDatabase(databaseID)
	if err != nil {
		return ValidationResult{}, err
	}

	if len(histories) == 0 {
		return ValidationResult{Status: ValidationStatusOK}, nil
	}

	for _, h := range histories {
		if h.TimelineID == timelineID {
			return ValidationResult{Status: ValidationStatusOK}, nil
		}
	}

	return ValidationResult{
		Status:  ValidationStatusOKWithWarning,
		Message: fmt.Sprintf("no history file for timeline %d", timelineID),
	}, nil
}

func (s *ChainViewService) GetChainEndTimestamp(rootFullBackupID uuid.UUID) (time.Time, error) {
	full, err := s.fullBackupRepository.FindByID(rootFullBackupID)
	if err != nil {
		return time.Time{}, err
	}
	if full == nil {
		return time.Time{}, fmt.Errorf("chain_view: full backup not found: %s", rootFullBackupID)
	}

	latest := time.Time{}
	if full.CompletedAt != nil {
		latest = *full.CompletedAt
	}

	incrs, err := s.incrementalBackupRepository.FindAllByRootFull(rootFullBackupID)
	if err != nil {
		return time.Time{}, err
	}

	for _, incr := range incrs {
		if incr.CompletedAt != nil && incr.CompletedAt.After(latest) {
			latest = *incr.CompletedAt
		}
	}

	span, err := s.GetChainSpan(rootFullBackupID)
	if err != nil {
		return time.Time{}, err
	}

	segments, err := s.walSegmentRepository.FindByChainSpan(
		full.DatabaseID, full.TimelineID, span.Start, span.End,
	)
	if err != nil {
		return time.Time{}, err
	}

	for _, seg := range segments {
		if seg.ReceivedAt.After(latest) {
			latest = seg.ReceivedAt
		}
	}

	return latest, nil
}

func (s *ChainViewService) FindWalOrphansByDatabase(databaseID uuid.UUID) ([]WalOrphanRef, error) {
	walOrphans, err := s.walSegmentRepository.FindOrphans(databaseID)
	if err != nil {
		return nil, err
	}

	refs := make([]WalOrphanRef, 0, len(walOrphans))
	for _, w := range walOrphans {
		refs = append(refs, WalOrphanRef{WalSegment: w})
	}

	return refs, nil
}

// Precedence:
//  1. A newer COMPLETED FULL on the same database  → CLOSED_BY_NEWER_FULL.
//  2. Any downstream INCR with status=CHAIN_BROKEN → BROKEN_BY_INCR.
//  3. Otherwise                                    → EXTENDABLE.
func (s *ChainViewService) getChainState(rootFullBackupID uuid.UUID) (ChainState, error) {
	full, err := s.fullBackupRepository.FindByID(rootFullBackupID)
	if err != nil {
		return "", err
	}
	if full == nil {
		return "", fmt.Errorf("chain_view: full backup not found: %s", rootFullBackupID)
	}

	newerFulls, err := s.findNewerCompletedFulls(full)
	if err != nil {
		return "", err
	}
	if len(newerFulls) > 0 {
		return ChainStateClosedByNewerFull, nil
	}

	incrs, err := s.incrementalBackupRepository.FindAllByRootFull(rootFullBackupID)
	if err != nil {
		return "", err
	}

	for _, incr := range incrs {
		if incr.Status == physical_enums.PhysicalBackupStatusChainBroken {
			return ChainStateBrokenByIncr, nil
		}
	}

	return ChainStateExtendable, nil
}

func (s *ChainViewService) findNewerCompletedFulls(
	full *physical_models.PhysicalFullBackup,
) ([]*physical_models.PhysicalFullBackup, error) {
	all, err := s.fullBackupRepository.FindCompletedNewestFirstByDatabase(full.DatabaseID)
	if err != nil {
		return nil, err
	}

	newer := make([]*physical_models.PhysicalFullBackup, 0)
	for _, candidate := range all {
		if candidate.ID == full.ID {
			continue
		}

		if candidate.CreatedAt.After(full.CreatedAt) {
			newer = append(newer, candidate)
		}
	}

	return newer, nil
}

func (s *ChainViewService) buildChainView(
	full *physical_models.PhysicalFullBackup,
) (*ChainView, error) {
	span, err := s.GetChainSpan(full.ID)
	if err != nil {
		return nil, err
	}

	incrs, err := s.incrementalBackupRepository.FindAllByRootFull(full.ID)
	if err != nil {
		return nil, err
	}

	segments, err := s.walSegmentRepository.FindByChainSpan(
		full.DatabaseID, full.TimelineID, span.Start, span.End,
	)
	if err != nil {
		return nil, err
	}

	gaps, err := s.FindWalGapsInChain(full.ID)
	if err != nil {
		return nil, err
	}

	maxReplayable := walmath.LSN(0)
	if full.StopLSN != nil {
		maxReplayable = *full.StopLSN
	}

	for _, seg := range segments {
		segEnd := seg.EndLSN

		insideGap := false
		for _, gap := range gaps {
			if segEnd > gap.Start {
				insideGap = true
				break
			}
		}
		if insideGap {
			break
		}

		if segEnd > maxReplayable {
			maxReplayable = segEnd
		}
	}

	return &ChainView{
		RootFull:         full,
		Incrementals:     incrs,
		WalSegments:      segments,
		Gaps:             gaps,
		Span:             span,
		MaxReplayableLSN: maxReplayable,
	}, nil
}
