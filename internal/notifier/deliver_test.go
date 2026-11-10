package notifier

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

func testFindings() []internal.Finding {
	return []internal.Finding{{ID: "t", Severity: internal.SeverityWarning, Title: "Test", Description: "d"}}
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestDeliver_RedirectIsAFailure: net/http replays a redirected POST as a
// body-less GET. That used to deliver an empty request and log "sent".
func TestDeliver_RedirectIsAFailure(t *testing.T) {
	var finalHits atomic.Int32
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHits.Add(1)
	}))
	defer final.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	for _, typ := range []string{"generic", "discord", "slack", "gotify", "ntfy"} {
		n := New(nil, quietLogger())
		var status string
		n.SetResultHook(func(_, _, s string, _ int, _ string) { status = s })
		err := n.NotifyWebhook(internal.WebhookConfig{Name: "w", Type: typ, URL: redirector.URL, Enabled: true}, testFindings(), "host")
		if err == nil {
			t.Errorf("%s: redirecting webhook reported success", typ)
		}
		if status != "failed" {
			t.Errorf("%s: logged status %q, want failed", typ, status)
		}
	}
	if n := finalHits.Load(); n != 0 {
		t.Errorf("the redirect target received %d requests; redirects must not be followed", n)
	}
}

// TestDeliver_ErrorsDoNotCarryTheURL: for Discord, Slack and Gotify the
// URL is the credential, and the error text is stored and served back.
func TestDeliver_ErrorsDoNotCarryTheURL(t *testing.T) {
	// A closed port: connection refused.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL + "/api/webhooks/1234/SECRET-TOKEN-abc?token=QUERYSECRET"
	dead.Close()

	n := New(nil, quietLogger())
	var logged string
	n.SetResultHook(func(_, _, _ string, _ int, msg string) { logged = msg })
	err := n.NotifyWebhook(internal.WebhookConfig{Name: "w", Type: "discord", URL: url, Enabled: true}, testFindings(), "host")
	if err == nil {
		t.Fatal("expected a connection error")
	}
	for _, secret := range []string{"SECRET-TOKEN-abc", "QUERYSECRET", "/api/webhooks/"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(logged, secret) {
			t.Errorf("error text contains %q: %q", secret, err.Error())
		}
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error lost its cause: %q", err.Error())
	}
}

// TestDeliver_ErrorBodyIsBounded: only a short, single-line excerpt of the
// far end's answer is kept.
func TestDeliver_ErrorBodyIsBounded(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("first line\n\n   second line\t" + strings.Repeat("x", 100_000)))
	}))
	defer up.Close()

	n := New(nil, quietLogger())
	err := n.NotifyWebhook(internal.WebhookConfig{Name: "w", Type: "generic", URL: up.URL, Enabled: true}, testFindings(), "host")
	if err == nil {
		t.Fatal("expected an error for a 500")
	}
	msg := err.Error()
	if len(msg) > 300 {
		t.Errorf("error is %d bytes, want it truncated", len(msg))
	}
	if !strings.Contains(msg, "500") || !strings.Contains(msg, "first line second line") {
		t.Errorf("error should keep the status and a whitespace-collapsed excerpt: %q", msg)
	}
}

func TestDeliver_2xxIsSuccess(t *testing.T) {
	var gotBody atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost && len(b) > 0 {
			gotBody.Add(1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer up.Close()
	n := New(nil, quietLogger())
	if err := n.NotifyWebhook(internal.WebhookConfig{Name: "w", Type: "discord", URL: up.URL, Enabled: true}, testFindings(), "host"); err != nil {
		t.Fatalf("204 treated as failure: %v", err)
	}
	if gotBody.Load() != 1 {
		t.Error("the webhook did not receive a POST with a body")
	}
}
