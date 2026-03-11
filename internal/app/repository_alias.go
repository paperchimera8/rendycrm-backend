package app

import (
	"context"
	"database/sql"

	repo "github.com/vital/rendycrm-app/internal/repository"
)

type Repository struct {
	*repo.Repository
}

type slotRepairStats struct {
	relinkedBookings   int
	createdSlots       int
	fixedSlotDates     int
	freedOrphans       int
	mergedDuplicateIDs int
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{Repository: repo.NewRepository(db)}
}

func (s slotRepairStats) touched() bool {
	return s.relinkedBookings > 0 || s.createdSlots > 0 || s.fixedSlotDates > 0 || s.freedOrphans > 0 || s.mergedDuplicateIDs > 0
}

func (r *Repository) repairScheduleConsistency(ctx context.Context, workspaceID string) (slotRepairStats, error) {
	stats, err := r.Repository.RepairScheduleConsistencyStats(ctx, workspaceID)
	if err != nil {
		return slotRepairStats{}, err
	}
	return slotRepairStats{
		relinkedBookings:   stats.RelinkedBookings,
		createdSlots:       stats.CreatedSlots,
		fixedSlotDates:     stats.FixedSlotDates,
		freedOrphans:       stats.FreedOrphans,
		mergedDuplicateIDs: stats.MergedDuplicateIDs,
	}, nil
}
