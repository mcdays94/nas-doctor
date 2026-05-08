// Duplicacy backup-monitor runner. Disk-read by design — no
// `duplicacy` binary is invoked, no subprocess spawned, no network
// calls made. The runner reads the on-disk JSON snapshot files that
// Duplicacy writes to its local cache, which is identical between the
// vanilla CLI install and the saspus/duplicacy-web container layout —
// only the directory structure around the snapshots differs.
//
// Two path-resolvers handle the two layouts:
//
//   - kind=cli-repo : Path points at the repo root. The runner reads
//     <Path>/.duplicacy/preferences (JSON array of storage configs),
//     picks the first entry's "name" as the storage name (defaulting
//     to "default" when the file lacks one), and walks
//     <Path>/.duplicacy/cache/<storage>/snapshots/.
//
//   - kind=web-cache : Path points at the cache root that the
//     saspus/duplicacy-web container writes under (typically
//     /cache/localhost/0/.duplicacy/cache or similar). StorageID
//     names the per-repo subdir under it. The runner walks
//     <Path>/<StorageID>/snapshots/ directly.
//
// Both resolvers converge on the same JSON-snapshot parser
// (parseDuplicacySnapshotFile). PRD #310 / issue #311.
package collector

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DuplicacyRunner is the deep-module interface that separates
// Duplicacy disk-read mechanics from the rest of the backup
// collector. Production code uses diskDuplicacyRunner; tests can
// inject a fake by implementing this single-method interface. See
// PRD #310 §1 for the architectural choice (disk-read, not
// subprocess).
//
// Read inspects the on-disk state of one configured Duplicacy entry
// and returns a populated DuplicacyState. The function is total —
// every classifier outcome maps to a non-error return with the
// corresponding ReasonCode. Errors are reserved for genuinely-
// unexpected cases (e.g. an unrelated I/O fault that escaped the
// classifier); callers should treat them as DuplicacyReasonUnknown
// for UI purposes. V1a callers in V1b/V1c will dispatch on
// state.ReasonCode regardless.
type DuplicacyRunner interface {
	Read(ctx context.Context, entry DuplicacyEntry) (DuplicacyState, error)
}

// DuplicacyEntry mirrors api.DuplicacyEntry in shape. Lives in the
// collector package so callers below the api layer can pass entries
// without importing api. Issue #311.
type DuplicacyEntry struct {
	// Enabled toggles the entry off without deleting its config.
	Enabled bool
	// Label is a user-supplied display name; optional.
	Label string
	// Kind selects the path-resolver. Closed set: DuplicacyKindCLIRepo,
	// DuplicacyKindWebCache.
	Kind string
	// Path is the container-visible path. Repo root for cli-repo;
	// cache root for web-cache.
	Path string
	// StorageID names the per-repo subdir under Path when Kind=
	// web-cache. Ignored for cli-repo (resolved from preferences).
	StorageID string
	// StaleAfter is the age threshold in days. Zero means "use
	// default" — the runner substitutes DuplicacyDefaultStaleAfterDays
	// at read time.
	StaleAfter int
}

// Duplicacy entry kind values. The runner switches on Kind to pick
// its path-resolver.
const (
	DuplicacyKindCLIRepo  = "cli-repo"
	DuplicacyKindWebCache = "web-cache"
)

// DuplicacyDefaultStaleAfterDays is the StaleAfter default applied
// when DuplicacyEntry.StaleAfter is zero. Substituted at read time —
// not at config time — so users can leave the field blank and get
// sensible behaviour without persisting a magic number into their
// settings blob (PRD #310 §10 user story 5; issue #311 acceptance
// criterion 2).
const DuplicacyDefaultStaleAfterDays = 30

// DuplicacyReason is the closed-set classifier outcome for one
// entry's read. Per-provider type — NOT shared with BorgReason — so
// the precedent for adding Restic, PBS, Duplicati later is "add a
// per-provider reason type" rather than "extend a god-enum"
// (PRD #310 §4 / user story 12).
type DuplicacyReason string

// All eight DuplicacyReason values. The runner produces exactly one
// ReasonCode per Read() call (currently_running is orthogonal — see
// DuplicacyState.CurrentlyRunning).
const (
	// DuplicacyReasonOK — repo healthy, recent snapshot found, count
	// and timestamps populated. Renders as success on the dashboard.
	DuplicacyReasonOK DuplicacyReason = "ok"

	// DuplicacyReasonPathNotFound — the configured Path does not
	// exist (typo, missing bind mount, deleted directory). Surfaced
	// distinctly from "not a Duplicacy repo" so users know whether
	// the issue is in their docker-compose or their Duplicacy
	// config (user story 15).
	DuplicacyReasonPathNotFound DuplicacyReason = "path_not_found"

	// DuplicacyReasonPathUnreadable — Path exists but stat()s
	// produce a permission/IO error. Different from PathNotFound
	// so a chmod/SELinux fix points the user at the right knob.
	DuplicacyReasonPathUnreadable DuplicacyReason = "path_unreadable"

	// DuplicacyReasonNotARepo — Path exists and is readable but
	// the expected `.duplicacy/` subdirectory (cli-repo) is absent.
	// User has the directory mounted but it isn't a Duplicacy repo.
	DuplicacyReasonNotARepo DuplicacyReason = "not_a_duplicacy_repo"

	// DuplicacyReasonStorageIDNotFound — Path resolves but the named
	// StorageID subdirectory under it (web-cache) is absent. User
	// likely typo'd the storage_id field (user story 7).
	DuplicacyReasonStorageIDNotFound DuplicacyReason = "storage_id_not_found"

	// DuplicacyReasonNoSnapshotsYet — repo configuration is valid
	// but no snapshot files exist yet. Distinct from "broken" so a
	// freshly-configured but never-run repo isn't red-flagged
	// (user story 4).
	DuplicacyReasonNoSnapshotsYet DuplicacyReason = "no_snapshots_yet"

	// DuplicacyReasonStale — newest snapshot is older than the
	// entry's StaleAfter threshold. Most likely cause: backup cron
	// silently failed (user story 5).
	DuplicacyReasonStale DuplicacyReason = "stale"

	// DuplicacyReasonCorruptSnapshot — at least one snapshot file
	// failed JSON-unmarshal. Graceful failure — surface the reason
	// rather than crashing (user story 14). When mixed with valid
	// snapshots in the same repo, the runner prefers the corrupt
	// signal so users notice partial damage.
	DuplicacyReasonCorruptSnapshot DuplicacyReason = "corrupt_snapshot"
)

// DuplicacyState is the runner's per-entry result. Carries enough
// fields to feed the V1b Test endpoint and V1c dashboard widget +
// Prometheus exporter without a second probe.
type DuplicacyState struct {
	// ReasonCode is exactly one DuplicacyReason value (above). The
	// dashboard switches on this to decide severity; the exporter
	// emits it as a {reason="…"} label on a per-entry status gauge.
	ReasonCode DuplicacyReason

	// SnapshotCount is the total number of distinct snapshot revision
	// files discovered across all snapshot IDs in the repo. Zero
	// when ReasonCode != ok and ReasonCode != stale (the two states
	// where snapshots demonstrably exist).
	SnapshotCount int

	// LatestBackupAt is the end_time (or start_time fallback) of the
	// newest snapshot found, parsed from the snapshot file's JSON.
	// Zero time when no snapshots were found.
	LatestBackupAt time.Time

	// LatestBackupSizeBytes is the file_size field from the newest
	// snapshot. Zero when no snapshots were found.
	LatestBackupSizeBytes int64

	// LatestBackupFiles is the number_of_files field from the newest
	// snapshot. Zero when no snapshots were found.
	LatestBackupFiles int64

	// CurrentlyRunning is the orthogonal aux flag set when a lock
	// or incomplete-snapshot marker is detected on disk. Best-
	// effort — Duplicacy's lock semantics aren't fully stable
	// across versions, so false-negatives are tracked as a known
	// limitation (PRD #310 user story 13). False-positives are
	// acceptable; users see "currently running" on a stuck-lock
	// repo and investigate.
	CurrentlyRunning bool

	// LatestSnapshotID is the snapshot id (per-repo identifier; e.g.
	// "documents", "media") that produced LatestBackupAt. Empty
	// when no snapshots were found. Useful to V1c for rendering
	// "documents @ rev 87" on the dashboard.
	LatestSnapshotID string

	// LatestSnapshotRevision is the integer revision number of the
	// newest snapshot, parsed from its filename. Zero when no
	// snapshots were found. Useful to V1b/V1c for the same reason
	// as LatestSnapshotID.
	LatestSnapshotRevision int

	// SnapshotIDs is the sorted, deduped list of distinct snapshot
	// ids discovered in the repo. V1c dashboard renders aggregate-
	// only but per-id drill-down is anticipated; capturing the IDs
	// here means V1b/V1c don't need to re-walk the cache. Empty
	// when no snapshots were found.
	SnapshotIDs []string
}

// NewDiskDuplicacyRunner returns the production DuplicacyRunner. The
// production runner reads files directly from the filesystem with no
// I/O bypass — callers usually inject this at server wiring time so
// tests can substitute a fake.
func NewDiskDuplicacyRunner() DuplicacyRunner { return &diskDuplicacyRunner{} }

// diskDuplicacyRunner is the production DuplicacyRunner. Reads from
// the OS filesystem via os.* + encoding/json. Stateless; safe to
// share across goroutines.
//
// nowFn is the clock used for staleness comparisons. Defaults to
// time.Now in production; tests can construct a runner with a fixed
// clock so verbatim fixtures (with real captured Unix timestamps)
// don't drift into the "stale" bucket as wall-clock time advances
// after the fixtures were committed. Use newDiskDuplicacyRunnerAt
// in tests to pin a deterministic "now".
type diskDuplicacyRunner struct {
	nowFn func() time.Time
}

// newDiskDuplicacyRunnerAt is the test-only constructor that pins
// the runner's clock to a fixed time. Lets fixture-based tests use
// realistic-looking captured Unix timestamps without committing
// timestamps that age into staleness.
func newDiskDuplicacyRunnerAt(now time.Time) *diskDuplicacyRunner {
	return &diskDuplicacyRunner{nowFn: func() time.Time { return now }}
}

// now returns the runner's current time. Falls back to time.Now
// when nowFn is nil so the production constructor stays trivial.
func (r *diskDuplicacyRunner) now() time.Time {
	if r.nowFn == nil {
		return time.Now()
	}
	return r.nowFn()
}

// Read implements the DuplicacyRunner interface. Total function —
// every classifier outcome is encoded as ReasonCode rather than
// returned as a Go error. The error return is reserved for genuinely
// unexpected faults (e.g. ctx cancellation surfaced from a future
// deeper IO path); the V1a runner is synchronous and never returns a
// non-nil error today, but the signature accommodates V1c streaming
// extensions without breaking callers.
func (r *diskDuplicacyRunner) Read(ctx context.Context, entry DuplicacyEntry) (DuplicacyState, error) {
	// Universal preconditions: Path must exist and be readable.
	// These checks come first so a typo'd Path produces a clear
	// reason regardless of Kind.
	if reason, ok := classifyPathPrecondition(entry.Path); !ok {
		return DuplicacyState{ReasonCode: reason}, nil
	}

	switch entry.Kind {
	case DuplicacyKindCLIRepo:
		return r.readCLIRepo(entry)
	case DuplicacyKindWebCache:
		return r.readWebCache(entry)
	default:
		// Unknown kind — best-effort. Treat as not-a-repo so the
		// user sees the entry as misconfigured. V1b's form
		// validation rejects unknown kinds before reaching the
		// runner, so this branch is defense-in-depth.
		return DuplicacyState{ReasonCode: DuplicacyReasonNotARepo}, nil
	}
}

// readCLIRepo handles kind=cli-repo. Layout:
//
//	<Path>/
//	  .duplicacy/
//	    preferences          (JSON array of storage configs)
//	    cache/
//	      <storage_name>/
//	        snapshots/
//	          <snapshot_id>/
//	            <revision>   (JSON snapshot file)
func (r *diskDuplicacyRunner) readCLIRepo(entry DuplicacyEntry) (DuplicacyState, error) {
	dotDup := filepath.Join(entry.Path, ".duplicacy")
	st, err := os.Stat(dotDup)
	if err != nil {
		// .duplicacy/ missing → path is not a duplicacy repo. We
		// already verified Path itself exists in the precondition
		// check, so a missing .duplicacy dir is the canonical
		// "configured directory exists but isn't a repo" signal.
		if errors.Is(err, fs.ErrNotExist) {
			return DuplicacyState{ReasonCode: DuplicacyReasonNotARepo}, nil
		}
		return DuplicacyState{ReasonCode: DuplicacyReasonPathUnreadable}, nil
	}
	if !st.IsDir() {
		return DuplicacyState{ReasonCode: DuplicacyReasonNotARepo}, nil
	}

	storageName := readCLIRepoStorageName(filepath.Join(dotDup, "preferences"))
	snapshotsDir := filepath.Join(dotDup, "cache", storageName, "snapshots")

	state := r.walkSnapshotsDir(snapshotsDir, entry)
	state.CurrentlyRunning = detectCLIRepoRunning(dotDup, storageName)
	return state, nil
}

// readWebCache handles kind=web-cache. Layout (saspus/duplicacy-web):
//
//	<Path>/
//	  <StorageID>/
//	    snapshots/
//	      <snapshot_id>/
//	        <revision>   (JSON snapshot file)
func (r *diskDuplicacyRunner) readWebCache(entry DuplicacyEntry) (DuplicacyState, error) {
	storageDir := filepath.Join(entry.Path, entry.StorageID)
	st, err := os.Stat(storageDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DuplicacyState{ReasonCode: DuplicacyReasonStorageIDNotFound}, nil
		}
		return DuplicacyState{ReasonCode: DuplicacyReasonPathUnreadable}, nil
	}
	if !st.IsDir() {
		return DuplicacyState{ReasonCode: DuplicacyReasonStorageIDNotFound}, nil
	}

	snapshotsDir := filepath.Join(storageDir, "snapshots")
	state := r.walkSnapshotsDir(snapshotsDir, entry)
	state.CurrentlyRunning = detectWebCacheRunning(storageDir)
	return state, nil
}

// walkSnapshotsDir is the shared post-resolver step: given a path to
// `<...>/snapshots`, enumerate every <id>/<revision> file, parse
// each as a Duplicacy snapshot JSON, and produce the aggregate
// DuplicacyState. Reason-code derivation lives in
// classifySnapshotWalk for table-test friendliness.
func (r *diskDuplicacyRunner) walkSnapshotsDir(snapshotsDir string, entry DuplicacyEntry) DuplicacyState {
	staleDays := entry.StaleAfter
	if staleDays <= 0 {
		staleDays = DuplicacyDefaultStaleAfterDays
	}

	st, err := os.Stat(snapshotsDir)
	if err != nil || !st.IsDir() {
		// No snapshots dir at all — never-ran repo. Distinct from
		// "no snapshots inside the dir" which is also no_snapshots_yet
		// but goes through the iteration path.
		return DuplicacyState{ReasonCode: DuplicacyReasonNoSnapshotsYet}
	}

	idEntries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return DuplicacyState{ReasonCode: DuplicacyReasonPathUnreadable}
	}

	walk := snapshotWalkResult{}
	idSet := make(map[string]struct{})
	for _, ide := range idEntries {
		if !ide.IsDir() {
			continue
		}
		id := ide.Name()
		idDir := filepath.Join(snapshotsDir, id)
		revs, err := os.ReadDir(idDir)
		if err != nil {
			continue
		}
		idSet[id] = struct{}{}
		for _, rev := range revs {
			if rev.IsDir() {
				continue
			}
			revNum, ok := parseRevisionFromName(rev.Name())
			if !ok {
				continue
			}
			revPath := filepath.Join(idDir, rev.Name())
			snap, perr := parseDuplicacySnapshotFile(revPath)
			if perr != nil {
				walk.corrupt = true
				continue
			}
			walk.count++
			endTime := snap.endTime()
			if walk.latest.IsZero() || endTime.After(walk.latest) {
				walk.latest = endTime
				walk.latestSizeBytes = snap.FileSize
				walk.latestFiles = snap.NumberOfFiles
				walk.latestID = id
				walk.latestRev = revNum
			}
		}
	}

	walk.ids = make([]string, 0, len(idSet))
	for id := range idSet {
		walk.ids = append(walk.ids, id)
	}
	sort.Strings(walk.ids)

	return classifySnapshotWalkAt(walk, staleDays, r.now())
}

// snapshotWalkResult is the aggregate built up by walkSnapshotsDir
// before classification. Pulled out so classifySnapshotWalk can be
// table-tested as a pure function.
type snapshotWalkResult struct {
	count           int
	corrupt         bool
	latest          time.Time
	latestSizeBytes int64
	latestFiles     int64
	latestID        string
	latestRev       int
	ids             []string
}

// classifySnapshotWalkAt is the pure-function reason classifier —
// the table-test centrepiece. Encodes the precedence between the
// reason codes that emerge after the path-resolver has succeeded:
//
//   - corrupt always wins over any other count-based outcome (a
//     partially-damaged repo should surface the damage signal even
//     if some snapshots are still parseable; safer for the user).
//   - count==0 → no_snapshots_yet (configured but never ran, or all
//     files were unparseable).
//   - latest older than staleDays → stale.
//   - otherwise → ok.
//
// now is parameterised so tests pass deterministic clocks; the
// runner uses its own nowFn (defaulting to time.Now). path_*,
// not_a_repo and storage_id_not_found all short-circuit before this
// function — they emerge from the resolvers, not from the
// post-resolver classifier.
func classifySnapshotWalkAt(w snapshotWalkResult, staleDays int, now time.Time) DuplicacyState {
	state := DuplicacyState{
		SnapshotCount:          w.count,
		LatestBackupAt:         w.latest,
		LatestBackupSizeBytes:  w.latestSizeBytes,
		LatestBackupFiles:      w.latestFiles,
		LatestSnapshotID:       w.latestID,
		LatestSnapshotRevision: w.latestRev,
		SnapshotIDs:            w.ids,
	}
	if w.corrupt {
		state.ReasonCode = DuplicacyReasonCorruptSnapshot
		return state
	}
	if w.count == 0 || w.latest.IsZero() {
		state.ReasonCode = DuplicacyReasonNoSnapshotsYet
		return state
	}
	if staleDays > 0 && now.Sub(w.latest) > time.Duration(staleDays)*24*time.Hour {
		state.ReasonCode = DuplicacyReasonStale
		return state
	}
	state.ReasonCode = DuplicacyReasonOK
	return state
}

// classifyPathPrecondition returns the ReasonCode for a Path that
// fails its universal precondition check (existence + readability),
// plus a bool that's false when the precondition fails. Returns
// ("", true) when the path passes — caller proceeds with Kind-
// specific resolution.
func classifyPathPrecondition(path string) (DuplicacyReason, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		// Empty path is treated as "not found" — safer to surface
		// the misconfiguration than silently treat as no-op.
		return DuplicacyReasonPathNotFound, false
	}
	st, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DuplicacyReasonPathNotFound, false
		}
		// Anything else (EACCES, EIO, ENOTDIR mid-path, etc.) is
		// path_unreadable. We could try to differentiate further
		// but the actionable user-facing outcome is identical.
		return DuplicacyReasonPathUnreadable, false
	}
	if !st.IsDir() {
		// Path resolves but isn't a directory — treat as
		// not-a-repo for cli-repo. Web-cache happens to take the
		// same branch since the StorageID subdir lookup will then
		// fail too. Returning path_unreadable here keeps the
		// universal precondition uniform; the caller's Kind-
		// specific code distinguishes further if needed.
		return DuplicacyReasonPathUnreadable, false
	}
	return "", true
}

// readCLIRepoStorageName reads `<repo>/.duplicacy/preferences`
// (Duplicacy 3.x stores it as a plain JSON array) and returns the
// first storage entry's name. Returns "default" on any read or parse
// error so a malformed/missing preferences file falls back to the
// most common storage name and the runner keeps making progress —
// the snapshots-dir walk will then surface no_snapshots_yet if the
// directory really doesn't exist, which is the right outcome.
func readCLIRepoStorageName(prefsPath string) string {
	const fallback = "default"
	raw, err := os.ReadFile(prefsPath)
	if err != nil {
		return fallback
	}
	var prefs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return fallback
	}
	for _, p := range prefs {
		if name := strings.TrimSpace(p.Name); name != "" {
			return name
		}
	}
	return fallback
}

// duplicacySnapshotJSON mirrors the subset of Duplicacy's
// CreateSnapshotFromDescription payload that the runner consumes.
// Duplicacy stamps the snapshot file with start_time + end_time as
// Unix seconds (int64). Pulled into its own struct so verbatim
// fixture tests pin the schema.
type duplicacySnapshotJSON struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Revision      int    `json:"revision"`
	StartTime     int64  `json:"start_time"`
	EndTime       int64  `json:"end_time"`
	FileSize      int64  `json:"file_size"`
	NumberOfFiles int64  `json:"number_of_files"`
	Tag           string `json:"tag,omitempty"`
	Options       string `json:"options,omitempty"`
}

// endTime returns EndTime when set, falling back to StartTime when
// EndTime is zero (a backup that started but never finished writes
// EndTime=0). Zero return when both are unset.
func (s duplicacySnapshotJSON) endTime() time.Time {
	if s.EndTime > 0 {
		return time.Unix(s.EndTime, 0).UTC()
	}
	if s.StartTime > 0 {
		return time.Unix(s.StartTime, 0).UTC()
	}
	return time.Time{}
}

// parseDuplicacySnapshotFile reads one snapshot file and returns the
// decoded subset. Errors propagate to walkSnapshotsDir which records
// the corrupt flag — partial-damage in a repo surfaces as
// corrupt_snapshot rather than masking the issue.
func parseDuplicacySnapshotFile(path string) (duplicacySnapshotJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return duplicacySnapshotJSON{}, err
	}
	var snap duplicacySnapshotJSON
	if err := json.Unmarshal(raw, &snap); err != nil {
		return duplicacySnapshotJSON{}, err
	}
	return snap, nil
}

// parseRevisionFromName parses a revision number from a snapshot
// filename. Duplicacy names revision files as decimal integers —
// "1", "2", "87". We accept a leading "v" or zero-padded form
// defensively (older versions occasionally varied).
func parseRevisionFromName(name string) (int, bool) {
	s := strings.TrimSpace(name)
	if s == "" {
		return 0, false
	}
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimLeft(s, "0")
	if s == "" {
		// All-zero name like "00" or "0" — treat as revision 0 so
		// fixture authors can include it without surprise.
		return 0, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// detectCLIRepoRunning is the best-effort lock-detection heuristic
// for kind=cli-repo. Returns true when ANY of these markers is
// present:
//
//   - <dotDup>/locks/ directory exists and contains at least one
//     non-empty entry (Duplicacy creates per-storage lock files
//     under this path during operations on some versions);
//   - <dotDup>/cache/<storage>/incomplete file exists (in-progress
//     marker written by the create command).
//
// False-negatives are acceptable per PRD #310 user story 13 (the
// flag is best-effort and tracked as a known limitation if it
// doesn't fire on a particular Duplicacy version). False-positives
// are also acceptable — a stuck-lock from a crashed previous run
// surfaces as "currently running" and prompts the user to
// investigate, which is the right outcome.
func detectCLIRepoRunning(dotDup, storageName string) bool {
	if hasNonEmptyDir(filepath.Join(dotDup, "locks")) {
		return true
	}
	if pathExists(filepath.Join(dotDup, "cache", storageName, "incomplete")) {
		return true
	}
	return false
}

// detectWebCacheRunning is the kind=web-cache analogue. The web
// container writes its incomplete marker directly under the
// per-storage subdir.
func detectWebCacheRunning(storageDir string) bool {
	if pathExists(filepath.Join(storageDir, "incomplete")) {
		return true
	}
	if hasNonEmptyDir(filepath.Join(storageDir, "locks")) {
		return true
	}
	return false
}

// hasNonEmptyDir is true when path is a directory containing at
// least one entry. Returns false on any stat/read error so a
// permission issue doesn't trigger a false-positive
// CurrentlyRunning flag.
func hasNonEmptyDir(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// pathExists is true when path stat()s successfully.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
