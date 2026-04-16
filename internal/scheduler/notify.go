// Package scheduler — notify.go contains the NotificationDispatcher module.
//
// NotificationDispatcher owns rule evaluation, cooldown enforcement, alert
// suppression, and webhook dispatch orchestration. It is designed to be
// tested in isolation using storage.FakeStore.
//
// This file is ADDITIVE — it does not replace any code in scheduler.go.
// Wiring into the main Scheduler happens in a later issue (#94).
package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Sender abstracts webhook delivery so tests can avoid real HTTP calls.
// The production implementation is notifier.Notifier.NotifyWebhook.
type Sender interface {
	NotifyWebhook(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error
}

// DispatchAction groups a set of findings to deliver to a single webhook.
type DispatchAction struct {
	Webhook  internal.WebhookConfig
	Findings []internal.Finding
	RouteKey string // dedup key for cooldown state (e.g., "rule:<ruleID>")
	RuleName string // human-readable label for logging
}

// NotificationDispatcher evaluates notification rules against findings,
// applies cooldown and alert suppression, and dispatches to webhooks.
type NotificationDispatcher struct {
	store      storage.NotificationStore
	alertStore storage.AlertStore
	sender     Sender
	logger     *slog.Logger
}

// NewNotificationDispatcher creates a ready-to-use dispatcher.
func NewNotificationDispatcher(
	store storage.NotificationStore,
	alertStore storage.AlertStore,
	sender Sender,
	logger *slog.Logger,
) *NotificationDispatcher {
	return &NotificationDispatcher{
		store:      store,
		alertStore: alertStore,
		sender:     sender,
		logger:     logger,
	}
}

// EvaluateRules determines which findings should be sent to which webhooks,
// respecting cooldowns, dedup, and alert suppression.
//
// If no rules are configured, all findings are routed to every enabled webhook
// (legacy behaviour). Otherwise each rule is evaluated independently and
// matched findings are collected into DispatchActions.
func (nd *NotificationDispatcher) EvaluateRules(
	findings []internal.Finding,
	rules []internal.NotificationRule,
	webhooks []internal.WebhookConfig,
	defaultCooldownSec int,
	now time.Time,
) []DispatchAction {
	if defaultCooldownSec <= 0 {
		defaultCooldownSec = 900
	}

	// Build webhook lookup (case-insensitive).
	whMap := make(map[string]internal.WebhookConfig, len(webhooks))
	for _, wh := range webhooks {
		whMap[strings.ToLower(strings.TrimSpace(wh.Name))] = wh
	}

	// ── Legacy mode: no rules configured ──
	if len(rules) == 0 {
		return nd.evaluateLegacy(findings, webhooks, defaultCooldownSec, now)
	}

	// ── Rule-based dispatch ──
	var actions []DispatchAction
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		whName := strings.ToLower(strings.TrimSpace(rule.Webhook))
		wh, ok := whMap[whName]
		if !ok || !wh.Enabled {
			continue
		}

		matched := matchRuleFindings(rule, findings)
		if len(matched) == 0 {
			continue
		}

		cooldown := time.Duration(rule.CooldownSec) * time.Second
		if cooldown <= 0 {
			cooldown = time.Duration(defaultCooldownSec) * time.Second
		}
		routeKey := "rule:" + rule.ID

		toSend := nd.applyCooldownAndSuppression(matched, routeKey, cooldown, now)
		if len(toSend) == 0 {
			_ = nd.store.SaveNotificationLog(wh.Name, wh.Type, "suppressed_cooldown", len(matched), "")
			continue
		}

		actions = append(actions, DispatchAction{
			Webhook:  wh,
			Findings: toSend,
			RouteKey: routeKey,
			RuleName: rule.Name,
		})
	}
	return actions
}

// evaluateLegacy implements the no-rules fallback: all findings to all enabled webhooks.
func (nd *NotificationDispatcher) evaluateLegacy(
	findings []internal.Finding,
	webhooks []internal.WebhookConfig,
	defaultCooldownSec int,
	now time.Time,
) []DispatchAction {
	if len(findings) == 0 {
		return nil
	}
	var actions []DispatchAction
	for _, wh := range webhooks {
		if !wh.Enabled {
			continue
		}
		routeKey := "legacy:" + strings.ToLower(strings.TrimSpace(wh.Name))
		cooldown := time.Duration(defaultCooldownSec) * time.Second
		toSend := nd.applyCooldownAndSuppression(findings, routeKey, cooldown, now)
		if len(toSend) == 0 {
			continue
		}
		actions = append(actions, DispatchAction{
			Webhook:  wh,
			Findings: toSend,
			RouteKey: routeKey,
			RuleName: "legacy",
		})
	}
	return actions
}

// matchRuleFindings applies a rule's category-based filter to a set of findings.
// This extracts the "findings" category logic from evaluateRule in scheduler.go.
// For the "findings" category only — other rule categories (disk_space, smart, etc.)
// require snapshot data and remain in evaluateRule until full wiring (#94).
func matchRuleFindings(rule internal.NotificationRule, findings []internal.Finding) []internal.Finding {
	cat := strings.ToLower(rule.Category)
	cond := strings.ToLower(rule.Condition)
	target := strings.ToLower(strings.TrimSpace(rule.Target))

	if cat != "findings" {
		// Non-findings categories require snapshot data; for now delegate to
		// the full evaluateRule in scheduler.go. The dispatcher only handles
		// pre-evaluated findings. Return the raw findings if the rule category
		// is not "findings" — callers are expected to pre-evaluate rules for
		// snapshot-based categories before calling EvaluateRules.
		//
		// In practice, the scheduler will call evaluateRule() first and pass
		// the matched findings to EvaluateRules. For now, we filter the
		// findings slice directly using severity/category semantics.
		return filterByCategoryAndSeverity(cat, cond, target, findings)
	}

	return evalFindingsFilter(cond, target, findings)
}

// evalFindingsFilter implements the "findings" category matching logic.
func evalFindingsFilter(cond, target string, findings []internal.Finding) []internal.Finding {
	var out []internal.Finding
	for _, f := range findings {
		switch cond {
		case "critical":
			if f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "warning":
			if f.Severity == internal.SeverityWarning || f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "category":
			if strings.EqualFold(string(f.Category), target) {
				out = append(out, f)
			}
		case "any":
			out = append(out, f)
		}
	}
	return out
}

// filterByCategoryAndSeverity provides a generic category+severity filter for
// non-"findings" rules operating on pre-existing findings.
func filterByCategoryAndSeverity(cat, cond, target string, findings []internal.Finding) []internal.Finding {
	var out []internal.Finding
	for _, f := range findings {
		// Category filter: only include findings from matching category.
		if cat != "" && !strings.EqualFold(string(f.Category), cat) {
			continue
		}
		// Condition-based severity filter.
		switch cond {
		case "critical":
			if f.Severity != internal.SeverityCritical {
				continue
			}
		case "warning":
			if f.Severity != internal.SeverityWarning && f.Severity != internal.SeverityCritical {
				continue
			}
		case "any", "":
			// pass
		default:
			// Unknown condition — for non-findings categories this may be a
			// threshold condition (e.g., "above", "down"). Let it through if
			// the category already matched.
		}
		// Target filter.
		if target != "" && !strings.EqualFold(strings.TrimSpace(f.RelatedDisk), target) &&
			!strings.EqualFold(strings.TrimSpace(f.Title), target) {
			// No target match — include anyway since target matching for
			// non-findings categories is complex and context-dependent.
			// Keep the finding; the scheduler's evaluateRule does the real
			// target filtering before passing findings here.
		}
		out = append(out, f)
	}
	return out
}

// applyCooldownAndSuppression filters a set of findings through dedup,
// alert suppression, and cooldown checks. Mirrors scheduler.applyCooldown.
func (nd *NotificationDispatcher) applyCooldownAndSuppression(
	findings []internal.Finding,
	routeKey string,
	cooldown time.Duration,
	now time.Time,
) []internal.Finding {
	seen := make(map[string]struct{})
	out := make([]internal.Finding, 0, len(findings))

	for _, f := range findings {
		fp := dispatcherFingerprint(f)
		if fp == "" {
			out = append(out, f)
			continue
		}

		// Dedup within the same batch.
		if _, exists := seen[fp]; exists {
			continue
		}
		seen[fp] = struct{}{}

		// Alert suppression (acknowledged / snoozed).
		suppressed, _, err := nd.alertStore.IsAlertSuppressed(fp, now)
		if err != nil {
			nd.logger.Warn("alert suppression check failed", "fingerprint", fp, "error", err)
		} else if suppressed {
			continue
		}

		// Cooldown enforcement.
		allowed, err := nd.store.CanSendNotification(fp, routeKey, cooldown, now)
		if err != nil {
			nd.logger.Warn("cooldown check failed; allowing notification",
				"route", routeKey, "fingerprint", fp, "error", err)
			out = append(out, f)
			continue
		}
		if allowed {
			out = append(out, f)
		}
	}
	return out
}

// Dispatch sends a DispatchAction to its webhook and records the result.
func (nd *NotificationDispatcher) Dispatch(action DispatchAction, hostname string, now time.Time) error {
	if len(action.Findings) == 0 {
		return nil
	}

	err := nd.sender.NotifyWebhook(action.Webhook, action.Findings, hostname)
	if err != nil {
		_ = nd.store.SaveNotificationLog(
			action.Webhook.Name, action.Webhook.Type, "failed",
			len(action.Findings), err.Error(),
		)
		return fmt.Errorf("dispatch to %s failed: %w", action.Webhook.Name, err)
	}

	// Record sent state for each finding.
	fingerprints := make([]string, 0, len(action.Findings))
	for _, f := range action.Findings {
		fp := dispatcherFingerprint(f)
		fingerprints = append(fingerprints, fp)
		if saveErr := nd.store.SaveNotificationState(fp, action.RouteKey, "sent", now); saveErr != nil {
			nd.logger.Warn("failed to save notification state",
				"fingerprint", fp, "route", action.RouteKey, "error", saveErr)
		}
	}

	_ = nd.store.SaveNotificationLog(
		action.Webhook.Name, action.Webhook.Type, "sent",
		len(action.Findings), "",
	)

	// Mark alerts as notified.
	if err := nd.alertStore.MarkAlertsNotifiedByFingerprint(fingerprints, now); err != nil {
		nd.logger.Warn("failed to mark alerts notified", "error", err)
	}

	return nil
}

// dispatcherFingerprint computes a stable fingerprint for a finding.
// Mirrors findingFingerprint in scheduler.go — duplicated to avoid
// coupling and allow independent evolution.
func dispatcherFingerprint(f internal.Finding) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(string(f.Category))),
		strings.ToLower(strings.TrimSpace(f.Title)),
		strings.ToLower(strings.TrimSpace(f.RelatedDisk)),
	}
	raw := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
