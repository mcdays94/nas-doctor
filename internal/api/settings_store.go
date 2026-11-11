package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Settings access
//
// Every read and write of the persisted settings blob goes through this
// file. Two rules hold:
//
//  1. Reads never write. getSettings used to marshal the blob back to the
//     store on every call (every page load, status poll and auth check),
//     so a reader that loaded just before a save could write the old blob
//     back after it and silently undo the save.
//  2. Writes are read-modify-write under settingsMu. Handlers that change
//     one field (chart range, dismissed findings, fleet list, theme) and
//     the full settings save all serialise on the same lock, so none of
//     them can overwrite another's change with a stale copy.

// ScanIntervalMin is the shortest global scan interval the API accepts.
// It matches the per-subsystem floor (ScanIntervalSecMin).
const ScanIntervalMin = ScanIntervalSecMin * time.Second

// defaultSettings returns the package defaults adjusted for this process:
// the scan interval a fresh install shows and runs is the one it was
// started with (NAS_DOCTOR_INTERVAL / -interval), not a hard-coded 30m.
func (s *Server) defaultSettings() Settings {
	d := defaultSettings()
	if s.bootScanInterval != "" {
		d.ScanInterval = s.bootScanInterval
	}
	return d
}

// SetBootScanInterval records the interval the process was started with so
// that it is the default until the user saves a different one.
func (s *Server) SetBootScanInterval(d time.Duration) {
	if d > 0 {
		s.bootScanInterval = FormatScanInterval(d)
	}
}

// SetStaticWebhooks registers webhooks that come from the environment or
// the config file. They are delivered to alongside the ones saved in
// Settings and are never written to the settings blob.
func (s *Server) SetStaticWebhooks(webhooks []internal.WebhookConfig) {
	s.staticWebhooks = append([]internal.WebhookConfig(nil), webhooks...)
}

// MergeWebhooks returns the saved webhooks followed by any static ones that
// are not already covered: a static webhook is skipped when a saved one has
// the same name or points at the same URL, so a destination configured both
// ways is not notified twice.
func MergeWebhooks(saved, static []internal.WebhookConfig) []internal.WebhookConfig {
	if len(static) == 0 {
		return saved
	}
	names := make(map[string]struct{}, len(saved))
	urls := make(map[string]struct{}, len(saved))
	out := make([]internal.WebhookConfig, 0, len(saved)+len(static))
	for _, wh := range saved {
		names[strings.ToLower(strings.TrimSpace(wh.Name))] = struct{}{}
		urls[strings.TrimSpace(wh.URL)] = struct{}{}
		out = append(out, wh)
	}
	for _, wh := range static {
		if _, dup := names[strings.ToLower(strings.TrimSpace(wh.Name))]; dup {
			continue
		}
		if _, dup := urls[strings.TrimSpace(wh.URL)]; dup {
			continue
		}
		out = append(out, wh)
	}
	return out
}

// FormatScanInterval renders d the way the settings UI writes intervals
// ("30m", "6h", "1h30m") rather than Go's "6h0m0s".
func FormatScanInterval(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// LoadSettings reads the persisted settings with defaults and migrations
// applied. found is false when nothing has been saved yet, in which case
// the returned value is the defaults. It is the read used at process
// startup so boot and the API agree on what the settings are.
func LoadSettings(store storage.ConfigStore, bootInterval time.Duration) (settings Settings, found bool, err error) {
	settings = defaultSettings()
	if bootInterval > 0 {
		settings.ScanInterval = FormatScanInterval(bootInterval)
	}
	raw, err := store.GetConfig(settingsConfigKey)
	if err != nil {
		if errors.Is(err, storage.ErrConfigNotFound) {
			return settings, false, nil
		}
		return settings, false, err
	}
	if raw == "" {
		return settings, false, nil
	}
	settings = migrateSettings([]byte(raw), settings)
	if settings.SettingsVersion < currentSettingsVersion {
		settings.SettingsVersion = currentSettingsVersion
	}
	return settings, true, nil
}

// readSettings is the lock-free read shared by loadSettings and the write
// path. stale reports that the stored blob predates currentSettingsVersion
// and should be rewritten once.
func (s *Server) readSettings() (settings Settings, stale bool, err error) {
	settings = s.defaultSettings()
	if s.store == nil {
		return settings, false, nil
	}
	raw, err := s.store.GetConfig(settingsConfigKey)
	if err != nil {
		if errors.Is(err, storage.ErrConfigNotFound) {
			return settings, false, nil
		}
		return settings, false, err
	}
	if raw == "" {
		return settings, false, nil
	}
	settings = migrateSettings([]byte(raw), settings)
	if settings.SettingsVersion < currentSettingsVersion {
		settings.SettingsVersion = currentSettingsVersion
		stale = true
	}
	return settings, stale, nil
}

// loadSettings returns the stored settings with defaults and migrations
// applied. Unlike getSettings it reports a failed read instead of quietly
// substituting defaults, so callers that must not mistake "database error"
// for "nothing configured" (authentication above all) can tell the two
// apart.
//
// The only time it writes is the one-off rewrite of a blob that predates
// currentSettingsVersion, done under settingsMu after re-reading so it
// cannot clobber a concurrent save.
func (s *Server) loadSettings() (Settings, error) {
	settings, stale, err := s.readSettings()
	if err != nil || !stale {
		return settings, err
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	settings, stale, err = s.readSettings()
	if err == nil && stale {
		if perr := s.persistSettingsLocked(settings); perr != nil {
			s.logger.Warn("failed to persist migrated settings", "error", perr)
		}
	}
	return settings, err
}

// getSettings loads the current settings, falling back to defaults when
// they cannot be read. Use loadSettings where that distinction matters.
func (s *Server) getSettings() Settings {
	settings, err := s.loadSettings()
	if err != nil && s.logger != nil {
		s.logger.Warn("failed to read settings, using defaults", "error", err)
	}
	return settings
}

// persistSettingsLocked writes settings to the store. The caller holds
// settingsMu.
func (s *Server) persistSettingsLocked(settings Settings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return s.store.SetConfig(settingsConfigKey, string(data))
}

// updateSettings applies fn to the current settings and persists the
// result, all under settingsMu. If fn returns an error nothing is written.
// It refuses to write when the current settings cannot be read, so a
// transient read failure can never replace the stored blob with defaults.
func (s *Server) updateSettings(fn func(*Settings) error) (Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	settings, _, err := s.readSettings()
	if err != nil {
		return settings, fmt.Errorf("read settings: %w", err)
	}
	if err := fn(&settings); err != nil {
		return settings, err
	}
	if err := s.persistSettingsLocked(settings); err != nil {
		return settings, err
	}
	return settings, nil
}

// settingsAtomicKeys are top-level settings whose value is replaced whole
// by a save rather than merged key by key. They are maps the client owns
// in full, where a key missing from the request means "remove it".
var settingsAtomicKeys = map[string]bool{
	"section_heights": true,
	"section_order":   true,
}

// mergeSettingsPatch applies a JSON settings document on top of current
// and returns the result. Objects merge key by key; arrays and scalars
// replace. A key the request does not mention keeps its stored value.
//
// The save endpoint used to replace the whole blob with whatever the
// client sent, so every field a client did not know about — the dashboard
// layout and chart range for the settings page, anything at all for a
// partial API call — was reset to its zero value by each save.
func mergeSettingsPatch(current Settings, body []byte) (Settings, error) {
	var merged Settings

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var patch map[string]any
	if err := dec.Decode(&patch); err != nil {
		return merged, fmt.Errorf("invalid JSON: %w", err)
	}
	if patch == nil {
		return merged, errors.New("invalid JSON: settings must be an object")
	}
	if dec.More() {
		return merged, errors.New("invalid JSON: unexpected data after settings object")
	}

	curJSON, err := json.Marshal(current)
	if err != nil {
		return merged, fmt.Errorf("marshal settings: %w", err)
	}
	baseDec := json.NewDecoder(bytes.NewReader(curJSON))
	baseDec.UseNumber()
	var base map[string]any
	if err := baseDec.Decode(&base); err != nil {
		return merged, fmt.Errorf("decode settings: %w", err)
	}

	mergeJSONObject(base, patch, settingsAtomicKeys)

	out, err := json.Marshal(base)
	if err != nil {
		return merged, fmt.Errorf("marshal settings: %w", err)
	}
	// Decode into a zero value, not into current: encoding/json reuses the
	// elements of an existing slice, which would let fields of a stored
	// list entry leak into the entry that replaces it.
	if err := json.Unmarshal(out, &merged); err != nil {
		return merged, fmt.Errorf("invalid JSON: %w", err)
	}
	return merged, nil
}

// mergeJSONObject copies src into dst. Where both hold an object for the
// same key the two are merged recursively, unless the key is in atomic.
func mergeJSONObject(dst, src map[string]any, atomic map[string]bool) {
	for k, v := range src {
		srcObj, srcIsObj := v.(map[string]any)
		dstObj, dstIsObj := dst[k].(map[string]any)
		if srcIsObj && dstIsObj && !atomic[k] {
			mergeJSONObject(dstObj, srcObj, nil)
			continue
		}
		dst[k] = v
	}
}

// jsonHasPath reports whether raw is an object containing the given chain
// of keys. Migrations use it to tell a value the user stored from one that
// is merely absent.
func jsonHasPath(raw []byte, path ...string) bool {
	cur := json.RawMessage(raw)
	for _, key := range path {
		var obj map[string]json.RawMessage
		if json.Unmarshal(cur, &obj) != nil {
			return false
		}
		next, ok := obj[key]
		if !ok {
			return false
		}
		cur = next
	}
	return true
}

// ResetAPIKey removes the API key from the stored settings, leaving
// everything else untouched. It backs the -reset-api-key command-line
// flag: with sign-in on, the web UI is behind the key too and a lost key
// can no longer be read off the Settings page, so there has to be a way
// back in for whoever controls the host (see also ShowAPIKey). removed is
// false when no key was set.
func ResetAPIKey(store storage.ConfigStore) (removed bool, err error) {
	raw, err := store.GetConfig(settingsConfigKey)
	if err != nil {
		if errors.Is(err, storage.ErrConfigNotFound) {
			return false, nil
		}
		return false, err
	}
	// Edit the raw document rather than round-tripping the struct, so the
	// command cannot change anything but the one key.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return false, fmt.Errorf("stored settings are not valid JSON: %w", err)
	}
	cur, ok := doc["api_key"]
	if !ok || string(cur) == `""` || string(cur) == "null" {
		return false, nil
	}
	delete(doc, "api_key")
	out, err := json.Marshal(doc)
	if err != nil {
		return false, err
	}
	return true, store.SetConfig(settingsConfigKey, string(out))
}
