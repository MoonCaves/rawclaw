#!/bin/sh
# scripts/lrvl-merge.sh — Lock-Rebase-Verify-Land (LRVL) Atomic Merge Engine
# Part of RFC-001: Sovereign Game-Theoretic Autonomous Auto-Merge Protocol
# Executed by am-orch (The Referee) upon unanimous consensus.
set -eu

if [ $# -lt 1 ]; then
  echo "Usage: $0 <candidate-branch>"
  exit 1
fi

BRANCH="$1"
REPO_DIR="/Users/jay-m4/code/rawclaw"
LOCK_FILE="$REPO_DIR/.git/merge.lock"

echo "==> [LRVL 1/6] Acquiring exclusive repository merge lock..."
# Portable POSIX/macOS atomic flock via Python fcntl
python3 - <<EOF
import fcntl, sys, time
lock_file = open("$LOCK_FILE", "w")
try:
    fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
    print("Lock acquired successfully.")
except BlockingIOError:
    print("ERROR: Merge lock currently held by another orchestrator process.", file=sys.stderr)
    sys.exit(1)
EOF

cleanup() {
  echo "==> [LRVL Cleanup] Releasing merge lock..."
  rm -f "$LOCK_FILE"
}
trap cleanup EXIT

echo "==> [LRVL 2/6] Fetching latest main and rebasing candidate branch..."
git -C "$REPO_DIR" fetch origin main
git -C "$REPO_DIR" checkout "$BRANCH"
git -C "$REPO_DIR" rebase origin/main

echo "==> [LRVL 3/6] Executing Deterministic Harness Verification Gate..."
"$REPO_DIR/scripts/harness-gate.sh"

echo "==> [LRVL 4/6] Executing Fast-Forward Merge into main..."
git -C "$REPO_DIR" checkout main
git -C "$REPO_DIR" merge --ff-only "$BRANCH"
git -C "$REPO_DIR" push origin main

echo "==> [LRVL 5/6] Rebuilding sovereign binary to ~/.local/bin/rawclaw..."
CGO_ENABLED=0 go build -o ~/.local/bin/rawclaw "$REPO_DIR/cmd/rawclaw"

echo "==> [LRVL 6/6] Auto-merge complete. Binary active at ~/.local/bin/rawclaw."
echo "===> CANDIDATE BRANCH $BRANCH AUTONOMOUSLY LANDED."
exit 0
