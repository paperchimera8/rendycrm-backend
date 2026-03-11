package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

type slotRepairStats struct {
	relinkedBookings   int
	createdSlots       int
	fixedSlotDates     int
	freedOrphans       int
	mergedDuplicateIDs int
}

func (s slotRepairStats) touched() bool {
	return s.relinkedBookings > 0 || s.createdSlots > 0 || s.fixedSlotDates > 0 || s.freedOrphans > 0 || s.mergedDuplicateIDs > 0
}

func (r *Repository) workspaceLocation(ctx context.Context, workspaceID string) (*time.Location, error) {
	return r.workspaceLocationTx(ctx, nil, workspaceID)
}

func (r *Repository) workspaceLocationTx(ctx context.Context, tx *sql.Tx, workspaceID string) (*time.Location, error) {
	queryer := interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}(r.db)
	if tx != nil {
		queryer = tx
	}
	var timezone string
	if err := queryer.QueryRowContext(ctx, `
		SELECT COALESCE(ss.timezone, w.timezone, 'UTC')
		FROM workspaces w
		LEFT JOIN slot_settings ss ON ss.workspace_id = w.id
		WHERE w.id = $1
	`, workspaceID).Scan(&timezone); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

func slotDateInLocation(startsAt time.Time, loc *time.Location) string {
	return startsAt.In(loc).Format("2006-01-02")
}

func (r *Repository) slotDateForTimeTx(ctx context.Context, tx *sql.Tx, workspaceID string, startsAt time.Time) (string, error) {
	loc, err := r.workspaceLocationTx(ctx, tx, workspaceID)
	if err != nil {
		return "", err
	}
	return slotDateInLocation(startsAt, loc), nil
}

func (r *Repository) slotDateForTime(ctx context.Context, workspaceID string, startsAt time.Time) (string, error) {
	loc, err := r.workspaceLocation(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return slotDateInLocation(startsAt, loc), nil
}

func (r *Repository) syncSlotStatusTx(ctx context.Context, tx *sql.Tx, workspaceID, slotID string) (bool, error) {
	if slotID == "" {
		return false, nil
	}
	var current DailySlotStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, slotID, workspaceID).Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	var (
		confirmedCount int
		pendingCount   int
		activeHolds    int
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('confirmed', 'completed')),
			COUNT(*) FILTER (WHERE status = 'pending')
		FROM bookings
		WHERE workspace_id = $1 AND daily_slot_id = $2 AND status <> 'cancelled'
	`, workspaceID, slotID).Scan(&confirmedCount, &pendingCount); err != nil {
		return false, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM slot_holds
		WHERE workspace_id = $1 AND daily_slot_id = $2 AND expires_at > NOW()
	`, workspaceID, slotID).Scan(&activeHolds); err != nil {
		return false, err
	}

	next := current
	switch {
	case confirmedCount > 0:
		next = DailySlotBooked
	case pendingCount > 0 || activeHolds > 0:
		next = DailySlotHeld
	case current == DailySlotBlocked:
		next = DailySlotBlocked
	default:
		next = DailySlotFree
	}

	if next == current {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE daily_slots
		SET status = $3
		WHERE id = $1 AND workspace_id = $2
	`, slotID, workspaceID, next); err != nil {
		return false, err
	}
	return current != DailySlotFree && next == DailySlotFree, nil
}

func (r *Repository) syncSlotsTx(ctx context.Context, tx *sql.Tx, workspaceID string, slotIDs ...string) (int, error) {
	seen := map[string]struct{}{}
	freed := 0
	for _, slotID := range slotIDs {
		if slotID == "" {
			continue
		}
		if _, ok := seen[slotID]; ok {
			continue
		}
		seen[slotID] = struct{}{}
		wasFreed, err := r.syncSlotStatusTx(ctx, tx, workspaceID, slotID)
		if err != nil {
			return freed, err
		}
		if wasFreed {
			freed++
		}
	}
	return freed, nil
}

func (r *Repository) repairScheduleConsistency(ctx context.Context, workspaceID string) (slotRepairStats, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return slotRepairStats{}, err
	}
	defer tx.Rollback()

	stats, err := r.repairScheduleConsistencyTx(ctx, tx, workspaceID)
	if err != nil {
		return slotRepairStats{}, err
	}
	if err := tx.Commit(); err != nil {
		return slotRepairStats{}, err
	}
	if stats.touched() {
		log.Printf(
			"schedule repair workspace=%s relinked_bookings=%d created_slots=%d fixed_slot_dates=%d freed_orphans=%d merged_duplicate_slots=%d",
			workspaceID,
			stats.relinkedBookings,
			stats.createdSlots,
			stats.fixedSlotDates,
			stats.freedOrphans,
			stats.mergedDuplicateIDs,
		)
	}
	return stats, nil
}

func (r *Repository) RepairScheduleConsistency(ctx context.Context, workspaceID string) error {
	_, err := r.repairScheduleConsistency(ctx, workspaceID)
	return err
}

type RepairScheduleStats struct {
	RelinkedBookings   int
	CreatedSlots       int
	FixedSlotDates     int
	FreedOrphans       int
	MergedDuplicateIDs int
}

func (r *Repository) RepairScheduleConsistencyStats(ctx context.Context, workspaceID string) (RepairScheduleStats, error) {
	stats, err := r.repairScheduleConsistency(ctx, workspaceID)
	if err != nil {
		return RepairScheduleStats{}, err
	}
	return RepairScheduleStats{
		RelinkedBookings:   stats.relinkedBookings,
		CreatedSlots:       stats.createdSlots,
		FixedSlotDates:     stats.fixedSlotDates,
		FreedOrphans:       stats.freedOrphans,
		MergedDuplicateIDs: stats.mergedDuplicateIDs,
	}, nil
}

func (r *Repository) repairScheduleConsistencyTx(ctx context.Context, tx *sql.Tx, workspaceID string) (slotRepairStats, error) {
	var stats slotRepairStats

	type slotRow struct {
		id       string
		startsAt time.Time
		slotDate string
	}
	slotRows, err := tx.QueryContext(ctx, `
		SELECT id, starts_at, slot_date::text
		FROM daily_slots
		WHERE workspace_id = $1
		ORDER BY starts_at ASC, id ASC
		FOR UPDATE
	`, workspaceID)
	if err != nil {
		return stats, err
	}
	var slots []slotRow
	for slotRows.Next() {
		var row slotRow
		if err := slotRows.Scan(&row.id, &row.startsAt, &row.slotDate); err != nil {
			slotRows.Close()
			return stats, err
		}
		slots = append(slots, row)
	}
	if err := slotRows.Err(); err != nil {
		slotRows.Close()
		return stats, err
	}
	slotRows.Close()

	for _, slot := range slots {
		expectedDate, err := r.slotDateForTimeTx(ctx, tx, workspaceID, slot.startsAt)
		if err != nil {
			return stats, err
		}
		if slot.slotDate == expectedDate {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE daily_slots
			SET slot_date = $3
			WHERE id = $1 AND workspace_id = $2
		`, slot.id, workspaceID, expectedDate); err != nil {
			return stats, err
		}
		stats.fixedSlotDates++
	}

	type duplicateGroup struct {
		startsAt time.Time
		endsAt   time.Time
	}
	dupRows, err := tx.QueryContext(ctx, `
		SELECT starts_at, ends_at
		FROM daily_slots
		WHERE workspace_id = $1
		GROUP BY starts_at, ends_at
		HAVING COUNT(*) > 1
	`, workspaceID)
	if err != nil {
		return stats, err
	}
	var duplicates []duplicateGroup
	for dupRows.Next() {
		var group duplicateGroup
		if err := dupRows.Scan(&group.startsAt, &group.endsAt); err != nil {
			dupRows.Close()
			return stats, err
		}
		duplicates = append(duplicates, group)
	}
	if err := dupRows.Err(); err != nil {
		dupRows.Close()
		return stats, err
	}
	dupRows.Close()

	for _, group := range duplicates {
		rows, err := tx.QueryContext(ctx, `
			SELECT id
			FROM daily_slots
			WHERE workspace_id = $1 AND starts_at = $2 AND ends_at = $3
			ORDER BY created_at ASC, id ASC
			FOR UPDATE
		`, workspaceID, group.startsAt, group.endsAt)
		if err != nil {
			return stats, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return stats, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return stats, err
		}
		rows.Close()
		if len(ids) < 2 {
			continue
		}
		keepID := ids[0]
		for _, duplicateID := range ids[1:] {
			if _, err := tx.ExecContext(ctx, `
				UPDATE bookings
				SET daily_slot_id = $3
				WHERE workspace_id = $1 AND daily_slot_id = $2
			`, workspaceID, duplicateID, keepID); err != nil {
				return stats, err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE slot_holds
				SET daily_slot_id = $3
				WHERE workspace_id = $1 AND daily_slot_id = $2
			`, workspaceID, duplicateID, keepID); err != nil {
				return stats, err
			}
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM daily_slots
				WHERE id = $1 AND workspace_id = $2
			`, duplicateID, workspaceID); err != nil {
				return stats, err
			}
			stats.mergedDuplicateIDs++
		}
	}

	type bookingRow struct {
		id         string
		startsAt   time.Time
		endsAt     time.Time
		notes      string
		dailySlot  string
		slotHoldID string
	}
	bookingRows, err := tx.QueryContext(ctx, `
		SELECT id, starts_at, ends_at, notes, COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, '')
		FROM bookings
		WHERE workspace_id = $1 AND status <> 'cancelled'
		ORDER BY starts_at ASC, id ASC
		FOR UPDATE
	`, workspaceID)
	if err != nil {
		return stats, err
	}
	var bookings []bookingRow
	for bookingRows.Next() {
		var row bookingRow
		if err := bookingRows.Scan(&row.id, &row.startsAt, &row.endsAt, &row.notes, &row.dailySlot, &row.slotHoldID); err != nil {
			bookingRows.Close()
			return stats, err
		}
		bookings = append(bookings, row)
	}
	if err := bookingRows.Err(); err != nil {
		bookingRows.Close()
		return stats, err
	}
	bookingRows.Close()

	for _, booking := range bookings {
		exactSlotExists := true
		if _, _, err := r.exactSlotTx(ctx, tx, workspaceID, booking.startsAt, booking.endsAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				exactSlotExists = false
			} else {
				return stats, err
			}
		}
		slotID, err := r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, booking.id, booking.dailySlot, booking.startsAt, booking.endsAt)
		if err != nil {
			return stats, err
		}
		if !exactSlotExists {
			stats.createdSlots++
		}
		if booking.dailySlot != slotID {
			if _, err := tx.ExecContext(ctx, `
				UPDATE bookings
				SET daily_slot_id = $3
				WHERE id = $1 AND workspace_id = $2
			`, booking.id, workspaceID, slotID); err != nil {
				return stats, err
			}
			stats.relinkedBookings++
		}

		if booking.slotHoldID != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE slot_holds
				SET daily_slot_id = $3
				WHERE id = $1 AND workspace_id = $2
			`, booking.slotHoldID, workspaceID, slotID); err != nil {
				return stats, err
			}
		}
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM daily_slots
		WHERE workspace_id = $1
	`, workspaceID)
	if err != nil {
		return stats, err
	}
	var allSlotIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return stats, err
		}
		allSlotIDs = append(allSlotIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()

	freed, err := r.syncSlotsTx(ctx, tx, workspaceID, allSlotIDs...)
	if err != nil {
		return stats, err
	}
	stats.freedOrphans += freed

	if err := r.rebalanceSlotPositionsTx(ctx, tx, workspaceID); err != nil {
		return stats, err
	}

	return stats, nil
}

func (r *Repository) rebalanceSlotPositionsTx(ctx context.Context, tx *sql.Tx, workspaceID string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, slot_date::text
		FROM daily_slots
		WHERE workspace_id = $1
		ORDER BY slot_date ASC, starts_at ASC, id ASC
		FOR UPDATE
	`, workspaceID)
	if err != nil {
		return err
	}
	type positionUpdate struct {
		id       string
		slotDate string
		position int
	}
	var updates []positionUpdate

	currentDate := ""
	position := 0
	for rows.Next() {
		var (
			id       string
			slotDate string
		)
		if err := rows.Scan(&id, &slotDate); err != nil {
			rows.Close()
			return err
		}
		if slotDate != currentDate {
			currentDate = slotDate
			position = 0
		}
		updates = append(updates, positionUpdate{id: id, slotDate: slotDate, position: position})
		position++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `
			UPDATE daily_slots
			SET position = $4
			WHERE id = $1 AND workspace_id = $2 AND slot_date = $3
		`, update.id, workspaceID, update.slotDate, update.position); err != nil {
			return err
		}
	}
	return nil
}
