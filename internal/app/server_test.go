package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeAvailableSlotsSkipsConflicts(t *testing.T) {
	base := time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC)
	rules := []AvailabilityRule{{ID: "avr_1", DayOfWeek: 1, StartMinute: 9 * 60, EndMinute: 15 * 60, Enabled: true}}
	bookings := []Booking{{StartsAt: time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC), Status: BookingConfirmed}}
	slots := computeAvailableSlots(base, rules, nil, bookings, nil)
	if len(slots) == 0 {
		t.Fatal("expected available slots")
	}
	if slots[0].Start.Hour() == 9 {
		t.Fatalf("expected first slot to skip booked window, got %s", slots[0].Start)
	}
}

func TestSlotAvailableRejectsException(t *testing.T) {
	start := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	exceptions := []AvailabilityException{{StartsAt: start.Add(-15 * time.Minute), EndsAt: end.Add(15 * time.Minute)}}
	if slotAvailable(start, end, exceptions, nil, nil) {
		t.Fatal("expected slot to be unavailable")
	}
}

func TestComputeAvailableSlotsRoundsUpAfterPartialBooking(t *testing.T) {
	base := time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC)
	rules := []AvailabilityRule{{ID: "avr_1", DayOfWeek: 1, StartMinute: 10 * 60, EndMinute: 15 * 60, Enabled: true}}
	bookings := []Booking{{StartsAt: time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 3, 9, 12, 30, 0, 0, time.UTC), Status: BookingConfirmed}}
	slots := computeAvailableSlots(base, rules, nil, bookings, nil)
	for _, slot := range slots {
		if slot.Start.Hour() == 12 {
			t.Fatalf("expected 12:00-13:00 slot to be blocked, got %s-%s", slot.Start, slot.End)
		}
	}
}

func TestHandleAppServesIndexFallback(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	server := &Server{cfg: Config{StaticDir: tmpDir}}

	request := httptest.NewRequest(http.MethodGet, "/settings", nil)
	recorder := httptest.NewRecorder()
	server.handleApp(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body != "<html>ok</html>" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestHandleAppServesStaticFilesAndRejectsMissingAssets(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetsDir := filepath.Join(tmpDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	server := &Server{cfg: Config{StaticDir: tmpDir}}

	t.Run("serves existing asset", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
		recorder := httptest.NewRecorder()
		server.handleApp(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "console.log('ok')" {
			t.Fatalf("unexpected asset body: %q", body)
		}
		if cache := recorder.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
			t.Fatalf("unexpected asset cache control: %q", cache)
		}
	})

	t.Run("missing asset returns 404", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
		recorder := httptest.NewRecorder()
		server.handleApp(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", recorder.Code)
		}
	})

	t.Run("spa route falls back to index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/dialogs/123", nil)
		recorder := httptest.NewRecorder()
		server.handleApp(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "<html>app</html>" {
			t.Fatalf("unexpected fallback body: %q", body)
		}
		if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
			t.Fatalf("unexpected spa cache control: %q", cache)
		}
	})
}
