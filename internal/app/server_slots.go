package app

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleSlotsEditor(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	day := time.Now().UTC()
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date")
			return
		}
		day = parsed
	}
	editor, err := s.runtime.repository.SlotEditor(r.Context(), auth.Workspace.ID, day)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "slot editor query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, editor)
}

func (s *Server) handleWeekSlots(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	day := time.Now().UTC()
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date")
			return
		}
		day = parsed
	}
	items, err := s.runtime.repository.WeekSlots(r.Context(), auth.Workspace.ID, day)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "week slots query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"days": items})
}

func (s *Server) handleSlotSettings(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload SlotSettings
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	settings, err := s.runtime.repository.UpdateSlotSettings(r.Context(), auth.Workspace.ID, payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleSlotColors(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.runtime.repository.SlotColors(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "slot colors query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var payload struct {
			Name string `json:"name"`
			Hex  string `json:"hex"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		item, err := s.runtime.repository.CreateSlotColor(r.Context(), auth.Workspace.ID, payload.Name, payload.Hex)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusCreated, item)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSlotColorByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/slots/colors/"), "/")
	if id == "" {
		s.writeError(w, http.StatusNotFound, "color not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var payload struct {
			Name string `json:"name"`
			Hex  string `json:"hex"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		item, err := s.runtime.repository.UpdateSlotColor(r.Context(), auth.Workspace.ID, id, payload.Name, payload.Hex)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.runtime.repository.DeleteSlotColor(r.Context(), auth.Workspace.ID, id); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSlotColorsReorder(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if err := s.runtime.repository.ReorderSlotColors(r.Context(), auth.Workspace.ID, payload.IDs); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSlotTemplates(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.runtime.repository.SlotTemplates(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "slot templates query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var payload SlotTemplate
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		item, err := s.runtime.repository.CreateSlotTemplate(r.Context(), auth.Workspace.ID, payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusCreated, item)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSlotTemplateByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/slots/templates/"), "/")
	if id == "" {
		s.writeError(w, http.StatusNotFound, "template not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var payload SlotTemplate
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		item, err := s.runtime.repository.UpdateSlotTemplate(r.Context(), auth.Workspace.ID, id, payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.runtime.repository.DeleteSlotTemplate(r.Context(), auth.Workspace.ID, id); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSlotTemplatesReorder(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if err := s.runtime.repository.ReorderSlotTemplates(r.Context(), auth.Workspace.ID, payload.IDs); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDaySlots(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		rawDate := strings.TrimSpace(r.URL.Query().Get("date"))
		if rawDate == "" {
			s.writeError(w, http.StatusBadRequest, "missing date")
			return
		}
		day, err := time.Parse("2006-01-02", rawDate)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date")
			return
		}
		items, err := s.runtime.repository.DaySlots(r.Context(), auth.Workspace.ID, day)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "day slots query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var payload struct {
			SlotDate        string `json:"slotDate"`
			StartsAt        string `json:"startsAt"`
			DurationMinutes int    `json:"durationMinutes"`
			ColorPresetID   string `json:"colorPresetId"`
			Status          string `json:"status"`
			Note            string `json:"note"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		startsAt, err := time.Parse(time.RFC3339, payload.StartsAt)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid startsAt")
			return
		}
		duration := payload.DurationMinutes
		if duration <= 0 {
			settings, settingsErr := s.runtime.repository.SlotSettings(r.Context(), auth.Workspace.ID)
			if settingsErr != nil {
				s.writeError(w, http.StatusInternalServerError, "slot settings query failed")
				return
			}
			duration = settings.DefaultDurationMinutes
		}
		status := DailySlotStatus(payload.Status)
		if status == "" {
			status = DailySlotFree
		}
		item, err := s.runtime.repository.CreateDaySlot(r.Context(), auth.Workspace.ID, DailySlot{
			SlotDate:        payload.SlotDate,
			StartsAt:        startsAt,
			EndsAt:          startsAt.Add(time.Duration(duration) * time.Minute),
			DurationMinutes: duration,
			ColorPresetID:   payload.ColorPresetID,
			Status:          status,
			Note:            payload.Note,
		})
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusCreated, item)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleDaySlotByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/slots/day-slots/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.writeError(w, http.StatusNotFound, "slot not found")
		return
	}
	id := parts[0]
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "block":
			item, err := s.runtime.repository.SetDaySlotStatus(r.Context(), auth.Workspace.ID, id, DailySlotBlocked)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.writeJSON(w, http.StatusOK, item)
			return
		case "unblock":
			item, err := s.runtime.repository.SetDaySlotStatus(r.Context(), auth.Workspace.ID, id, DailySlotFree)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.writeJSON(w, http.StatusOK, item)
			return
		}
	}
	switch r.Method {
	case http.MethodPatch:
		var payload struct {
			SlotDate        string `json:"slotDate"`
			StartsAt        string `json:"startsAt"`
			DurationMinutes int    `json:"durationMinutes"`
			ColorPresetID   string `json:"colorPresetId"`
			Note            string `json:"note"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		startsAt, err := time.Parse(time.RFC3339, payload.StartsAt)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid startsAt")
			return
		}
		item, err := s.runtime.repository.UpdateDaySlot(r.Context(), auth.Workspace.ID, id, DailySlot{
			SlotDate:        payload.SlotDate,
			StartsAt:        startsAt,
			EndsAt:          startsAt.Add(time.Duration(payload.DurationMinutes) * time.Minute),
			DurationMinutes: payload.DurationMinutes,
			ColorPresetID:   payload.ColorPresetID,
			Note:            payload.Note,
		})
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.runtime.repository.DeleteDaySlot(r.Context(), auth.Workspace.ID, id); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleDaySlotsReorder(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		SlotDate string   `json:"slotDate"`
		IDs      []string `json:"ids"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if err := s.runtime.repository.ReorderDaySlots(r.Context(), auth.Workspace.ID, payload.SlotDate, payload.IDs); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDaySlotMove(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		ID             string `json:"id"`
		TargetSlotDate string `json:"targetSlotDate"`
		TargetIndex    int    `json:"targetIndex"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if payload.ID == "" || payload.TargetSlotDate == "" {
		s.writeError(w, http.StatusBadRequest, "missing move target")
		return
	}
	item, err := s.runtime.repository.MoveDaySlot(r.Context(), auth.Workspace.ID, payload.ID, payload.TargetSlotDate, payload.TargetIndex)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAvailableSlots(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	dateFrom := time.Now().UTC()
	dateTo := dateFrom.AddDate(0, 0, 14)
	if raw := strings.TrimSpace(r.URL.Query().Get("date_from")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date_from")
			return
		}
		dateFrom = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("date_to")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date_to")
			return
		}
		dateTo = parsed
	}
	items, err := s.runtime.repository.AvailableDaySlots(r.Context(), auth.Workspace.ID, dateFrom, dateTo)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "available slots query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSlotByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/slots/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "hold" || r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		CustomerID string `json:"customerId"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	hold, err := s.runtime.repository.CreateSlotHold(r.Context(), auth.Workspace.ID, parts[0], payload.CustomerID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventBookingUpdated, Data: hold})
	s.writeJSON(w, http.StatusCreated, map[string]any{"hold": hold})
}

func (s *Server) handleSlotHoldByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodDelete {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/slot-holds/"), "/")
	if id == "" {
		s.writeError(w, http.StatusNotFound, "hold not found")
		return
	}
	if err := s.runtime.repository.ReleaseSlotHold(r.Context(), auth.Workspace.ID, id); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
