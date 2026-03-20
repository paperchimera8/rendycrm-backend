package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

var mondayFirstLabels = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

func (r *Repository) EnsureSlotSystem(ctx context.Context, workspaceID string) error {
	if _, err := r.ensureSlotSettings(ctx, workspaceID); err != nil {
		return err
	}
	if _, err := r.ensureDefaultSlotColors(ctx, workspaceID); err != nil {
		return err
	}
	if err := r.cleanupExpiredSlotHolds(ctx, workspaceID); err != nil {
		return err
	}
	if _, err := r.repairScheduleConsistency(ctx, workspaceID); err != nil {
		return err
	}
	return r.removeTemplateSlots(ctx, workspaceID)
}

func (r *Repository) SlotEditor(ctx context.Context, workspaceID string, day time.Time) (SlotEditorResponse, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return SlotEditorResponse{}, err
	}
	settings, err := r.SlotSettings(ctx, workspaceID)
	if err != nil {
		return SlotEditorResponse{}, err
	}
	colors, err := r.SlotColors(ctx, workspaceID)
	if err != nil {
		return SlotEditorResponse{}, err
	}
	daySlots, err := r.DaySlots(ctx, workspaceID, day)
	if err != nil {
		return SlotEditorResponse{}, err
	}
	return SlotEditorResponse{
		Settings:      settings,
		Colors:        colors,
		WeekTemplates: []SlotTemplate{},
		DaySlots:      daySlots,
	}, nil
}

func weekStartMonday(day time.Time) time.Time {
	offset := (int(day.Weekday()) + 6) % 7
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).AddDate(0, 0, -offset)
}

func (r *Repository) WeekSlots(ctx context.Context, workspaceID string, day time.Time) ([]WeekSlotDay, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return nil, err
	}
	loc, err := r.workspaceLocation(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	start := weekStartMonday(day.In(loc))
	end := start.AddDate(0, 0, 6)
	items, err := r.daySlotsBetweenDateKeys(ctx, workspaceID, start.Format("2006-01-02"), end.Format("2006-01-02"), false)
	if err != nil {
		return nil, err
	}
	slotsByDate := make(map[string][]DailySlot, 7)
	for _, item := range items {
		slotsByDate[item.SlotDate] = append(slotsByDate[item.SlotDate], item)
	}
	days := make([]WeekSlotDay, 0, 7)
	for index := 0; index < 7; index++ {
		current := start.AddDate(0, 0, index)
		key := current.Format("2006-01-02")
		days = append(days, WeekSlotDay{
			Date:    key,
			Weekday: index + 1,
			Label:   mondayFirstLabels[index],
			Slots:   append([]DailySlot{}, slotsByDate[key]...),
		})
	}
	return days, nil
}

func (r *Repository) SlotSettings(ctx context.Context, workspaceID string) (SlotSettings, error) {
	var settings SlotSettings
	err := r.db.QueryRowContext(ctx, `
		SELECT workspace_id, timezone, default_duration_minutes, generation_horizon_days
		FROM slot_settings
		WHERE workspace_id = $1
	`, workspaceID).Scan(
		&settings.WorkspaceID,
		&settings.Timezone,
		&settings.DefaultDurationMinutes,
		&settings.GenerationHorizonDays,
	)
	return settings, err
}

func (r *Repository) UpdateSlotSettings(ctx context.Context, workspaceID string, settings SlotSettings) (SlotSettings, error) {
	if settings.Timezone == "" {
		settings.Timezone = "UTC"
	}
	if settings.DefaultDurationMinutes <= 0 {
		settings.DefaultDurationMinutes = 60
	}
	if settings.GenerationHorizonDays <= 0 {
		settings.GenerationHorizonDays = 30
	}
	if _, err := time.LoadLocation(settings.Timezone); err != nil {
		return SlotSettings{}, fmt.Errorf("invalid timezone")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO slot_settings (workspace_id, timezone, default_duration_minutes, generation_horizon_days, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (workspace_id) DO UPDATE
		SET timezone = EXCLUDED.timezone,
			default_duration_minutes = EXCLUDED.default_duration_minutes,
			generation_horizon_days = EXCLUDED.generation_horizon_days,
			updated_at = NOW()
	`, workspaceID, settings.Timezone, settings.DefaultDurationMinutes, settings.GenerationHorizonDays)
	if err != nil {
		return SlotSettings{}, err
	}
	return r.SlotSettings(ctx, workspaceID)
}

func (r *Repository) removeTemplateSlots(ctx context.Context, workspaceID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM daily_slots
		WHERE workspace_id = $1
			AND source_template_id IS NOT NULL
			AND is_manual = FALSE
			AND status IN ('free', 'blocked')
			AND NOT EXISTS (
				SELECT 1
				FROM bookings b
				WHERE b.workspace_id = daily_slots.workspace_id
					AND b.daily_slot_id = daily_slots.id
					AND b.status <> 'cancelled'
			)
	`, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE daily_slots
		SET is_manual = TRUE,
			source_template_id = NULL
		WHERE workspace_id = $1
			AND source_template_id IS NOT NULL
	`, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_templates WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SlotColors(ctx context.Context, workspaceID string) ([]SlotColorPreset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, hex, position
		FROM slot_color_presets
		WHERE workspace_id = $1
		ORDER BY position ASC, created_at ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SlotColorPreset
	for rows.Next() {
		var item SlotColorPreset
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Name, &item.Hex, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateSlotColor(ctx context.Context, workspaceID, name, hex string) (SlotColorPreset, error) {
	var position int
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM slot_color_presets WHERE workspace_id = $1`, workspaceID).Scan(&position); err != nil {
		return SlotColorPreset{}, err
	}
	item := SlotColorPreset{
		ID:          newID("clr"),
		WorkspaceID: workspaceID,
		Name:        name,
		Hex:         hex,
		Position:    position,
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO slot_color_presets (id, workspace_id, name, hex, position)
		VALUES ($1, $2, $3, $4, $5)
	`, item.ID, workspaceID, item.Name, item.Hex, item.Position)
	return item, err
}

func (r *Repository) UpdateSlotColor(ctx context.Context, workspaceID, colorID, name, hex string) (SlotColorPreset, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE slot_color_presets
		SET name = $3, hex = $4
		WHERE id = $1 AND workspace_id = $2
	`, colorID, workspaceID, name, hex)
	if err != nil {
		return SlotColorPreset{}, err
	}
	colors, err := r.SlotColors(ctx, workspaceID)
	if err != nil {
		return SlotColorPreset{}, err
	}
	for _, color := range colors {
		if color.ID == colorID {
			return color, nil
		}
	}
	return SlotColorPreset{}, sql.ErrNoRows
}

func (r *Repository) ReorderSlotColors(ctx context.Context, workspaceID string, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE slot_color_presets SET position = $3 WHERE id = $1 AND workspace_id = $2`, id, workspaceID, index); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) DeleteSlotColor(ctx context.Context, workspaceID, colorID string) error {
	colors, err := r.SlotColors(ctx, workspaceID)
	if err != nil {
		return err
	}
	var fallback string
	for _, color := range colors {
		if color.ID != colorID {
			fallback = color.ID
			break
		}
	}
	if fallback == "" {
		return errors.New("at least one color preset is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE slot_templates SET color_preset_id = $3 WHERE workspace_id = $1 AND color_preset_id = $2`, workspaceID, colorID, fallback); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE daily_slots SET color_preset_id = $3 WHERE workspace_id = $1 AND color_preset_id = $2`, workspaceID, colorID, fallback); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_color_presets WHERE id = $1 AND workspace_id = $2`, colorID, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SlotTemplates(ctx context.Context, workspaceID string) ([]SlotTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.workspace_id, t.weekday, t.start_minute, t.duration_minutes, COALESCE(t.color_preset_id, ''), COALESCE(c.name, ''), COALESCE(c.hex, '#CBD5E1'), t.position, t.enabled
		FROM slot_templates t
		LEFT JOIN slot_color_presets c ON c.id = t.color_preset_id
		WHERE t.workspace_id = $1
		ORDER BY t.weekday ASC, t.position ASC, t.start_minute ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SlotTemplate
	for rows.Next() {
		var item SlotTemplate
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Weekday, &item.StartMinute, &item.DurationMinutes, &item.ColorPresetID, &item.ColorName, &item.ColorHex, &item.Position, &item.Enabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateSlotTemplate(ctx context.Context, workspaceID string, template SlotTemplate) (SlotTemplate, error) {
	if template.DurationMinutes <= 0 {
		settings, err := r.SlotSettings(ctx, workspaceID)
		if err != nil {
			return SlotTemplate{}, err
		}
		template.DurationMinutes = settings.DefaultDurationMinutes
	}
	if template.ColorPresetID == "" {
		colors, err := r.SlotColors(ctx, workspaceID)
		if err != nil {
			return SlotTemplate{}, err
		}
		if len(colors) > 0 {
			template.ColorPresetID = colors[0].ID
		}
	}
	var position int
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM slot_templates WHERE workspace_id = $1 AND weekday = $2`, workspaceID, template.Weekday).Scan(&position); err != nil {
		return SlotTemplate{}, err
	}
	template.ID = newID("stp")
	template.WorkspaceID = workspaceID
	template.Position = position
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO slot_templates (id, workspace_id, weekday, start_minute, duration_minutes, color_preset_id, position, enabled)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
	`, template.ID, workspaceID, template.Weekday, template.StartMinute, template.DurationMinutes, template.ColorPresetID, template.Position, template.Enabled); err != nil {
		return SlotTemplate{}, err
	}
	templates, err := r.SlotTemplates(ctx, workspaceID)
	if err != nil {
		return SlotTemplate{}, err
	}
	for _, item := range templates {
		if item.ID == template.ID {
			return item, nil
		}
	}
	return SlotTemplate{}, sql.ErrNoRows
}

func (r *Repository) UpdateSlotTemplate(ctx context.Context, workspaceID, templateID string, template SlotTemplate) (SlotTemplate, error) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE slot_templates
		SET weekday = $3,
			start_minute = $4,
			duration_minutes = $5,
			color_preset_id = NULLIF($6, ''),
			enabled = $7
		WHERE id = $1 AND workspace_id = $2
	`, templateID, workspaceID, template.Weekday, template.StartMinute, template.DurationMinutes, template.ColorPresetID, template.Enabled); err != nil {
		return SlotTemplate{}, err
	}
	templates, err := r.SlotTemplates(ctx, workspaceID)
	if err != nil {
		return SlotTemplate{}, err
	}
	for _, item := range templates {
		if item.ID == templateID {
			return item, nil
		}
	}
	return SlotTemplate{}, sql.ErrNoRows
}

func (r *Repository) ReorderSlotTemplates(ctx context.Context, workspaceID string, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE slot_templates SET position = $3 WHERE id = $1 AND workspace_id = $2`, id, workspaceID, index); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) DeleteSlotTemplate(ctx context.Context, workspaceID, templateID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	loc, err := r.workspaceLocationTx(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	currentDate := time.Now().In(loc).Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_templates WHERE id = $1 AND workspace_id = $2`, templateID, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM daily_slots
			WHERE workspace_id = $1
				AND source_template_id = $2
				AND is_manual = FALSE
				AND status = 'free'
				AND slot_date >= $3
		`, workspaceID, templateID, currentDate); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) DaySlots(ctx context.Context, workspaceID string, day time.Time) ([]DailySlot, error) {
	if err := r.cleanupExpiredSlotHolds(ctx, workspaceID); err != nil {
		return nil, err
	}
	dayKey, err := r.slotDateForTime(ctx, workspaceID, day)
	if err != nil {
		return nil, err
	}
	return r.daySlotsBetweenDateKeys(ctx, workspaceID, dayKey, dayKey, false)
}

func (r *Repository) FreeDaySlotsBetween(ctx context.Context, workspaceID string, from, to time.Time) ([]DailySlot, error) {
	return r.daySlotsBetween(ctx, workspaceID, from, to, true)
}

func (r *Repository) daySlotsBetween(ctx context.Context, workspaceID string, from, to time.Time, freeOnly bool) ([]DailySlot, error) {
	fromKey, err := r.slotDateForTime(ctx, workspaceID, from)
	if err != nil {
		return nil, err
	}
	toKey, err := r.slotDateForTime(ctx, workspaceID, to)
	if err != nil {
		return nil, err
	}
	return r.daySlotsBetweenDateKeys(ctx, workspaceID, fromKey, toKey, freeOnly)
}

func (r *Repository) daySlotsBetweenDateKeys(ctx context.Context, workspaceID, fromDate, toDate string, freeOnly bool) ([]DailySlot, error) {
	query := `
		SELECT ds.id, ds.workspace_id, ds.slot_date, ds.starts_at, ds.ends_at, ds.duration_minutes,
			COALESCE(ds.color_preset_id, ''), COALESCE(c.name, ''), COALESCE(c.hex, '#CBD5E1'),
			ds.position, ds.status, COALESCE(ds.source_template_id, ''), ds.is_manual, ds.note,
			COALESCE(b.id, ''), COALESCE(cust.name, '')
		FROM daily_slots ds
		LEFT JOIN slot_color_presets c ON c.id = ds.color_preset_id
		LEFT JOIN bookings b ON b.daily_slot_id = ds.id AND b.workspace_id = ds.workspace_id AND b.status <> 'cancelled'
		LEFT JOIN customers cust ON cust.id = b.customer_id
		WHERE ds.workspace_id = $1 AND ds.slot_date >= $2 AND ds.slot_date <= $3
	`
	if freeOnly {
		query += ` AND ds.status = 'free'`
	}
	query += ` ORDER BY ds.slot_date ASC, ds.position ASC, ds.starts_at ASC`
	rows, err := r.db.QueryContext(ctx, `
	`+query, workspaceID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DailySlot
	for rows.Next() {
		var (
			item     DailySlot
			slotDate time.Time
		)
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &slotDate, &item.StartsAt, &item.EndsAt, &item.DurationMinutes, &item.ColorPresetID, &item.ColorName, &item.ColorHex, &item.Position, &item.Status, &item.SourceTemplateID, &item.IsManual, &item.Note, &item.BookingID, &item.CustomerName); err != nil {
			return nil, err
		}
		item.SlotDate = slotDate.Format("2006-01-02")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AvailableDaySlots(ctx context.Context, workspaceID string, from, to time.Time) ([]DailySlot, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return nil, err
	}
	settings, err := r.SlotSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rules, exceptions, bookings, holds, err := r.Availability(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		loc = time.UTC
	}
	startDay := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(to.In(loc).Year(), to.In(loc).Month(), to.In(loc).Day(), 0, 0, 0, 0, loc)

	positions := map[string]int{}
	seen := map[string]struct{}{}
	items := make([]DailySlot, 0, 64)
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		weekday := int(day.Weekday())
		for _, rule := range rules {
			ruleWeekday := rule.DayOfWeek
			if ruleWeekday == 7 {
				ruleWeekday = 0
			}
			if !rule.Enabled || ruleWeekday != weekday || rule.EndMinute <= rule.StartMinute {
				continue
			}
			windowStart := day.Add(time.Duration(rule.StartMinute) * time.Minute)
			windowEnd := day.Add(time.Duration(rule.EndMinute) * time.Minute)
			for slotStart := windowStart; !slotStart.Add(time.Hour).After(windowEnd); slotStart = slotStart.Add(time.Hour) {
				slotEnd := slotStart.Add(time.Hour)
				if !slotAvailable(slotStart, slotEnd, exceptions, bookings, holds) {
					continue
				}
				id := "avl_" + slotStart.UTC().Format("200601021504")
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				slotDate := slotStart.In(loc).Format("2006-01-02")
				position := positions[slotDate]
				positions[slotDate] = position + 1
				items = append(items, DailySlot{
					ID:               id,
					WorkspaceID:      workspaceID,
					SlotDate:         slotDate,
					StartsAt:         slotStart.UTC(),
					EndsAt:           slotEnd.UTC(),
					DurationMinutes:  60,
					ColorPresetID:    "",
					ColorName:        "",
					ColorHex:         "#CBD5E1",
					Position:         position,
					Status:           DailySlotFree,
					SourceTemplateID: "computed",
					IsManual:         false,
					Note:             "",
					BookingID:        "",
					CustomerName:     "",
				})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartsAt.Equal(items[j].StartsAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].StartsAt.Before(items[j].StartsAt)
	})
	return items, nil
}

func (r *Repository) CreateDaySlot(ctx context.Context, workspaceID string, daySlot DailySlot) (DailySlot, error) {
	if daySlot.Status == "" {
		daySlot.Status = DailySlotFree
	}
	daySlot.ID = newID("dsl")
	daySlot.WorkspaceID = workspaceID
	daySlot.IsManual = true
	slotDate, err := r.slotDateForTime(ctx, workspaceID, daySlot.StartsAt)
	if err != nil {
		return DailySlot{}, err
	}
	daySlot.SlotDate = slotDate
	if err := r.validateSlotWindow(ctx, workspaceID, "", daySlot.StartsAt, daySlot.EndsAt); err != nil {
		return DailySlot{}, err
	}
	var position int
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM daily_slots WHERE workspace_id = $1 AND slot_date = $2`, workspaceID, daySlot.SlotDate).Scan(&position); err != nil {
		return DailySlot{}, err
	}
	daySlot.Position = position
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, color_preset_id, position, status, is_manual, note)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, TRUE, $10)
	`, daySlot.ID, workspaceID, daySlot.SlotDate, daySlot.StartsAt, daySlot.EndsAt, daySlot.DurationMinutes, daySlot.ColorPresetID, daySlot.Position, daySlot.Status, daySlot.Note)
	if err != nil {
		return DailySlot{}, err
	}
	items, err := r.DaySlots(ctx, workspaceID, daySlot.StartsAt)
	if err != nil {
		return DailySlot{}, err
	}
	for _, item := range items {
		if item.ID == daySlot.ID {
			return item, nil
		}
	}
	return DailySlot{}, sql.ErrNoRows
}

func (r *Repository) UpdateDaySlot(ctx context.Context, workspaceID, slotID string, patch DailySlot) (DailySlot, error) {
	current, err := r.DailySlot(ctx, workspaceID, slotID)
	if err != nil {
		return DailySlot{}, err
	}
	if current.Status == DailySlotHeld || current.Status == DailySlotBooked {
		return DailySlot{}, errors.New("booked or held slot cannot be edited")
	}
	slotDate, err := r.slotDateForTime(ctx, workspaceID, patch.StartsAt)
	if err != nil {
		return DailySlot{}, err
	}
	patch.SlotDate = slotDate
	if err := r.validateSlotWindow(ctx, workspaceID, slotID, patch.StartsAt, patch.EndsAt); err != nil {
		return DailySlot{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE daily_slots
		SET slot_date = $3,
			starts_at = $4,
			ends_at = $5,
			duration_minutes = $6,
			color_preset_id = NULLIF($7, ''),
			note = $8
		WHERE id = $1 AND workspace_id = $2
	`, slotID, workspaceID, patch.SlotDate, patch.StartsAt, patch.EndsAt, patch.DurationMinutes, patch.ColorPresetID, patch.Note)
	if err != nil {
		return DailySlot{}, err
	}
	return r.DailySlot(ctx, workspaceID, slotID)
}

func (r *Repository) ReorderDaySlots(ctx context.Context, workspaceID, slotDate string, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE daily_slots SET position = $4 WHERE id = $1 AND workspace_id = $2 AND slot_date = $3`, id, workspaceID, slotDate, index); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func slotTimesForDate(targetDate string, startsAt, endsAt time.Time, loc *time.Location) (time.Time, time.Time, error) {
	base, err := time.ParseInLocation("2006-01-02", targetDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	startLocal := startsAt.In(loc)
	newStartsAt := time.Date(base.Year(), base.Month(), base.Day(), startLocal.Hour(), startLocal.Minute(), startLocal.Second(), startLocal.Nanosecond(), loc)
	return newStartsAt.UTC(), newStartsAt.Add(endsAt.Sub(startsAt)).UTC(), nil
}

func (r *Repository) slotIDsForDateTx(ctx context.Context, tx *sql.Tx, workspaceID, slotDate string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM daily_slots
		WHERE workspace_id = $1 AND slot_date = $2
		ORDER BY position ASC, starts_at ASC
	`, workspaceID, slotDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func reorderIDList(ids []string, movedID string, targetIndex int) []string {
	next := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != movedID {
			next = append(next, id)
		}
	}
	if targetIndex < 0 {
		targetIndex = 0
	}
	if targetIndex > len(next) {
		targetIndex = len(next)
	}
	next = append(next[:targetIndex], append([]string{movedID}, next[targetIndex:]...)...)
	return next
}

func writeDayPositionsTx(ctx context.Context, tx *sql.Tx, workspaceID, slotDate string, ids []string) error {
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			UPDATE daily_slots
			SET position = $4
			WHERE id = $1 AND workspace_id = $2 AND slot_date = $3
		`, id, workspaceID, slotDate, index); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) MoveDaySlot(ctx context.Context, workspaceID, slotID, targetSlotDate string, targetIndex int) (DailySlot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return DailySlot{}, err
	}
	defer tx.Rollback()

	var current DailySlot
	var slotDate time.Time
	if err := tx.QueryRowContext(ctx, `
		SELECT ds.id, ds.workspace_id, ds.slot_date, ds.starts_at, ds.ends_at, ds.duration_minutes,
			COALESCE(ds.color_preset_id, ''), COALESCE(c.name, ''), COALESCE(c.hex, '#CBD5E1'),
			ds.position, ds.status, COALESCE(ds.source_template_id, ''), ds.is_manual, ds.note,
			COALESCE(b.id, ''), COALESCE(cust.name, '')
		FROM daily_slots ds
		LEFT JOIN slot_color_presets c ON c.id = ds.color_preset_id
		LEFT JOIN bookings b ON b.daily_slot_id = ds.id AND b.workspace_id = ds.workspace_id AND b.status <> 'cancelled'
		LEFT JOIN customers cust ON cust.id = b.customer_id
		WHERE ds.id = $1 AND ds.workspace_id = $2
		FOR UPDATE OF ds
	`, slotID, workspaceID).Scan(
		&current.ID,
		&current.WorkspaceID,
		&slotDate,
		&current.StartsAt,
		&current.EndsAt,
		&current.DurationMinutes,
		&current.ColorPresetID,
		&current.ColorName,
		&current.ColorHex,
		&current.Position,
		&current.Status,
		&current.SourceTemplateID,
		&current.IsManual,
		&current.Note,
		&current.BookingID,
		&current.CustomerName,
	); err != nil {
		return DailySlot{}, err
	}
	current.SlotDate = slotDate.Format("2006-01-02")
	if current.Status == DailySlotHeld || current.Status == DailySlotBooked {
		return DailySlot{}, errors.New("booked or held slot cannot be moved")
	}

	loc, err := r.workspaceLocationTx(ctx, tx, workspaceID)
	if err != nil {
		return DailySlot{}, err
	}
	targetStartsAt, targetEndsAt, err := slotTimesForDate(targetSlotDate, current.StartsAt, current.EndsAt, loc)
	if err != nil {
		return DailySlot{}, fmt.Errorf("invalid target date")
	}
	if err := r.validateSlotWindow(ctx, workspaceID, slotID, targetStartsAt, targetEndsAt); err != nil {
		return DailySlot{}, err
	}

	sourceIDs, err := r.slotIDsForDateTx(ctx, tx, workspaceID, current.SlotDate)
	if err != nil {
		return DailySlot{}, err
	}
	targetIDs := sourceIDs
	if current.SlotDate != targetSlotDate {
		targetIDs, err = r.slotIDsForDateTx(ctx, tx, workspaceID, targetSlotDate)
		if err != nil {
			return DailySlot{}, err
		}
	}

	if current.SlotDate != targetSlotDate {
		sourceIDs = reorderIDList(sourceIDs, slotID, len(sourceIDs))
		sourceIDs = sourceIDs[:len(sourceIDs)-1]
	}
	targetIDs = reorderIDList(targetIDs, slotID, targetIndex)

	if _, err := tx.ExecContext(ctx, `
		UPDATE daily_slots
		SET slot_date = $3, starts_at = $4, ends_at = $5
		WHERE id = $1 AND workspace_id = $2
	`, slotID, workspaceID, targetSlotDate, targetStartsAt, targetEndsAt); err != nil {
		return DailySlot{}, err
	}

	if current.SlotDate == targetSlotDate {
		if err := writeDayPositionsTx(ctx, tx, workspaceID, targetSlotDate, targetIDs); err != nil {
			return DailySlot{}, err
		}
	} else {
		if err := writeDayPositionsTx(ctx, tx, workspaceID, current.SlotDate, sourceIDs); err != nil {
			return DailySlot{}, err
		}
		if err := writeDayPositionsTx(ctx, tx, workspaceID, targetSlotDate, targetIDs); err != nil {
			return DailySlot{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return DailySlot{}, err
	}
	return r.DailySlot(ctx, workspaceID, slotID)
}

func (r *Repository) DeleteDaySlot(ctx context.Context, workspaceID, slotID string) error {
	if _, err := r.DailySlot(ctx, workspaceID, slotID); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var completedCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM bookings
		WHERE workspace_id = $1
			AND daily_slot_id = $2
			AND status = 'completed'
	`, workspaceID, slotID).Scan(&completedCount); err != nil {
		return err
	}
	if completedCount > 0 {
		return errors.New("completed booking slot cannot be deleted")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE bookings
		SET status = 'cancelled',
			slot_hold_id = NULL
		WHERE workspace_id = $1
			AND daily_slot_id = $2
			AND status IN ('pending', 'confirmed')
	`, workspaceID, slotID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, slotID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM daily_slots WHERE id = $1 AND workspace_id = $2`, slotID, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SetDaySlotStatus(ctx context.Context, workspaceID, slotID string, status DailySlotStatus) (DailySlot, error) {
	slot, err := r.DailySlot(ctx, workspaceID, slotID)
	if err != nil {
		return DailySlot{}, err
	}
	if slot.Status == DailySlotHeld || slot.Status == DailySlotBooked {
		return DailySlot{}, errors.New("booked or held slot cannot be edited")
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE daily_slots SET status = $3 WHERE id = $1 AND workspace_id = $2`, slotID, workspaceID, status); err != nil {
		return DailySlot{}, err
	}
	return r.DailySlot(ctx, workspaceID, slotID)
}

func (r *Repository) DailySlot(ctx context.Context, workspaceID, slotID string) (DailySlot, error) {
	var (
		item     DailySlot
		slotDate time.Time
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT ds.id, ds.workspace_id, ds.slot_date, ds.starts_at, ds.ends_at, ds.duration_minutes,
			COALESCE(ds.color_preset_id, ''), COALESCE(c.name, ''), COALESCE(c.hex, '#CBD5E1'),
			ds.position, ds.status, COALESCE(ds.source_template_id, ''), ds.is_manual, ds.note,
			COALESCE(b.id, ''), COALESCE(cust.name, '')
		FROM daily_slots ds
		LEFT JOIN slot_color_presets c ON c.id = ds.color_preset_id
		LEFT JOIN bookings b ON b.daily_slot_id = ds.id AND b.workspace_id = ds.workspace_id AND b.status <> 'cancelled'
		LEFT JOIN customers cust ON cust.id = b.customer_id
		WHERE ds.id = $1 AND ds.workspace_id = $2
	`, slotID, workspaceID).Scan(&item.ID, &item.WorkspaceID, &slotDate, &item.StartsAt, &item.EndsAt, &item.DurationMinutes, &item.ColorPresetID, &item.ColorName, &item.ColorHex, &item.Position, &item.Status, &item.SourceTemplateID, &item.IsManual, &item.Note, &item.BookingID, &item.CustomerName)
	if err != nil {
		return DailySlot{}, err
	}
	item.SlotDate = slotDate.Format("2006-01-02")
	return item, nil
}

func (r *Repository) CreateSlotHold(ctx context.Context, workspaceID, dailySlotID, customerID string) (SlotHold, error) {
	if err := r.cleanupExpiredSlotHolds(ctx, workspaceID); err != nil {
		return SlotHold{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SlotHold{}, err
	}
	defer tx.Rollback()

	var hold SlotHold
	var status DailySlotStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT starts_at, ends_at, status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, dailySlotID, workspaceID).Scan(&hold.StartsAt, &hold.EndsAt, &status); err != nil {
		return SlotHold{}, err
	}
	if status != DailySlotFree {
		return SlotHold{}, errors.New("slot unavailable")
	}
	hold.ID = newID("hold")
	hold.WorkspaceID = workspaceID
	hold.CustomerID = customerID
	hold.DailySlotID = dailySlotID
	hold.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO slot_holds (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, hold.ID, workspaceID, customerID, dailySlotID, hold.StartsAt, hold.EndsAt, hold.ExpiresAt); err != nil {
		return SlotHold{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE daily_slots SET status = 'held' WHERE id = $1 AND workspace_id = $2`, dailySlotID, workspaceID); err != nil {
		return SlotHold{}, err
	}
	if err := tx.Commit(); err != nil {
		return SlotHold{}, err
	}
	return hold, nil
}

func (r *Repository) ReleaseSlotHold(ctx context.Context, workspaceID, holdID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var dailySlotID string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(daily_slot_id, '')
		FROM slot_holds
		WHERE id = $1 AND workspace_id = $2
	`, holdID, workspaceID).Scan(&dailySlotID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, holdID, workspaceID); err != nil {
		return err
	}
	if dailySlotID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE daily_slots
			SET status = 'free'
			WHERE id = $1 AND workspace_id = $2 AND NOT EXISTS (
				SELECT 1 FROM bookings WHERE daily_slot_id = $1 AND workspace_id = $2 AND status <> 'cancelled'
			)
		`, dailySlotID, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ensureSlotSettings(ctx context.Context, workspaceID string) (SlotSettings, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO slot_settings (workspace_id, timezone, default_duration_minutes, generation_horizon_days)
		SELECT id, timezone, 60, 30
		FROM workspaces
		WHERE id = $1
		ON CONFLICT (workspace_id) DO NOTHING
	`, workspaceID)
	if err != nil {
		return SlotSettings{}, err
	}
	return r.SlotSettings(ctx, workspaceID)
}

func (r *Repository) ensureDefaultSlotColors(ctx context.Context, workspaceID string) ([]SlotColorPreset, error) {
	colors, err := r.SlotColors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(colors) > 0 {
		return colors, nil
	}
	defaults := []struct {
		name string
		hex  string
	}{
		{"Базовый", "#C7D2FE"},
		{"Прайм", "#FBCFE8"},
		{"Экспресс", "#BBF7D0"},
	}
	for index, item := range defaults {
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO slot_color_presets (id, workspace_id, name, hex, position)
			VALUES ($1, $2, $3, $4, $5)
		`, fmt.Sprintf("clr_%d", index+1), workspaceID, item.name, item.hex, index); err != nil {
			return nil, err
		}
	}
	return r.SlotColors(ctx, workspaceID)
}

func (r *Repository) migrateLegacyAvailability(ctx context.Context, workspaceID string, settings SlotSettings, colors []SlotColorPreset) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM slot_templates WHERE workspace_id = $1`, workspaceID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	colorID := ""
	if len(colors) > 0 {
		colorID = colors[0].ID
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT day_of_week, start_minute, end_minute, enabled
		FROM availability_rules
		WHERE workspace_id = $1
		ORDER BY day_of_week, start_minute
	`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacyRule struct {
		weekday int
		start   int
		end     int
		enabled bool
	}
	var legacy []legacyRule
	for rows.Next() {
		var item legacyRule
		if err := rows.Scan(&item.weekday, &item.start, &item.end, &item.enabled); err != nil {
			return err
		}
		legacy = append(legacy, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	positionByDay := map[int]int{}
	if len(legacy) == 0 {
		return nil
	}
	step := 120
	for _, rule := range legacy {
		if !rule.enabled {
			continue
		}
		for minute := rule.start; minute+settings.DefaultDurationMinutes <= rule.end; minute += step {
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO slot_templates (id, workspace_id, weekday, start_minute, duration_minutes, color_preset_id, position, enabled)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, TRUE)
			`, newID("stp"), workspaceID, rule.weekday, minute, settings.DefaultDurationMinutes, colorID, positionByDay[rule.weekday]); err != nil {
				return err
			}
			positionByDay[rule.weekday]++
		}
	}
	return nil
}

func (r *Repository) syncGeneratedDailySlots(ctx context.Context, workspaceID string) error {
	settings, err := r.SlotSettings(ctx, workspaceID)
	if err != nil {
		return err
	}
	templates, err := r.SlotTemplates(ctx, workspaceID)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		loc = time.UTC
	}
	nowLocal := time.Now().In(loc)
	startDay := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	endDay := startDay.AddDate(0, 0, settings.GenerationHorizonDays)

	for _, template := range templates {
		if !template.Enabled {
			continue
		}
		for day := startDay; day.Before(endDay); day = day.AddDate(0, 0, 1) {
			if int(day.Weekday()) != template.Weekday {
				continue
			}
			startLocal := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).Add(time.Duration(template.StartMinute) * time.Minute)
			endLocal := startLocal.Add(time.Duration(template.DurationMinutes) * time.Minute)
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, color_preset_id, position, status, source_template_id, is_manual, note)
				VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, 'free', $9, FALSE, '')
				ON CONFLICT (source_template_id, slot_date) WHERE source_template_id IS NOT NULL DO UPDATE
				SET starts_at = EXCLUDED.starts_at,
					ends_at = EXCLUDED.ends_at,
					duration_minutes = EXCLUDED.duration_minutes,
					color_preset_id = EXCLUDED.color_preset_id,
					position = EXCLUDED.position
				WHERE daily_slots.is_manual = FALSE AND daily_slots.status = 'free'
			`, newID("dsl"), workspaceID, day.Format("2006-01-02"), startLocal.UTC(), endLocal.UTC(), template.DurationMinutes, template.ColorPresetID, template.Position, template.ID); err != nil {
				return err
			}
		}
	}

	activeTemplateIDs := make([]string, 0, len(templates))
	for _, template := range templates {
		if template.Enabled {
			activeTemplateIDs = append(activeTemplateIDs, template.ID)
		}
	}
	if len(activeTemplateIDs) == 0 {
		if _, err := r.db.ExecContext(ctx, `
			DELETE FROM daily_slots
			WHERE workspace_id = $1
				AND is_manual = FALSE
				AND status = 'free'
				AND slot_date >= $2
				AND slot_date < $3
		`, workspaceID, startDay.Format("2006-01-02"), endDay.Format("2006-01-02")); err != nil {
			return err
		}
	} else {
		query := `
			DELETE FROM daily_slots
			WHERE workspace_id = $1
				AND is_manual = FALSE
				AND status = 'free'
				AND slot_date >= $2
				AND slot_date < $3
				AND source_template_id IS NOT NULL
				AND NOT (source_template_id = ANY($4))
		`
		if _, err := r.db.ExecContext(ctx, query, workspaceID, startDay.Format("2006-01-02"), endDay.Format("2006-01-02"), activeTemplateIDs); err != nil {
			return err
		}
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE daily_slots ds
		SET status = 'blocked',
			note = CASE WHEN ds.note = '' THEN ae.reason ELSE ds.note END
		FROM availability_exceptions ae
		WHERE ds.workspace_id = $1
			AND ds.workspace_id = ae.workspace_id
			AND ds.slot_date >= $2
			AND ds.slot_date < $3
			AND ds.status = 'free'
			AND ds.starts_at < ae.ends_at
			AND ds.ends_at > ae.starts_at
	`, workspaceID, startDay.Format("2006-01-02"), endDay.Format("2006-01-02")); err != nil {
		return err
	}

	return nil
}

func (r *Repository) backfillDailySlotsForBookings(ctx context.Context, workspaceID string) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, customer_id, starts_at, ends_at, status, notes
		FROM bookings
		WHERE workspace_id = $1
			AND daily_slot_id IS NULL
			AND status <> 'cancelled'
	`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct {
		id         string
		customerID string
		startsAt   time.Time
		endsAt     time.Time
		status     BookingStatus
		notes      string
	}
	var items []item
	for rows.Next() {
		var row item
		if err := rows.Scan(&row.id, &row.customerID, &row.startsAt, &row.endsAt, &row.status, &row.notes); err != nil {
			return err
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, booking := range items {
		status := DailySlotHeld
		if booking.status == BookingConfirmed || booking.status == BookingCompleted {
			status = DailySlotBooked
		}
		if booking.status == BookingCancelled {
			status = DailySlotFree
		}
		var slotID string
		err := r.db.QueryRowContext(ctx, `
			SELECT id
			FROM daily_slots
			WHERE workspace_id = $1 AND starts_at = $2 AND ends_at = $3
			LIMIT 1
		`, workspaceID, booking.startsAt, booking.endsAt).Scan(&slotID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if slotID == "" {
			settings, settingsErr := r.SlotSettings(ctx, workspaceID)
			if settingsErr != nil {
				return settingsErr
			}
			colors, colorsErr := r.SlotColors(ctx, workspaceID)
			if colorsErr != nil {
				return colorsErr
			}
			colorID := ""
			if len(colors) > 0 {
				colorID = colors[0].ID
			}
			slotDate, slotDateErr := r.slotDateForTime(ctx, workspaceID, booking.startsAt)
			if slotDateErr != nil {
				return slotDateErr
			}
			slotID = newID("dsl")
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, color_preset_id, position, status, is_manual, note)
				VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), 0, $8, TRUE, $9)
			`, slotID, workspaceID, slotDate, booking.startsAt, booking.endsAt, int(booking.endsAt.Sub(booking.startsAt).Minutes()), colorID, status, booking.notes); err != nil {
				return err
			}
			_ = settings
		} else {
			if _, err := r.db.ExecContext(ctx, `UPDATE daily_slots SET status = $3 WHERE id = $1 AND workspace_id = $2`, slotID, workspaceID, status); err != nil {
				return err
			}
		}
		if _, err := r.db.ExecContext(ctx, `UPDATE bookings SET daily_slot_id = $3 WHERE id = $1 AND workspace_id = $2`, booking.id, workspaceID, slotID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) cleanupExpiredSlotHolds(ctx context.Context, workspaceID string) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(daily_slot_id, '')
		FROM slot_holds
		WHERE workspace_id = $1 AND expires_at <= NOW()
	`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type expiredHold struct {
		id          string
		dailySlotID string
	}
	var expired []expiredHold
	for rows.Next() {
		var item expiredHold
		if err := rows.Scan(&item.id, &item.dailySlotID); err != nil {
			return err
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, hold := range expired {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, hold.id, workspaceID); err != nil {
			return err
		}
		if hold.dailySlotID != "" {
			if _, err := r.db.ExecContext(ctx, `
				UPDATE daily_slots
				SET status = 'free'
				WHERE id = $1
					AND workspace_id = $2
					AND status = 'held'
					AND NOT EXISTS (
						SELECT 1 FROM bookings WHERE daily_slot_id = $1 AND workspace_id = $2 AND status <> 'cancelled'
					)
			`, hold.dailySlotID, workspaceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) validateSlotWindow(ctx context.Context, workspaceID, slotID string, startsAt, endsAt time.Time) error {
	if !endsAt.After(startsAt) {
		return errors.New("end must be after start")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM daily_slots
		WHERE workspace_id = $1
			AND id <> $2
			AND starts_at < $4
			AND ends_at > $3
	`, workspaceID, slotID, startsAt, endsAt).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("slot overlaps existing slot")
	}
	return nil
}

func dateFromString(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

func moveIDs(ids []string, draggedID, targetID string) []string {
	if draggedID == targetID {
		return ids
	}
	result := slices.Clone(ids)
	from := slices.Index(result, draggedID)
	to := slices.Index(result, targetID)
	if from == -1 || to == -1 {
		return ids
	}
	result = append(result[:from], result[from+1:]...)
	if from < to {
		to--
	}
	result = append(result[:to], append([]string{draggedID}, result[to:]...)...)
	return result
}
