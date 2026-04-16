package scheduler

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// ── Test helpers ──

// fakeSender records NotifyWebhook calls for assertion.
type fakeSender struct {
	calls []fakeSendCall
	err   error // if non-nil, NotifyWebhook returns this error
}

type fakeSendCall struct {
	Webhook  internal.WebhookConfig
	Findings []internal.Finding
	Hostname string
}

func (s *fakeSender) NotifyWebhook(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	s.calls = append(s.calls, fakeSendCall{Webhook: wh, Findings: findings, Hostname: hostname})
	return s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testWebhook(name string) internal.WebhookConfig {
	return internal.WebhookConfig{
		Name:    name,
		URL:     "https://example.com/" + name,
		Type:    "generic",
		Enabled: true,
	}
}

func criticalFinding(category internal.Category, title string) internal.Finding {
	return internal.Finding{
		ID:       "test-" + title,
		Severity: internal.SeverityCritical,
		Category: category,
		Title:    title,
	}
}

func warningFinding(category internal.Category, title string) internal.Finding {
	return internal.Finding{
		ID:       "test-" + title,
		Severity: internal.SeverityWarning,
		Category: category,
		Title:    title,
	}
}

func infoFinding(category internal.Category, title string) internal.Finding {
	return internal.Finding{
		ID:       "test-" + title,
		Severity: internal.SeverityInfo,
		Category: category,
		Title:    title,
	}
}

// ── Tests: Rule Matching ──

func TestEvaluateRules_SeverityMatch_CriticalIncluded(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "critical-only",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "critical",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Findings) != 1 {
		t.Fatalf("expected 1 finding in action, got %d", len(actions[0].Findings))
	}
	if actions[0].Findings[0].Title != "Disk failing" {
		t.Errorf("expected finding title 'Disk failing', got %q", actions[0].Findings[0].Title)
	}
}

func TestEvaluateRules_SeverityMismatch_WarningExcludedByCriticalRule(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		warningFinding(internal.CategoryDisk, "Disk space low"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "critical-only",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "critical",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (warning doesn't match critical rule), got %d", len(actions))
	}
}

func TestEvaluateRules_WarningRuleIncludesCritical(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
		warningFinding(internal.CategoryDisk, "Disk space low"),
		infoFinding(internal.CategoryDisk, "Disk info"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "warnings-and-up",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "warning",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Findings) != 2 {
		t.Fatalf("expected 2 findings (critical + warning), got %d", len(actions[0].Findings))
	}
}

// ── Tests: Category Filtering ──

func TestEvaluateRules_CategoryFilter_DiskIncluded_NetworkExcluded(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
		criticalFinding(internal.CategoryNetwork, "Network down"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disk-category",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "category",
			Target:      "disk",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Findings) != 1 {
		t.Fatalf("expected 1 finding (disk only), got %d", len(actions[0].Findings))
	}
	if actions[0].Findings[0].Category != internal.CategoryDisk {
		t.Errorf("expected disk category, got %s", actions[0].Findings[0].Category)
	}
}

// ── Tests: Check Type / Non-Findings Category Filter ──

func TestEvaluateRules_NonFindingsCategory_SpeedCheckOnly(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	// Pre-evaluated findings from different categories.
	findings := []internal.Finding{
		warningFinding(internal.CategorySpeedTest, "Download speed below threshold"),
		warningFinding(internal.CategoryService, "Service down: my-api"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "speed-only",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "speedtest", // non-findings category
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Findings) != 1 {
		t.Fatalf("expected 1 finding (speedtest only), got %d", len(actions[0].Findings))
	}
	if actions[0].Findings[0].Category != internal.CategorySpeedTest {
		t.Errorf("expected speedtest category, got %s", actions[0].Findings[0].Category)
	}
}

// ── Tests: Cooldown Enforcement ──

func TestEvaluateRules_CooldownWithinWindow_Suppressed(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	finding := criticalFinding(internal.CategoryDisk, "Disk failing")
	fp := dispatcherFingerprint(finding)
	routeKey := "rule:r1"

	// Pre-seed: notification was sent 30 seconds ago.
	_ = store.SaveNotificationState(fp, routeKey, "sent", now.Add(-30*time.Second))

	findings := []internal.Finding{finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disk-alerts",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60, // 60 seconds cooldown — we're within it
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (within cooldown), got %d", len(actions))
	}
}

func TestEvaluateRules_CooldownExpired_Allowed(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	finding := criticalFinding(internal.CategoryDisk, "Disk failing")
	fp := dispatcherFingerprint(finding)
	routeKey := "rule:r1"

	// Pre-seed: notification was sent 120 seconds ago.
	_ = store.SaveNotificationState(fp, routeKey, "sent", now.Add(-120*time.Second))

	findings := []internal.Finding{finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disk-alerts",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60, // 60 seconds cooldown — we're past it
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action (cooldown expired), got %d", len(actions))
	}
	if len(actions[0].Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(actions[0].Findings))
	}
}

// ── Tests: Alert Suppression ──

func TestEvaluateRules_AcknowledgedAlert_NotDispatched(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	finding := criticalFinding(internal.CategoryDisk, "Disk failing")
	fp := dispatcherFingerprint(finding)

	// Pre-seed: this finding's alert has been acknowledged.
	store.SuppressAlert(fp, "acknowledged")

	findings := []internal.Finding{finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disk-alerts",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (acknowledged alert), got %d", len(actions))
	}
}

func TestEvaluateRules_SnoozedAlert_NotDispatched(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	finding := warningFinding(internal.CategorySMART, "SMART attribute degraded")
	fp := dispatcherFingerprint(finding)

	// Pre-seed: this finding's alert has been snoozed.
	store.SuppressAlert(fp, "snoozed")

	findings := []internal.Finding{finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "smart-alerts",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (snoozed alert), got %d", len(actions))
	}
}

// ── Tests: Multiple Rules ──

func TestEvaluateRules_MultipleRules_TwoWebhooks(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "discord-alerts",
			Enabled:     true,
			Webhook:     "discord",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
		{
			ID:          "r2",
			Name:        "slack-alerts",
			Enabled:     true,
			Webhook:     "slack",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{
		testWebhook("discord"),
		testWebhook("slack"),
	}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions (one per webhook), got %d", len(actions))
	}
	// Verify each action targets a different webhook.
	names := map[string]bool{}
	for _, a := range actions {
		names[a.Webhook.Name] = true
	}
	if !names["discord"] || !names["slack"] {
		t.Errorf("expected discord and slack webhooks, got %v", names)
	}
}

// ── Tests: No Matching Webhook ──

func TestEvaluateRules_NonexistentWebhook_SkippedGracefully(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "orphan-rule",
			Enabled:     true,
			Webhook:     "nonexistent-webhook",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (webhook doesn't exist), got %d", len(actions))
	}
}

// ── Tests: Disabled Rules and Webhooks ──

func TestEvaluateRules_DisabledRule_Skipped(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disabled-rule",
			Enabled:     false,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (disabled rule), got %d", len(actions))
	}
}

func TestEvaluateRules_DisabledWebhook_Skipped(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "good-rule",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	disabledWH := testWebhook("alerts")
	disabledWH.Enabled = false
	webhooks := []internal.WebhookConfig{disabledWH}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (disabled webhook), got %d", len(actions))
	}
}

// ── Tests: Legacy Mode (No Rules) ──

func TestEvaluateRules_NoRules_LegacyAllToAllEnabled(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
		warningFinding(internal.CategoryNetwork, "Network issue"),
	}
	webhooks := []internal.WebhookConfig{
		testWebhook("discord"),
		testWebhook("slack"),
	}
	now := time.Now()

	actions := nd.EvaluateRules(findings, nil, webhooks, 900, now)

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions (one per enabled webhook), got %d", len(actions))
	}
	for _, a := range actions {
		if len(a.Findings) != 2 {
			t.Errorf("expected 2 findings per action in legacy mode, got %d", len(a.Findings))
		}
	}
}

func TestEvaluateRules_NoRules_NoFindings_NoActions(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(nil, nil, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (no findings), got %d", len(actions))
	}
}

// ── Tests: Dedup Within Batch ──

func TestEvaluateRules_DuplicateFindings_Deduped(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	// Same finding duplicated.
	finding := criticalFinding(internal.CategoryDisk, "Disk failing")
	findings := []internal.Finding{finding, finding, finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "disk-alerts",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if len(actions[0].Findings) != 1 {
		t.Fatalf("expected 1 finding (deduped), got %d", len(actions[0].Findings))
	}
}

// ── Tests: Dispatch ──

func TestDispatch_Success_RecordsState(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	action := DispatchAction{
		Webhook:  testWebhook("alerts"),
		Findings: []internal.Finding{criticalFinding(internal.CategoryDisk, "Disk failing")},
		RouteKey: "rule:r1",
		RuleName: "disk-alerts",
	}

	err := nd.Dispatch(action, "my-nas", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sender was called.
	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}
	if sender.calls[0].Hostname != "my-nas" {
		t.Errorf("expected hostname 'my-nas', got %q", sender.calls[0].Hostname)
	}

	// Verify notification log was saved.
	log, err := store.GetNotificationLog(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log))
	}
	if log[0].Status != "sent" {
		t.Errorf("expected log status 'sent', got %q", log[0].Status)
	}

	// Verify notification state was saved (cooldown will apply next time).
	fp := dispatcherFingerprint(action.Findings[0])
	canSend, _ := store.CanSendNotification(fp, action.RouteKey, time.Minute, now)
	if canSend {
		t.Error("expected CanSendNotification to return false (just sent), but got true")
	}
}

func TestDispatch_SenderError_LogsFailed(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{err: errors.New("connection refused")}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	action := DispatchAction{
		Webhook:  testWebhook("alerts"),
		Findings: []internal.Finding{criticalFinding(internal.CategoryDisk, "Disk failing")},
		RouteKey: "rule:r1",
		RuleName: "disk-alerts",
	}

	err := nd.Dispatch(action, "my-nas", now)
	if err == nil {
		t.Fatal("expected error from Dispatch, got nil")
	}

	// Verify notification log records the failure.
	log, _ := store.GetNotificationLog(10)
	if len(log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log))
	}
	if log[0].Status != "failed" {
		t.Errorf("expected log status 'failed', got %q", log[0].Status)
	}
	if log[0].ErrorMessage != "connection refused" {
		t.Errorf("expected error message 'connection refused', got %q", log[0].ErrorMessage)
	}
}

func TestDispatch_EmptyFindings_NoOp(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	action := DispatchAction{
		Webhook:  testWebhook("alerts"),
		Findings: nil,
		RouteKey: "rule:r1",
	}

	err := nd.Dispatch(action, "my-nas", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.calls) != 0 {
		t.Errorf("expected 0 sender calls, got %d", len(sender.calls))
	}
}

// ── Tests: Webhook Name Case Insensitivity ──

func TestEvaluateRules_WebhookNameCaseInsensitive(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	findings := []internal.Finding{
		criticalFinding(internal.CategoryDisk, "Disk failing"),
	}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "alerts-rule",
			Enabled:     true,
			Webhook:     "My Alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 60,
		},
	}
	wh := testWebhook("my alerts") // lowercase
	webhooks := []internal.WebhookConfig{wh}
	now := time.Now()

	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action (case-insensitive match), got %d", len(actions))
	}
}

// ── Tests: Default Cooldown ──

func TestEvaluateRules_RuleWithZeroCooldown_UsesDefault(t *testing.T) {
	store := storage.NewFakeStore()
	sender := &fakeSender{}
	nd := NewNotificationDispatcher(store, store, sender, testLogger())

	now := time.Now()
	finding := criticalFinding(internal.CategoryDisk, "Disk failing")
	fp := dispatcherFingerprint(finding)
	routeKey := "rule:r1"

	// Pre-seed: notification was sent 100 seconds ago.
	_ = store.SaveNotificationState(fp, routeKey, "sent", now.Add(-100*time.Second))

	findings := []internal.Finding{finding}
	rules := []internal.NotificationRule{
		{
			ID:          "r1",
			Name:        "no-cooldown-rule",
			Enabled:     true,
			Webhook:     "alerts",
			Category:    "findings",
			Condition:   "any",
			CooldownSec: 0, // zero → uses default
		},
	}
	webhooks := []internal.WebhookConfig{testWebhook("alerts")}

	// Default cooldown is 900 seconds — sent 100s ago, so still within cooldown.
	actions := nd.EvaluateRules(findings, rules, webhooks, 900, now)

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (within default 900s cooldown), got %d", len(actions))
	}
}

// ── Tests: dispatcherFingerprint ──

func TestDispatcherFingerprint_Deterministic(t *testing.T) {
	f := internal.Finding{
		Category: internal.CategoryDisk,
		Title:    "Disk failing",
	}
	fp1 := dispatcherFingerprint(f)
	fp2 := dispatcherFingerprint(f)
	if fp1 != fp2 {
		t.Errorf("fingerprints should be deterministic: %q != %q", fp1, fp2)
	}
	if len(fp1) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got len=%d", len(fp1))
	}
}

func TestDispatcherFingerprint_DifferentFindings_DifferentFingerprints(t *testing.T) {
	f1 := criticalFinding(internal.CategoryDisk, "Disk A failing")
	f2 := criticalFinding(internal.CategoryDisk, "Disk B failing")
	fp1 := dispatcherFingerprint(f1)
	fp2 := dispatcherFingerprint(f2)
	if fp1 == fp2 {
		t.Error("different findings should have different fingerprints")
	}
}
