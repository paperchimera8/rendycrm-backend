package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestServerServesAPIUnderPrefixWithoutFallingBackToSPA(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	server := &Server{cfg: Config{StaticDir: tmpDir}, mux: http.NewServeMux(), apiMux: http.NewServeMux()}
	server.routes()

	t.Run("api health works under prefix", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
			t.Fatalf("unexpected body: %q", body)
		}
	})

	t.Run("missing api route returns 404 instead of index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body == "<html>app</html>" {
			t.Fatalf("unexpected spa fallback body for api route")
		}
	})
}

func TestServerServesMountedAppAndAPI(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetsDir := filepath.Join(tmpDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index.js"), []byte("console.log('mounted')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	server := &Server{cfg: Config{StaticDir: tmpDir, AppBasePath: "/app"}, mux: http.NewServeMux(), apiMux: http.NewServeMux()}
	server.routes()

	t.Run("mounted spa route falls back to index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/app/dialogs", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "<html>app</html>" {
			t.Fatalf("unexpected body: %q", body)
		}
	})

	t.Run("mounted asset is served", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/app/assets/index.js", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "console.log('mounted')" {
			t.Fatalf("unexpected body: %q", body)
		}
	})

	t.Run("mounted api health works", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/app/api/health", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
			t.Fatalf("unexpected body: %q", body)
		}
	})

	t.Run("requests outside mount return 404", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/dialogs", nil)
		recorder := httptest.NewRecorder()
		server.serveHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", recorder.Code)
		}
	})
}

func TestResolveBotRuntimeProxyURL(t *testing.T) {
	got, err := resolveBotRuntimeProxyURL("http://bot-runtime:3100/base/", "/webhooks/telegram/operator", "a=1")
	if err != nil {
		t.Fatalf("resolveBotRuntimeProxyURL: %v", err)
	}
	want := "http://bot-runtime:3100/base/webhooks/telegram/operator?a=1"
	if got != want {
		t.Fatalf("unexpected proxy url: got %q want %q", got, want)
	}
}

func TestHandleWebhookProxiesTelegramRequestsToBotRuntime(t *testing.T) {
	var (
		gotPath            string
		gotMethod          string
		gotBody            string
		gotIngressSecret   string
		gotTelegramSecret  string
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody = string(body)
		gotIngressSecret = r.Header.Get(botRuntimeSecretHeader)
		gotTelegramSecret = r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true,"accepted":true}`))
	}))
	defer target.Close()

	server := &Server{
		cfg: Config{
			BotRuntimeBaseURL: target.URL,
			BotRuntimeSecret:  "shared-secret",
		},
	}

	request := httptest.NewRequest(http.MethodPost, "/webhooks/telegram/operator", strings.NewReader(`{"update_id":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "telegram-secret")
	recorder := httptest.NewRecorder()

	server.handleWebhook(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body != "{\"ok\":true,\"accepted\":true}" {
		t.Fatalf("unexpected body: %q", body)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected proxy method: %q", gotMethod)
	}
	if gotPath != "/webhooks/telegram/operator" {
		t.Fatalf("unexpected proxy path: %q", gotPath)
	}
	if gotBody != `{"update_id":1}` {
		t.Fatalf("unexpected proxy body: %q", gotBody)
	}
	if gotIngressSecret != "shared-secret" {
		t.Fatalf("unexpected proxy ingress secret: %q", gotIngressSecret)
	}
	if gotTelegramSecret != "telegram-secret" {
		t.Fatalf("unexpected telegram secret: %q", gotTelegramSecret)
	}
}
