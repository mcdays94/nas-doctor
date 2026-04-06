// Package notifier handles sending alerts via webhooks and other channels.
package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Notifier dispatches alerts to configured webhook endpoints.
type Notifier struct {
	webhooks []internal.WebhookConfig
	client   *http.Client
	logger   *slog.Logger
}

// New creates a Notifier with the given webhook configurations.
func New(webhooks []internal.WebhookConfig, logger *slog.Logger) *Notifier {
	return &Notifier{
		webhooks: webhooks,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// NotifyFindings sends alerts for the given findings to all configured webhooks.
func (n *Notifier) NotifyFindings(findings []internal.Finding, hostname string) {
	for _, wh := range n.webhooks {
		if !wh.Enabled {
			continue
		}
		// Filter findings by minimum severity
		filtered := filterBySeverity(findings, wh.MinLevel)
		if len(filtered) == 0 {
			continue
		}

		var err error
		switch strings.ToLower(wh.Type) {
		case "discord":
			err = n.sendDiscord(wh, filtered, hostname)
		case "slack":
			err = n.sendSlack(wh, filtered, hostname)
		case "gotify":
			err = n.sendGotify(wh, filtered, hostname)
		case "ntfy":
			err = n.sendNtfy(wh, filtered, hostname)
		default:
			err = n.sendGeneric(wh, filtered, hostname)
		}

		if err != nil {
			n.logger.Error("webhook notification failed", "name", wh.Name, "type", wh.Type, "error", err)
		} else {
			n.logger.Info("webhook notification sent", "name", wh.Name, "type", wh.Type, "findings", len(filtered))
		}
	}
}

// ---------- Discord ----------

type discordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []discordEmbed `json:"embeds"`
}
type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}
type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func (n *Notifier) sendDiscord(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	critical, warnings, infos := countBySeverity(findings)

	embed := discordEmbed{
		Title:       fmt.Sprintf("🏥 NAS Doctor — %s", hostname),
		Description: fmt.Sprintf("Diagnostic scan complete: **%d critical**, **%d warnings**, **%d info**", critical, warnings, infos),
		Color:       discordColor(findings),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	// Add top findings as fields (max 10)
	for i, f := range findings {
		if i >= 10 {
			break
		}
		embed.Fields = append(embed.Fields, discordField{
			Name:   fmt.Sprintf("%s %s", severityEmoji(f.Severity), f.Title),
			Value:  truncate(f.Description, 200),
			Inline: false,
		})
	}

	payload := discordPayload{Embeds: []discordEmbed{embed}}
	return n.postJSON(wh, payload)
}

// ---------- Slack ----------

type slackPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}
type slackBlock struct {
	Type string     `json:"type"`
	Text *slackText `json:"text,omitempty"`
}
type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (n *Notifier) sendSlack(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	critical, warnings, infos := countBySeverity(findings)

	var blocks []slackBlock
	blocks = append(blocks, slackBlock{
		Type: "header",
		Text: &slackText{Type: "plain_text", Text: fmt.Sprintf("🏥 NAS Doctor — %s", hostname)},
	})
	blocks = append(blocks, slackBlock{
		Type: "section",
		Text: &slackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%d critical* | *%d warnings* | *%d info*", critical, warnings, infos),
		},
	})

	for i, f := range findings {
		if i >= 8 {
			break
		}
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("%s *%s*\n%s", severityEmoji(f.Severity), f.Title, truncate(f.Description, 200)),
			},
		})
	}

	payload := slackPayload{
		Text:   fmt.Sprintf("NAS Doctor: %d critical, %d warnings on %s", critical, warnings, hostname),
		Blocks: blocks,
	}
	return n.postJSON(wh, payload)
}

// ---------- Gotify ----------

type gotifyPayload struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

func (n *Notifier) sendGotify(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	critical, warnings, _ := countBySeverity(findings)
	priority := 5
	if critical > 0 {
		priority = 8
	} else if warnings > 0 {
		priority = 6
	}

	var msg strings.Builder
	for i, f := range findings {
		if i >= 10 {
			break
		}
		msg.WriteString(fmt.Sprintf("%s %s\n  %s\n\n", severityEmoji(f.Severity), f.Title, truncate(f.Description, 150)))
	}

	payload := gotifyPayload{
		Title:    fmt.Sprintf("NAS Doctor — %s", hostname),
		Message:  msg.String(),
		Priority: priority,
	}
	return n.postJSON(wh, payload)
}

// ---------- Ntfy ----------

func (n *Notifier) sendNtfy(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	critical, warnings, _ := countBySeverity(findings)

	priority := "default"
	if critical > 0 {
		priority = "urgent"
	} else if warnings > 0 {
		priority = "high"
	}

	var body strings.Builder
	for i, f := range findings {
		if i >= 10 {
			break
		}
		body.WriteString(fmt.Sprintf("%s %s: %s\n", severityEmoji(f.Severity), f.Title, truncate(f.Description, 100)))
	}

	req, err := http.NewRequest("POST", wh.URL, strings.NewReader(body.String()))
	if err != nil {
		return err
	}
	req.Header.Set("Title", fmt.Sprintf("NAS Doctor — %s: %d critical, %d warnings", hostname, critical, warnings))
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", "hospital,server")

	for k, v := range wh.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ntfy returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ---------- Generic Webhook ----------

type genericPayload struct {
	Hostname  string             `json:"hostname"`
	Timestamp string             `json:"timestamp"`
	Summary   genericSummary     `json:"summary"`
	Findings  []internal.Finding `json:"findings"`
}
type genericSummary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

func (n *Notifier) sendGeneric(wh internal.WebhookConfig, findings []internal.Finding, hostname string) error {
	critical, warnings, infos := countBySeverity(findings)
	payload := genericPayload{
		Hostname:  hostname,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Summary: genericSummary{
			Critical: critical,
			Warning:  warnings,
			Info:     infos,
			Total:    len(findings),
		},
		Findings: findings,
	}
	return n.postJSON(wh, payload)
}

// ---------- Helpers ----------

func (n *Notifier) postJSON(wh internal.WebhookConfig, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Custom headers
	for k, v := range wh.Headers {
		req.Header.Set(k, v)
	}

	// HMAC signing for generic webhooks
	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(data)
		req.Header.Set("X-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func filterBySeverity(findings []internal.Finding, minLevel internal.Severity) []internal.Finding {
	minRank := severityRank(minLevel)
	var filtered []internal.Finding
	for _, f := range findings {
		if severityRank(f.Severity) >= minRank {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func severityRank(s internal.Severity) int {
	switch s {
	case internal.SeverityCritical:
		return 3
	case internal.SeverityWarning:
		return 2
	case internal.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func countBySeverity(findings []internal.Finding) (critical, warnings, infos int) {
	for _, f := range findings {
		switch f.Severity {
		case internal.SeverityCritical:
			critical++
		case internal.SeverityWarning:
			warnings++
		case internal.SeverityInfo:
			infos++
		}
	}
	return
}

func severityEmoji(s internal.Severity) string {
	switch s {
	case internal.SeverityCritical:
		return "🔴"
	case internal.SeverityWarning:
		return "🟡"
	case internal.SeverityInfo:
		return "🔵"
	default:
		return "⚪"
	}
}

func discordColor(findings []internal.Finding) int {
	for _, f := range findings {
		if f.Severity == internal.SeverityCritical {
			return 0xDC2626 // red
		}
	}
	for _, f := range findings {
		if f.Severity == internal.SeverityWarning {
			return 0xD97706 // amber
		}
	}
	return 0x16A34A // green
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
