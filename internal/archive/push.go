package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// maxPushAttempts bounds the push → pull-rebase → push loop. Machine dirs are
// disjoint by construction, so the only contention is ref-level; a handful of
// rounds outlasts any realistic burst of concurrent pushers.
const maxPushAttempts = 5

// PushLocal copies this machine's transcript trees into the clone, commits,
// and pushes (pull --rebase + push, bounded retries). Idempotent; safe
// mid-session (transcripts are append-only, so a half-written file in the
// archive is valid and superseded by the next push). Returns a report for
// status output and logging.
func (a *Archive) PushLocal(ctx context.Context) (PushReport, error) {
	var rep PushReport
	// Single-writer gate: pushes from two local processes (a timer firing over
	// a manual push, overlapping background syncs) would otherwise race over
	// the same clone and its .git/rawclaw-tmp staging dir. ErrBusy is the
	// clean "someone else is syncing" signal, not a failure.
	release, err := acquireSyncLock(ctx)
	if err != nil {
		return rep, err
	}
	defer release()
	if err := a.ensureClone(ctx); err != nil {
		return rep, err
	}
	if err := a.ensureRegistered(); err != nil {
		return rep, err
	}

	tombs, err := lifecycle.LoadTombstones("")
	if err != nil {
		return rep, fmt.Errorf("read delete tombstones: %w", err)
	}

	copied, err := a.syncTrees(ctx, tombs)
	if err != nil {
		return rep, err
	}
	rep.Copied = copied

	removed, err := a.removeTombstoned(ctx, tombs)
	if err != nil {
		return rep, err
	}
	rep.Removed = removed

	changed, err := a.stageMachineDir(ctx)
	if err != nil {
		return rep, err
	}
	if !changed {
		// Nothing newly staged — but a previous run may have committed and then
		// failed (or been killed) before its push landed. Without this check, a
		// machine that stops producing new transcripts would strand that commit
		// in the clone forever.
		ahead, err := a.aheadOfRemote(ctx)
		if err != nil {
			return rep, err
		}
		if !ahead {
			// Truly up to date: no commit, no push — but still a SUCCESSFUL
			// sync, so the stamp advances. Without this, an idle-but-healthy
			// machine's own sync record ages into "overdue" for lack of new
			// transcripts, indistinguishable from a dead sync loop.
			stampPush()
			return rep, nil
		}
		retries, err := a.pushWithRetry(ctx)
		rep.Retries = retries
		if err != nil {
			return rep, err
		}
		rep.Pushed = true
		stampPush()
		return rep, nil
	}
	if err := a.commit(ctx, fmt.Sprintf("%s: sync transcripts", a.cfg.Name)); err != nil {
		return rep, err
	}
	rep.Committed = true

	retries, err := a.pushWithRetry(ctx)
	rep.Retries = retries
	if err != nil {
		return rep, err
	}
	rep.Pushed = true
	stampPush()
	return rep, nil
}

// machineDir is this machine's top-level dir inside the clone.
func (a *Archive) machineDir() string {
	return filepath.Join(a.clone, a.cfg.Name)
}

// cloneSentinel is the completed-clone marker, written under .git (never
// staged) immediately after `git clone` succeeds. A `.git` dir WITHOUT the
// sentinel is SUSPECT: git sets up `.git` and the origin remote early, so a
// clone killed mid-creation passes every cheap "is this a repo" probe forever
// while missing objects or a checkout. But absence alone is not torn-state
// evidence (a pre-sentinel-era clone has no marker either): ensureClone
// adopts a sentinel-less clone that proves structurally complete, and only
// rebuilds one that both fails the proof AND holds no unpushed commits.
const cloneSentinel = "rawclaw-clone-ok"

// lockGracePeriod ages out `.git/index.lock` left behind when git itself dies
// holding it (process-group SIGKILL, power loss). Younger locks are honored —
// a concurrent healthy push may own them; no git operation on this small
// clone legitimately runs this long.
const lockGracePeriod = 15 * time.Minute

// ensureClone guarantees a usable local clone. An existing COMPLETED one
// (sentinel present) is kept: it must point at the configured remote (a
// silently-edited config must not keep pushing to the old remote), stale
// operation state left by a kill (mid-rebase/mid-merge markers, an aged
// index.lock) is cleared, and HEAD must resolve to a branch afterwards. A
// sentinel-LESS clone that proves structurally complete (matching origin,
// branch HEAD resolving to a commit) is a pre-sentinel-era clone — adopted in
// place by stamping the sentinel, never wiped. Rebuild (wipe + re-clone)
// happens only on POSITIVE evidence of a torn or wedged clone — a
// sentinel-less .git whose checkout never completed (killed mid-clone) or a
// detached/unreadable HEAD that recovery could not reattach — AND only after
// the stranded-work guard proves the clone holds no commit the remote lacks.
// Transient failures (unreadable config, exec errors, a dying ctx) surface as
// hard errors instead: a rebuild discards any locally-committed-but-unpushed
// sync, so destruction needs evidence, not absence of proof.
func (a *Archive) ensureClone(ctx context.Context) error {
	gitDir := filepath.Join(a.clone, ".git")
	_, gitErr := os.Stat(gitDir)
	_, okErr := os.Stat(filepath.Join(gitDir, cloneSentinel))
	// A stat failure that is NOT "does not exist" (EACCES, I/O error) is
	// environmental, not torn-state evidence — fail loudly rather than let it
	// route into the rebuild path and destroy a possibly-healthy clone.
	if gitErr != nil && !os.IsNotExist(gitErr) {
		return fmt.Errorf("stat clone git dir: %w", gitErr)
	}
	if okErr != nil && !os.IsNotExist(okErr) {
		return fmt.Errorf("stat clone sentinel: %w", okErr)
	}
	if gitErr == nil {
		completed := okErr == nil
		healthy, err := a.recoverExistingClone(ctx, gitDir, completed)
		if err != nil {
			return err
		}
		if healthy {
			if !completed {
				// A structurally complete pre-sentinel clone: adopt it by
				// stamping the marker instead of wiping a healthy repo (and
				// any unpushed sync commit inside it).
				if werr := os.WriteFile(filepath.Join(gitDir, cloneSentinel), nil, 0o644); werr != nil {
					return fmt.Errorf("mark adopted clone complete: %w", werr)
				}
			}
			return nil
		}
		// Rebuild candidate. Two fail-closed guards before any RemoveAll:
		// a dead caller ctx poisons every probe above, so its verdict is not
		// corruption evidence; and a clone holding commits the remote lacks
		// must never be destroyed, whatever the probes said.
		if cerr := ctx.Err(); cerr != nil {
			return fmt.Errorf("clone recovery interrupted: %w", cerr)
		}
		n, serr := a.strandedCommits(ctx)
		if serr != nil {
			return fmt.Errorf("verify clone holds no unpushed work (refusing to rebuild without proof): %w", serr)
		}
		if n > 0 {
			return fmt.Errorf(
				"local clone %s looks damaged but holds %d unpushed commit(s); refusing to rebuild — inspect it (git -C %s status), push or back up those commits, then delete the dir to re-clone",
				a.clone, n, a.clone)
		}
	}

	parent := filepath.Dir(a.clone)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create archive state dir: %w", err)
	}
	if err := os.RemoveAll(a.clone); err != nil {
		return fmt.Errorf("clear broken clone: %w", err)
	}
	if _, err := a.run(ctx, parent, "clone", a.cfg.Remote, a.clone); err != nil {
		return fmt.Errorf(
			"clone archive remote (the repository must already exist on your git host — create it, PRIVATE, then retry): %w",
			err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, cloneSentinel), nil, 0o644); err != nil {
		return fmt.Errorf("mark clone complete: %w", err)
	}
	return nil
}

// recoverExistingClone probes an existing clone (.git present) and reports
// whether it is usable, after running stale-operation recovery. completed
// says the sentinel was present. Usable for a COMPLETED clone: matching
// origin + HEAD naming a branch after recovery (an unborn HEAD on an
// empty-remote clone qualifies). A sentinel-LESS clone must additionally
// prove its checkout finished — HEAD resolving to a commit — because a clone
// killed mid-`git clone` passes every cheaper probe (git writes .git and the
// origin remote early) while a completed pre-sentinel clone always has a
// resolvable HEAD; that resolution is the positive completed-clone evidence
// the missing marker cannot give. Probe failures on a COMPLETED clone are
// environmental → hard error (the sentinel says the clone finished; failure
// is not evidence for destroying it); on a sentinel-less clone they mean
// "cannot prove complete" → not usable, and the caller's stranded-work guard
// decides whether a rebuild is safe.
func (a *Archive) recoverExistingClone(ctx context.Context, gitDir string, completed bool) (bool, error) {
	out, err := a.run(ctx, a.clone, "remote", "get-url", "origin")
	if err != nil {
		if completed {
			return false, fmt.Errorf("read clone remote: %w", err)
		}
		return false, nil // origin unreadable: not provably complete
	}
	if got := strings.TrimSpace(out); got != a.cfg.Remote {
		return false, fmt.Errorf(
			"local clone %s points at %s but the config says %s; delete the clone dir to re-clone",
			a.clone, got, a.cfg.Remote)
	}
	a.recoverStaleOps()
	if _, err := a.run(ctx, a.clone, "symbolic-ref", "--short", "HEAD"); err != nil {
		if headAttached(gitDir) {
			// The HEAD file itself still names a branch, so the symbolic-ref
			// failure was environmental — same posture as get-url above.
			return false, fmt.Errorf("resolve clone HEAD: %w", err)
		}
		// Positive wedge evidence: HEAD detached (or unreadable) even after
		// recovery.
		return false, nil
	}
	if completed {
		return true, nil // healthy (or recovered) clone
	}
	// Legacy adoption demands positive proof the original clone FINISHED —
	// two probes, because `git clone` writes refs and HEAD before the
	// checkout: HEAD resolving to a commit rules out a kill mid-fetch, and a
	// clean `status` (index and worktree exactly matching HEAD) rules out a
	// kill mid-CHECKOUT. A checkout-killed clone has a resolvable HEAD but no
	// (complete) index; adopting it would let the next push's `add -A` build
	// a near-empty index and commit a tree that drops every other machine's
	// dir from the archive tip. Any probe failure or any dirty output means
	// "cannot prove complete" — the caller's stranded-work guard then decides
	// whether the rebuild is safe.
	if _, err := a.run(ctx, a.clone, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return false, nil // unborn/unresolvable HEAD: the torn mid-fetch shape
	}
	if st, err := a.run(ctx, a.clone, "status", "--porcelain"); err != nil || strings.TrimSpace(st) != "" {
		return false, nil // dirty or unprovable: the torn mid-checkout shape
	}
	return true, nil // structurally complete legacy clone — adoptable
}

// strandedCommits counts commits reachable from any local ref or HEAD but
// from no remote-tracking ref — exactly the work a wipe-and-reclone would
// destroy. Zero is the only answer that licenses a rebuild; an error means
// safety could not be proven and the caller must refuse to wipe.
func (a *Archive) strandedCommits(ctx context.Context) (int, error) {
	out, err := a.run(ctx, a.clone, "rev-list", "--count", "--all", "--not", "--remotes")
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if perr != nil {
		return 0, fmt.Errorf("parse stranded commit count %q: %w", out, perr)
	}
	return n, nil
}

// headAttached reports whether the clone's HEAD file names a branch ("ref: "
// symref). Read directly from disk so a git that cannot even exec is not
// mistaken for a detached HEAD. Unreadable/raw-SHA HEAD = not attached.
func headAttached(gitDir string) bool {
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(b)), "ref: ")
}

// recoveryTimeout bounds the fresh-context git operations recovery performs
// (rebase/merge aborts). Generous for a small clone, but a hard ceiling — a
// hung git during recovery must not hang the verb forever.
const recoveryTimeout = 30 * time.Second

// recoveryContext is the context recovery-critical git operations run on: a
// FRESH bounded context, never the caller's. Recovery fires exactly when a
// previous run died mid-operation — often killed BY the same watchdog whose
// ctx the current caller carries — so a cancelled caller ctx would skip the
// abort, leave HEAD detached, and let the failed probe read as unrecoverable
// corruption (destroying an unpushed commit on rebuild). The pull/push abort
// paths use the same shape for the same reason.
func recoveryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), recoveryTimeout)
}

// recoverStaleOps clears operation state a kill can leave in the clone:
// a mid-rebase marker (the process, or rawclaw's own watchdog, dying between
// `pull --rebase` starting and its abort handler running — a wedged rebase
// detaches HEAD, which breaks currentBranch and with it every later push AND
// pull), a mid-merge marker (belt-and-suspenders; our verbs only rebase), and
// an index.lock older than lockGracePeriod (git itself died holding it).
// Best-effort: ensureClone's HEAD check decides whether recovery sufficed.
// Runs on recoveryContext, not the caller's ctx — see recoveryContext.
//
// Markers are aborted without an age gate, deliberately: the machine-wide
// sync flock serializes live rawclaw writers, so by the time we run, marker
// state can only belong to a DEAD holder — or to its orphaned git child,
// which was already ctx-cancelled (SIGTERM) as its parent died and is at
// worst finishing its last writes. Aborting under that race converges: the
// abort or the orphan wins, and either end state is one the HEAD check plus
// the next run recover from (interleave-tested).
func (a *Archive) recoverStaleOps() {
	rctx, cancel := recoveryContext()
	defer cancel()
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(a.clone, ".git", marker)); err == nil {
			_, _ = a.run(rctx, a.clone, "rebase", "--abort")
			break
		}
	}
	if _, err := os.Stat(filepath.Join(a.clone, ".git", "MERGE_HEAD")); err == nil {
		_, _ = a.run(rctx, a.clone, "merge", "--abort")
	}
	lock := filepath.Join(a.clone, ".git", "index.lock")
	if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > lockGracePeriod {
		_ = os.Remove(lock)
	}
}

// syncTrees copies changed transcript files from every source tree into the
// machine dir, preserving relative paths (<machine>/<source>/<rel>), and
// returns how many files were copied. Files are only ever added or updated —
// the archive never prunes, and foreign machine dirs are never touched.
// Tombstoned sessions are skipped at the copy: a deleted session whose local
// file survives (restored backup, delete race) must neither resurrect in the
// archive nor be re-copied-and-re-removed on every later push.
func (a *Archive) syncTrees(ctx context.Context, tombs map[string]struct{}) (int, error) {
	// Copies stage through a temp dir under .git: same filesystem (rename
	// stays atomic) but invisible to `git add`, so a temp file orphaned by a
	// kill can never be committed to the archive. Stale orphans are cleared.
	tmpDir := filepath.Join(a.clone, ".git", "rawclaw-tmp")
	if err := os.RemoveAll(tmpDir); err != nil {
		return 0, fmt.Errorf("clear copy temp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return 0, fmt.Errorf("create copy temp dir: %w", err)
	}

	copied := 0
	for _, tree := range sourceTrees() {
		if tree.root == "" || !isDir(tree.root) {
			continue // absent runtime: nothing to push
		}
		skip := tombstonedSources(tree, tombs)
		destRoot := filepath.Join(a.machineDir(), tree.id)
		for _, src := range paths.ContainedJSONL(tree.root) {
			if err := ctx.Err(); err != nil {
				return copied, err // honor cancellation between files
			}
			if _, dead := skip[src]; dead {
				continue // explicitly deleted: never (re)enters the archive
			}
			rel, err := filepath.Rel(tree.root, src)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue // outside the tree (shouldn't happen post-containment)
			}
			info, err := os.Stat(src)
			if err != nil {
				continue // vanished mid-walk: the next push catches it
			}
			dst := filepath.Join(destRoot, rel)
			if !needsCopy(src, info, dst) {
				continue
			}
			if err := copyFile(src, dst, tmpDir, info); err != nil {
				return copied, fmt.Errorf("copy %s: %w", rel, err)
			}
			copied++
		}
	}
	return copied, nil
}

// needsCopy is the rsync-style quick check deciding whether src must be
// (re)copied over dst: missing dst or a size change always copies (the
// append-only common case); equal size + equal mtime skips (copyFile mirrors
// the source mtime onto the dst, so this fast path holds across pushes);
// equal size but different mtime falls back to the head+tail content
// fingerprint — catching in-place rewrites that touch either end while
// skipping touch-only mtime churn. (A same-size rewrite confined to the
// fingerprint's blind middle region is not detected; for append-only
// transcripts that shape does not occur.)
func needsCopy(src string, srcInfo os.FileInfo, dst string) bool {
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return true
	}
	if srcInfo.Size() != dstInfo.Size() {
		return true
	}
	if srcInfo.ModTime().Equal(dstInfo.ModTime()) {
		return false
	}
	srcFP := provenance.FileFingerprint(src, srcInfo.Size())
	dstFP := provenance.FileFingerprint(dst, dstInfo.Size())
	if srcFP == "" || dstFP == "" {
		return true // unreadable side: copy rather than silently skip
	}
	return srcFP != dstFP
}

// copyFile streams src into dst via a temp file (in tmpDir, outside the
// tracked tree) + rename — a kill mid-copy never leaves a torn or stray file
// where git could stage it — then mirrors the source mtime onto dst so the
// quick check's mtime fast path holds on the next push.
func copyFile(src, dst, tmpDir string, srcInfo os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(tmpDir, "copy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // best-effort cleanup; the copy error is the story
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	// Zero atime = leave unchanged; only the mtime is mirrored.
	return os.Chtimes(dst, time.Time{}, srcInfo.ModTime())
}

// stageMachineDir stages this machine's dir and reports whether anything is
// actually staged — the no-changes signal that suppresses empty commits.
func (a *Archive) stageMachineDir(ctx context.Context) (bool, error) {
	if _, err := a.run(ctx, a.clone, "add", "-A", "--", a.cfg.Name); err != nil {
		return false, fmt.Errorf("stage machine dir: %w", err)
	}
	out, err := a.run(ctx, a.clone, "status", "--porcelain", "--", a.cfg.Name)
	if err != nil {
		return false, fmt.Errorf("check staged changes: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// withCommitIdentity prefixes args with the pinned synthetic identity for any
// git child that can CREATE commits: `commit` itself, and `pull --rebase`,
// whose rebase RE-CREATES every replayed local commit with the current
// committer. Unpinned, either one dies exit-128 "Committer identity unknown"
// on a machine with no git identity configured — for the rebase, exactly on
// the concurrent-push retry and stranded-commit pull paths. Pinned rather
// than read from config, so the archive's synthetic authorship stays uniform
// and no machine needs git setup to sync.
func withCommitIdentity(args ...string) []string {
	return append([]string{
		"-c", "user.name=rawclaw",
		"-c", "user.email=rawclaw@localhost",
	}, args...)
}

// commit creates a commit with a pinned identity, so pushes work on machines
// with no global git identity configured.
func (a *Archive) commit(ctx context.Context, msg string) error {
	_, err := a.run(ctx, a.clone, withCommitIdentity("commit", "-m", msg)...)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// aheadOfRemote reports whether HEAD holds commits the remote-tracking branch
// lacks — the stranded-commit case: a prior run committed, then its push
// failed or was killed. An unborn HEAD is never ahead; a branch the remote
// doesn't know yet always is.
func (a *Archive) aheadOfRemote(ctx context.Context) (bool, error) {
	if _, err := a.run(ctx, a.clone, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return false, nil // unborn HEAD: nothing committed yet
	}
	branch, err := a.currentBranch(ctx)
	if err != nil {
		return false, err
	}
	if _, err := a.run(ctx, a.clone, "rev-parse", "--verify", "--quiet", "origin/"+branch); err != nil {
		return true, nil // remote branch unknown locally: our commits never landed
	}
	out, err := a.run(ctx, a.clone, "rev-list", "--count", "origin/"+branch+"..HEAD")
	if err != nil {
		return false, fmt.Errorf("count unpushed commits: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return false, fmt.Errorf("parse unpushed commit count %q: %w", out, err)
	}
	return n > 0, nil
}

// currentBranch resolves the checked-out branch (the remote's default branch
// after a clone; also set on an empty clone's unborn HEAD).
func (a *Archive) currentBranch(ctx context.Context) (string, error) {
	out, err := a.run(ctx, a.clone, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// pushWithRetry pushes the current branch, retrying on non-fast-forward
// rejection (a concurrent machine pushed first): each round rebases the local
// commits onto the updated remote ref — machine dirs are disjoint, so the
// rebase is content-conflict-free — and pushes again, bounded by
// maxPushAttempts. Never force-pushes. A failed rebase is aborted before
// returning, so the clone is never left wedged mid-rebase. Returns how many
// retry rounds were needed.
func (a *Archive) pushWithRetry(ctx context.Context) (int, error) {
	branch, err := a.currentBranch(ctx)
	if err != nil {
		return 0, err
	}
	for attempt := 1; attempt <= maxPushAttempts; attempt++ {
		out, err := a.run(ctx, a.clone, "push", "-u", "origin", branch)
		if err == nil {
			return attempt - 1, nil
		}
		if !isRejectedPush(out) {
			return attempt - 1, fmt.Errorf("push archive: %w", err)
		}
		if attempt == maxPushAttempts {
			break
		}
		if _, rerr := a.run(ctx, a.clone, withCommitIdentity("pull", "--rebase", "origin", branch)...); rerr != nil {
			// Abort on a FRESH bounded context: if the failure above was this
			// ctx being canceled, the same ctx could never start the abort —
			// and the whole point is never leaving a wedged clone.
			rctx, cancel := recoveryContext()
			_, _ = a.run(rctx, a.clone, "rebase", "--abort")
			cancel()
			return attempt, fmt.Errorf("rebase onto updated remote: %w", rerr)
		}
	}
	return maxPushAttempts - 1, fmt.Errorf(
		"push rejected %d times in a row (remote %s is receiving concurrent pushes); try again shortly",
		maxPushAttempts, a.cfg.Remote)
}

// isRejectedPush classifies push output: true only for ref-contention
// rejections worth a rebase-retry; auth, network, and missing-remote failures
// return immediately instead of burning retries.
func isRejectedPush(out string) bool {
	for _, marker := range []string{"[rejected]", "non-fast-forward", "fetch first", "cannot lock ref"} {
		if strings.Contains(out, marker) {
			return true
		}
	}
	return false
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pushStampPath is <state-dir>/archive/last-push — the last-successful-push-
// SYNC record: it advances on every PushLocal that either pushed or verified
// the remote already has everything, so it means "the sync loop works", not
// "new content shipped". Like the pull stamp, its MTIME is the record (the
// body stays empty).
func pushStampPath() string {
	return filepath.Join(store.CacheDir(), "archive", "last-push")
}

// stampPush records a successful push sync by (re)writing the stamp file.
// Best-effort: a failed stamp only under-reports `archive status`.
func stampPush() {
	p := pushStampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, nil, 0o644)
}
