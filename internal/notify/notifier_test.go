package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"sync"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/alert"
)

func TestDispatcherSendSyncAndDeduplication(t *testing.T) {
	var (
		mu       sync.Mutex
		received []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var desktopCalls int
	var smtpCalls int
	d := NewDispatcher(Options{
		WebhookURL:        srv.URL,
		SlackWebhookURL:   srv.URL,
		DiscordWebhookURL: srv.URL,
		TeamsWebhookURL:   srv.URL,
		PagerDutyKey:      "pd-key",
		PagerDutyEndpoint: srv.URL,
		SNSEndpointURL:    srv.URL,
		Desktop:           true,
		SMTP: SMTPOptions{
			Host: "smtp.example.com",
			Port: 587,
			From: "alerts@example.com",
			To:   []string{"ops@example.com"},
		},
	})
	d.desktop = func(title, body string) error {
		desktopCalls++
		return nil
	}
	d.smtpFn = func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		smtpCalls++
		return nil
	}

	a := alert.Alert{
		ID:          "c1:HIGH_CPU:1",
		ContainerID: "c1",
		ServiceName: "api",
		RuleID:      "HIGH_CPU",
		Message:     "CPU usage exceeds 85%: 92.0 (threshold: 85.0)",
		Severity:    alert.SeverityWarning,
		Status:      alert.StatusFiring,
		Value:       92.0,
		FiredAt:     time.Now(),
	}

	if err := d.SendSync(context.Background(), a); err != nil {
		t.Fatalf("SendSync() error: %v", err)
	}

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 6 {
		t.Errorf("expected 6 HTTP webhook payloads, got %d", count)
	}
	if desktopCalls != 1 || smtpCalls != 1 {
		t.Errorf("expected desktopCalls=1, smtpCalls=1; got %d, %d", desktopCalls, smtpCalls)
	}

	// Verify deduplication on same ID + Status.
	if !d.markSent(a) {
		t.Error("first markSent should return true")
	}
	if d.markSent(a) {
		t.Error("duplicate markSent for same ID and status should return false")
	}
	a.Status = alert.StatusResolved
	if !d.markSent(a) {
		t.Error("markSent for resolved transition should return true")
	}
}
