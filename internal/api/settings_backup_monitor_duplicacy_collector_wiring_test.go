package api

import (
	"testing"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// TestApiDuplicacyEntriesToCollector_RoundTripsAllFields pins the
// api → collector field mapping so a future field rename surfaces
// at compile time + via test failure rather than silently dropping
// the value at the api boundary. Mirrors the pattern of
// TestApiBorgReposToCollector_RoundTripsAllFields. Issue #314.
func TestApiDuplicacyEntriesToCollector_RoundTripsAllFields(t *testing.T) {
	in := []DuplicacyEntry{
		{
			Enabled:    true,
			Label:      "  Documents  ", // trim
			Kind:       "cli-repo",
			Path:       "  /mnt/user/dup/docs  ",
			StorageID:  "",
			StaleAfter: 14,
		},
		{
			Enabled:    false,
			Label:      "Media",
			Kind:       "web-cache",
			Path:       "/mnt/user/duplicacy-web/cache/localhost/0/.duplicacy/cache",
			StorageID:  "media",
			StaleAfter: 30,
		},
	}
	got := apiDuplicacyEntriesToCollector(in)
	if len(got) != 2 {
		t.Fatalf("got %d entries; want 2", len(got))
	}
	want0 := collector.DuplicacyEntry{
		Enabled:    true,
		Label:      "Documents",
		Kind:       "cli-repo",
		Path:       "/mnt/user/dup/docs",
		StorageID:  "",
		StaleAfter: 14,
	}
	if got[0] != want0 {
		t.Errorf("entry 0 mismatch:\n got = %+v\nwant = %+v", got[0], want0)
	}
	want1 := collector.DuplicacyEntry{
		Enabled:    false,
		Label:      "Media",
		Kind:       "web-cache",
		Path:       "/mnt/user/duplicacy-web/cache/localhost/0/.duplicacy/cache",
		StorageID:  "media",
		StaleAfter: 30,
	}
	if got[1] != want1 {
		t.Errorf("entry 1 mismatch:\n got = %+v\nwant = %+v", got[1], want1)
	}
}

// TestApiDuplicacyEntriesToCollector_NilAndEmptyReturnNil asserts the
// nil-pass-through semantics required by Collector.SetBackupMonitorDuplicacy
// (which itself defensively-copies but defaults to clearing on nil).
func TestApiDuplicacyEntriesToCollector_NilAndEmptyReturnNil(t *testing.T) {
	if got := apiDuplicacyEntriesToCollector(nil); got != nil {
		t.Errorf("nil → got %+v; want nil", got)
	}
	if got := apiDuplicacyEntriesToCollector([]DuplicacyEntry{}); got != nil {
		t.Errorf("empty slice → got %+v; want nil (matches apiBorgReposToCollector pattern)", got)
	}
}
