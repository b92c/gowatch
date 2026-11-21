package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/b92c/gowatch/internal/alert"
)

// Options configures all supported alert notification channels.
type Options struct {
	WebhookURL        string
	SlackWebhookURL   string
	DiscordWebhookURL string
	TeamsWebhookURL   string
	PagerDutyKey      string
	PagerDutyEndpoint string
	SNSEndpointURL    string
	SMTP              SMTPOptions
	Desktop           bool
}

// SMTPOptions holds SMTP email delivery settings.
type SMTPOptions struct {
	Host     string
	Username string
	Password string // #nosec G117 -- runtime SMTP credential
	From     string
	To       []string
	Port     int
}

// Dispatcher sends alert notifications to configured channels with
// transition-level deduplication.
type Dispatcher struct {
	client  *http.Client
	sent    map[string]bool
	desktop func(title, body string) error
	smtpFn  func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
	opts    Options
	mu      sync.Mutex
}

// NewDispatcher creates a Dispatcher with the given options.
func NewDispatcher(opts Options) *Dispatcher {
	return &Dispatcher{
		opts:    opts,
		client:  &http.Client{Timeout: 5 * time.Second},
		sent:    make(map[string]bool),
		desktop: sendDesktopNotification,
		smtpFn:  smtp.SendMail,
	}
}

// Enabled reports whether at least one notification channel is configured.
func (d *Dispatcher) Enabled() bool {
	if d == nil {
		return false
	}
	o := d.opts
	return o.Desktop || o.WebhookURL != "" || o.SlackWebhookURL != "" ||
		o.DiscordWebhookURL != "" || o.TeamsWebhookURL != "" ||
		o.PagerDutyKey != "" || o.SNSEndpointURL != "" || o.SMTP.Host != ""
}

// Notify asynchronously dispatches a notification for an alert transition if it
// has not already been sent for that alert ID and status.
func (d *Dispatcher) Notify(a alert.Alert) {
	if !d.Enabled() || !d.markSent(a) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = d.SendSync(ctx, a)
	}()
}

func (d *Dispatcher) markSent(a alert.Alert) bool {
	key := a.ID + ":" + string(a.Status)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sent[key] {
		return false
	}
	d.sent[key] = true
	return true
}

// SendSync delivers the alert to every configured channel synchronously.
func (d *Dispatcher) SendSync(ctx context.Context, a alert.Alert) error {
	if !d.Enabled() {
		return nil
	}

	title := fmt.Sprintf("[GoWatch %s] %s (%s)", a.Severity, a.RuleID, a.Status)
	body := fmt.Sprintf("Service: %s | %s", a.ServiceName, a.Message)

	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if d.opts.WebhookURL != "" {
		record(d.postJSON(ctx, d.opts.WebhookURL, map[string]any{
			"id":           a.ID,
			"rule_id":      a.RuleID,
			"service":      a.ServiceName,
			"container_id": a.ContainerID,
			"severity":     string(a.Severity),
			"status":       string(a.Status),
			"message":      a.Message,
			"value":        a.Value,
			"fired_at":     a.FiredAt.Format(time.RFC3339),
		}))
	}
	if d.opts.SlackWebhookURL != "" {
		record(d.postJSON(ctx, d.opts.SlackWebhookURL, map[string]string{
			"text": fmt.Sprintf("*%s*\n%s", title, body),
		}))
	}
	if d.opts.DiscordWebhookURL != "" {
		record(d.postJSON(ctx, d.opts.DiscordWebhookURL, map[string]string{
			"content": fmt.Sprintf("**%s**\n%s", title, body),
		}))
	}
	if d.opts.TeamsWebhookURL != "" {
		record(d.postJSON(ctx, d.opts.TeamsWebhookURL, map[string]string{
			"@type": "MessageCard",
			"title": title,
			"text":  body,
		}))
	}
	if d.opts.PagerDutyKey != "" {
		endpoint := d.opts.PagerDutyEndpoint
		if endpoint == "" {
			endpoint = "https://events.pagerduty.com/v2/enqueue"
		}
		eventAction := "trigger"
		if a.Status == alert.StatusResolved {
			eventAction = "resolve"
		}
		record(d.postJSON(ctx, endpoint, map[string]any{
			"routing_key":  d.opts.PagerDutyKey,
			"event_action": eventAction,
			"dedup_key":    a.ID,
			"payload": map[string]any{
				"summary":  title + ": " + body,
				"source":   "gowatch",
				"severity": mapPagerDutySeverity(a.Severity),
			},
		}))
	}
	if d.opts.SNSEndpointURL != "" {
		record(d.postJSON(ctx, d.opts.SNSEndpointURL, map[string]string{
			"Subject": title,
			"Message": body,
		}))
	}
	if d.opts.SMTP.Host != "" && len(d.opts.SMTP.To) > 0 {
		record(d.sendEmail(title, body))
	}
	if d.opts.Desktop && d.desktop != nil {
		record(d.desktop(title, body))
	}

	return firstErr
}

func (d *Dispatcher) postJSON(ctx context.Context, url string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req) // #nosec G107 -- URL is explicitly configured by the user in trusted config
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("notification endpoint %s returned HTTP %d", url, resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) sendEmail(subject, body string) error {
	s := d.opts.SMTP
	port := s.Port
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", s.Host, port)
	from := s.From
	if from == "" {
		from = "gowatch@localhost"
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
		from, strings.Join(s.To, ", "), subject, body)
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	return d.smtpFn(addr, auth, from, s.To, []byte(msg))
}

func mapPagerDutySeverity(s alert.Severity) string {
	if s == alert.SeverityCritical {
		return "critical"
	}
	return "warning"
}

func sendDesktopNotification(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", body, title)
		return exec.Command("osascript", "-e", script).Run() // #nosec G204 -- fixed binary with quoted AppleScript literals
	case "linux":
		return exec.Command("notify-send", title, body).Run() // #nosec G204 -- fixed binary with explicit arguments
	default:
		return nil
	}
}
