#!/usr/bin/env bash
# Review helper for Wave 2 PRs (#136, #137, #138, #139).
# Runs build + tests on each PR branch in isolated worktrees, so your
# main checkout stays untouched. ZERO production impact.
#
# Usage:
#   ./scripts/review-wave2.sh              # build+test all 4
#   ./scripts/review-wave2.sh 137          # just one PR
#   ./scripts/review-wave2.sh smoke 137    # build+test AND start the binary on :8061

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKTREE_ROOT="/tmp/nd-review"
mkdir -p "$WORKTREE_ROOT"

# POSIX-compatible lookup (macOS bash 3.2 has no associative arrays)
branch_for_pr() {
  case "$1" in
    136) echo "fix/128-unraid-port-variable" ;;
    137) echo "fix/126-api-key-secure-context" ;;
    138) echo "fix/131-cost-per-tb-settings-ui" ;;
    139) echo "fix/133-purge-orphaned-service-checks" ;;
    *) echo "" ;;
  esac
}
desc_for_pr() {
  case "$1" in
    136) echo "#128 Unraid port fix" ;;
    137) echo "#126 API key secure-context" ;;
    138) echo "#131 Cost per TB" ;;
    139) echo "#133 Orphan service checks" ;;
    *) echo "" ;;
  esac
}

run_for_pr() {
  local pr="$1"
  local smoke="${2:-0}"
  local branch
  branch="$(branch_for_pr "$pr")"
  local desc
  desc="$(desc_for_pr "$pr")"
  local wt="$WORKTREE_ROOT/pr-$pr"

  if [ -z "$branch" ]; then
    echo "Unknown PR: $pr"
    return 1
  fi

  echo
  echo "════════════════════════════════════════════════════════════════"
  echo "  PR #$pr — $desc"
  echo "  Branch: $branch"
  echo "  Worktree: $wt"
  echo "════════════════════════════════════════════════════════════════"

  if [ -d "$wt" ]; then
    git -C "$ROOT" worktree remove --force "$wt" 2>/dev/null || rm -rf "$wt"
  fi

  git -C "$ROOT" fetch origin "$branch"
  git -C "$ROOT" worktree add "$wt" "origin/$branch"

  echo "─── go build ────────────────────────────────────────────────────"
  if (cd "$wt" && go build ./...); then
    echo "✓ build OK"
  else
    echo "✗ BUILD FAILED"
    return 1
  fi

  echo "─── go test ─────────────────────────────────────────────────────"
  if (cd "$wt" && go test ./...); then
    echo "✓ tests OK"
  else
    echo "✗ TESTS FAILED"
    return 1
  fi

  if [ "$smoke" = "1" ]; then
    echo "─── Starting binary on :8061 (Ctrl+C to stop) ───────────────────"
    echo "Smoke tests to run in browser at http://localhost:8061 :"
    case "$pr" in
      137) echo "  • Settings → API Key → click Generate. Field should populate with 'nd-<32hex>'."
           echo "  • Click Copy. Should copy to clipboard (or toast a fallback)." ;;
      138) echo "  • Settings → find 'Drive Replacement Cost' card → enter 22.50 → Save."
           echo "  • Navigate to /replacement-planner → prompt to 'Set Cost per TB' should be gone." ;;
      136) echo "  • Start the binary with NAS_DOCTOR_LISTEN=8067 (bare number) instead of :8067."
           echo "  • Verify the binary binds :8067 — the normalizer should prepend the colon."
           echo "  • Unraid template change itself needs testing on an actual Unraid box." ;;
      139) echo "  • Settings → add a service check (e.g. http://example.com)."
           echo "  • Save. Visit /service-checks — should appear."
           echo "  • Settings → delete that check → Save."
           echo "  • Refresh /service-checks — should be GONE (not flagged orphan)." ;;
    esac
    echo
    (cd "$wt" && NAS_DOCTOR_LISTEN=:8061 go run ./cmd/nas-doctor)
  fi
}

cleanup() {
  echo
  echo "To list / remove review worktree(s):"
  echo "  git -C $ROOT worktree list"
  echo "  git -C $ROOT worktree remove <path>"
}
trap cleanup EXIT

if [ "${1:-}" = "smoke" ]; then
  [ -n "${2:-}" ] || { echo "Usage: $0 smoke <PR#>"; exit 1; }
  run_for_pr "$2" 1
elif [ -n "${1:-}" ]; then
  run_for_pr "$1" 0
else
  for pr in 137 139 138 136; do   # ordered: simplest first
    run_for_pr "$pr" 0 || echo "⚠ PR #$pr failed — continuing"
  done
fi
