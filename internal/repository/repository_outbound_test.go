package repository

import "testing"

func TestTelegramUpdateDedupIdentity(t *testing.T) {
	kind, value := telegramUpdateDedupIdentity(100, 200, "cbq_1")
	if kind != "callback" || value != "cbq_1" {
		t.Fatalf("expected callback identity, got %s %s", kind, value)
	}

	kind, value = telegramUpdateDedupIdentity(100, 200, "")
	if kind != "message" || value != "200" {
		t.Fatalf("expected message identity, got %s %s", kind, value)
	}

	kind, value = telegramUpdateDedupIdentity(100, 0, "")
	if kind != "update" || value != "100" {
		t.Fatalf("expected update identity, got %s %s", kind, value)
	}
}
