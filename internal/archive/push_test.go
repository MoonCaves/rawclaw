package archive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTranscript writes a transcript file under root/<rel> and returns its path.
func writeTranscript(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// seedTranscripts creates one Claude and one Codex transcript under the
// isolated home and returns their absolute paths.
func seedTranscripts(t *testing.T, home string) (claudeFile, codexFile string) {
	t.Helper()
	claudeFile = writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-1111.jsonl", `{"type":"user","text":"hello"}`+"\n")
	codexFile = writeTranscript(t, filepath.Join(home, ".codex", "sessions"),
		"2026/07/rollout-abc.jsonl", `{"type":"session_meta"}`+"\n")
	return claudeFile, codexFile
}

// gitVerb returns the git subcommand in args — the first arg past the leading
// "-c key=val" config pairs withCommitIdentity prepends — so fake run seams
// classify invocations by verb regardless of identity pinning.
func gitVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++ // skip the -c value
			continue
		}
		return args[i]
	}
	return ""
}

// remoteCommitCount returns the number of commits on the remote's default branch.
func remoteCommitCount(t *testing.T, bare string) int {
	t.Helper()
	out := strings.TrimSpace(gitT(t, bare, "rev-list", "--count", "HEAD"))
	n := 0
	if _, err := fmt.Sscanf(out, "%d", &n); err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}

// TestPushLocal_TracerLandsBothTrees: init + push → both sources' transcript
// trees appear in the REMOTE under <machine>/<source>/..., relative paths
// preserved.
func TestPushLocal_TracerLandsBothTrees(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("PushLocal: %v", err)
	}
	if rep.Copied != 2 || !rep.Committed || !rep.Pushed {
		t.Errorf("report = %+v, want Copied=2 Committed Pushed", rep)
	}

	verify := checkoutRemote(t, bare)
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-1111.jsonl",
		"machine-a/codex/2026/07/rollout-abc.jsonl",
		"machine-a/.rawclaw-machine.json",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote missing %s: %v", want, err)
		}
	}
}

// TestPushLocal_NoChangesNoCommit: an unchanged tree pushes nothing — no copy,
// no commit, no new remote head.
func TestPushLocal_NoChangesNoCommit(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}
	before := remoteCommitCount(t, bare)

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if rep.Copied != 0 || rep.Committed || rep.Pushed {
		t.Errorf("report = %+v, want no-op", rep)
	}
	if after := remoteCommitCount(t, bare); after != before {
		t.Errorf("remote commits %d → %d, want unchanged", before, after)
	}
}

// TestPushLocal_AppendRecopiesOnlyThatFile: appending to one transcript
// re-copies exactly that file.
func TestPushLocal_AppendRecopiesOnlyThatFile(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	claudeFile, _ := seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	f, err := os.OpenFile(claudeFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"assistant","text":"more"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push after append: %v", err)
	}
	if rep.Copied != 1 {
		t.Errorf("Copied = %d, want 1 (only the appended file)", rep.Copied)
	}
	if !rep.Pushed {
		t.Error("appended file not pushed")
	}

	verify := checkoutRemote(t, bare)
	got, err := os.ReadFile(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-1111.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"more"`) {
		t.Error("remote copy missing the appended line")
	}
}

// injectConcurrentPusher wires a.run so that a second machine's head lands on
// the remote exactly between our commit and our first push — the ref-contention
// seam the rebase-retry path exists for.
func injectConcurrentPusher(t *testing.T, a *Archive, bare string) {
	t.Helper()
	// Machine B: a separate clone of the same remote, claiming its own dir.
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	gitT(t, "", "clone", bare, cloneB)
	pushB := func() {
		writeTranscript(t, cloneB, "machine-b/claude/-proj/sess-b.jsonl", "{}\n")
		if err := writeManifest(filepath.Join(cloneB, "machine-b"), manifest{
			MachineID: "beefbeefbeefbeefbeefbeefbeefbeef",
			Name:      "machine-b", Hostname: "b-host", UpdatedAt: "2026-01-01T00:00:00Z",
		}); err != nil {
			t.Error(err)
		}
		gitT(t, cloneB, "add", "-A")
		gitT(t, cloneB, "commit", "-m", "machine-b: sync transcripts")
		gitT(t, cloneB, "pull", "--rebase", "origin", "main")
		gitT(t, cloneB, "push", "origin", "HEAD")
	}

	// Inject machine B's push exactly between our commit and our first push.
	real := a.run
	injected := false
	a.run = func(ctx context.Context, dir string, args ...string) (string, error) {
		if !injected && len(args) > 0 && args[0] == "push" {
			injected = true
			pushB()
		}
		return real(ctx, dir, args...)
	}
}

// withoutGitIdentity makes every git child spawned under the test behave as on
// a fresh machine with NO usable committer identity: the global config is
// redirected to one that only disables ident auto-detection (plain absence is
// not enough — on hosts whose hostname canonicalizes, git silently invents
// user@host and would mask the bug), the system config is silenced, and every
// identity env var is cleared.
func withoutGitIdentity(t *testing.T) {
	t.Helper()
	gcfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gcfg, []byte("[user]\n\tuseConfigOnly = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gcfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for _, k := range []string{
		"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL",
	} {
		t.Setenv(k, "") // register restoration of any original value…
		os.Unsetenv(k)  // …then actually clear it for the child gits
	}
}

// TestPushLocal_ConcurrentPusherRebaseRetry: a second machine's head appears
// on the remote mid-push (after our commit, before our push) → the
// rebase-retry loop lands BOTH machines' trees.
func TestPushLocal_ConcurrentPusherRebaseRetry(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	injectConcurrentPusher(t, a, bare)

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("PushLocal with concurrent pusher: %v", err)
	}
	if !rep.Pushed {
		t.Error("push did not land")
	}
	if rep.Retries != 1 {
		t.Errorf("Retries = %d, want 1", rep.Retries)
	}

	verify := checkoutRemote(t, bare)
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-1111.jsonl",
		"machine-b/claude/-proj/sess-b.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote missing %s after concurrent push: %v", want, err)
		}
	}
}

// TestPushLocal_RebaseRetryWithoutGitIdentity: the concurrent-push retry path
// must succeed on a machine with NO git identity configured. commit() pins a
// synthetic identity, but the retry's `pull --rebase` RE-CREATES the replayed
// commit — unpinned, that child dies exit-128 "Committer identity unknown" on
// any fresh machine, and the sync never lands.
func TestPushLocal_RebaseRetryWithoutGitIdentity(t *testing.T) {
	home := newTestHome(t)
	withoutGitIdentity(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	injectConcurrentPusher(t, a, bare)

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("PushLocal on identity-less machine: %v", err)
	}
	if !rep.Pushed {
		t.Error("push did not land")
	}
	if rep.Retries != 1 {
		t.Errorf("Retries = %d, want 1", rep.Retries)
	}

	verify := checkoutRemote(t, bare)
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-1111.jsonl",
		"machine-b/claude/-proj/sess-b.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote missing %s after identity-less rebase retry: %v", want, err)
		}
	}
}

// TestPushWithRetry_Bounded: a remote that stays contended forever exhausts
// the bounded retries with a clear error — and the loop never force-pushes.
func TestPushWithRetry_Bounded(t *testing.T) {
	newTestHome(t)

	var calls [][]string
	a := &Archive{
		cfg:       Config{Remote: "example.invalid/archive.git", Name: "machine-a"},
		clone:     t.TempDir(),
		machineID: "cafecafecafecafecafecafecafecafe",
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			calls = append(calls, args)
			switch gitVerb(args) {
			case "symbolic-ref":
				return "main\n", nil
			case "push":
				return "! [rejected] main -> main (fetch first)", errors.New("exit status 1")
			case "pull":
				return "", nil // rebase "succeeds", remote is just faster than us
			default:
				return "", nil
			}
		},
	}

	retries, err := a.pushWithRetry(context.Background())
	if err == nil {
		t.Fatal("pushWithRetry succeeded, want bounded failure")
	}
	if !strings.Contains(err.Error(), "concurrent") {
		t.Errorf("error should explain the contention, got: %v", err)
	}
	if retries != maxPushAttempts-1 {
		t.Errorf("retries = %d, want %d", retries, maxPushAttempts-1)
	}

	pushes, rebases := 0, 0
	for _, c := range calls {
		joined := strings.Join(c, " ")
		switch gitVerb(c) {
		case "push":
			pushes++
			if strings.Contains(joined, "--force") || strings.Contains(joined, "-f ") {
				t.Errorf("force push issued: %q", joined)
			}
		case "pull":
			rebases++
			// The rebase re-creates replayed commits, so its invocation must
			// carry the same pinned identity as commit().
			if !strings.Contains(joined, "user.name=") || !strings.Contains(joined, "user.email=") {
				t.Errorf("rebase invocation missing pinned identity: %q", joined)
			}
		}
	}
	if pushes != maxPushAttempts || rebases != maxPushAttempts-1 {
		t.Errorf("pushes=%d rebases=%d, want %d/%d", pushes, rebases, maxPushAttempts, maxPushAttempts-1)
	}
}

// TestPushWithRetry_NonRejectFailsFast: a non-contention push failure (auth,
// network, missing remote) returns immediately instead of burning retries.
func TestPushWithRetry_NonRejectFailsFast(t *testing.T) {
	newTestHome(t)

	pushes := 0
	a := &Archive{
		cfg:   Config{Remote: "example.invalid/archive.git", Name: "m"},
		clone: t.TempDir(),
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "main\n", nil
			case "push":
				pushes++
				return "fatal: repository not found", errors.New("exit status 128")
			default:
				return "", nil
			}
		},
	}

	if _, err := a.pushWithRetry(context.Background()); err == nil {
		t.Fatal("pushWithRetry succeeded, want error")
	}
	if pushes != 1 {
		t.Errorf("pushes = %d, want 1 (fail fast on non-reject)", pushes)
	}
}

// TestPushWithRetry_AbortsWedgedRebase: a failing rebase is followed by
// `rebase --abort` so the clone is never left mid-rebase.
func TestPushWithRetry_AbortsWedgedRebase(t *testing.T) {
	newTestHome(t)

	var calls []string
	a := &Archive{
		cfg:   Config{Remote: "example.invalid/archive.git", Name: "m"},
		clone: t.TempDir(),
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			calls = append(calls, strings.Join(args, " "))
			switch gitVerb(args) {
			case "symbolic-ref":
				return "main\n", nil
			case "push":
				return "! [rejected] main -> main (non-fast-forward)", errors.New("exit status 1")
			case "pull":
				return "error: could not apply", errors.New("exit status 1")
			default:
				return "", nil
			}
		},
	}

	if _, err := a.pushWithRetry(context.Background()); err == nil {
		t.Fatal("pushWithRetry succeeded, want rebase failure")
	}
	aborted := false
	for _, c := range calls {
		if c == "rebase --abort" {
			aborted = true
		}
	}
	if !aborted {
		t.Error("failed rebase was not aborted — clone left wedged")
	}
}

// TestPushLocal_StrandedCommitRecovered: a commit whose push failed (network
// down, kill mid-push) must be pushed by the NEXT run even when no new
// transcript content arrives — otherwise a machine that stops producing
// transcripts strands its last sync forever.
func TestPushLocal_StrandedCommitRecovered(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// First push: the commit lands locally, the push itself dies.
	real := a.run
	a.run = func(ctx context.Context, dir string, args ...string) (string, error) {
		if args[0] == "push" {
			return "fatal: unable to access remote", errors.New("exit status 128")
		}
		return real(ctx, dir, args...)
	}
	if _, err := a.PushLocal(context.Background()); err == nil {
		t.Fatal("first push should have failed")
	}

	// Second run, network back, NOTHING new to copy: the stranded commit must
	// still go out.
	a.run = real
	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("recovery push: %v", err)
	}
	if rep.Copied != 0 || rep.Committed {
		t.Errorf("report = %+v, want no new copy/commit", rep)
	}
	if !rep.Pushed {
		t.Error("stranded commit was not pushed")
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-1111.jsonl")); err != nil {
		t.Errorf("stranded content never reached the remote: %v", err)
	}
}

// TestPushLocal_RemoteMismatchRefuses: an existing clone pointing at a
// different remote than the config is an error with a delete-to-re-clone
// hint, never a silent push to the old remote.
func TestPushLocal_RemoteMismatchRefuses(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	if _, err := Init(context.Background(), bare, "machine-a"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// The config now claims a different remote than the clone's origin.
	otherRemote := initBareRepo(t)
	if err := writeConfig(Config{Remote: otherRemote, Name: "machine-a"}); err != nil {
		t.Fatal(err)
	}
	a, err := Load()
	if err != nil || a == nil {
		t.Fatalf("Load: (%v, %v)", a, err)
	}

	_, err = a.PushLocal(context.Background())
	if err == nil {
		t.Fatal("PushLocal succeeded against a mismatched clone, want refusal")
	}
	if !strings.Contains(err.Error(), "delete the clone dir") {
		t.Errorf("error should hint at deleting the clone, got: %v", err)
	}
}

// TestPushLocal_ClearsStaleCopyTemps: temp files orphaned under
// .git/rawclaw-tmp by a killed run are cleared on the next push and never
// reach the archive.
func TestPushLocal_ClearsStaleCopyTemps(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	stale := filepath.Join(a.ClonePath(), ".git", "rawclaw-tmp", "copy-orphan")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("torn"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale copy temp survived the push")
	}
	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a", "copy-orphan")); !os.IsNotExist(err) {
		t.Error("orphan temp reached the archive")
	}
}

// TestNeedsCopy locks the rsync-style quick check.
func TestNeedsCopy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, content string, mtime time.Time) (string, os.FileInfo) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if !mtime.IsZero() {
			if err := os.Chtimes(p, time.Time{}, mtime); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return p, info
	}

	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	t.Run("missing dst copies", func(t *testing.T) {
		src, info := write("a-src", "hello\n", base)
		if !needsCopy(src, info, filepath.Join(dir, "a-dst-missing")) {
			t.Error("want copy for missing dst")
		}
	})
	t.Run("size change copies", func(t *testing.T) {
		src, info := write("b-src", "hello longer\n", base)
		dst, _ := write("b-dst", "hello\n", base)
		if !needsCopy(src, info, dst) {
			t.Error("want copy for size change")
		}
	})
	t.Run("same size same mtime skips", func(t *testing.T) {
		src, info := write("c-src", "hello\n", base)
		dst, _ := write("c-dst", "XXXXX\n", base) // content ignored on the fast path
		if needsCopy(src, info, dst) {
			t.Error("want skip for equal size+mtime")
		}
	})
	t.Run("same size different mtime same content skips", func(t *testing.T) {
		src, info := write("d-src", "hello\n", base)
		dst, _ := write("d-dst", "hello\n", base.Add(time.Hour))
		if needsCopy(src, info, dst) {
			t.Error("want skip: fingerprints equal (touch-only churn)")
		}
	})
	t.Run("same size different mtime different content copies", func(t *testing.T) {
		src, info := write("e-src", "hello\n", base)
		dst, _ := write("e-dst", "world\n", base.Add(time.Hour))
		if !needsCopy(src, info, dst) {
			t.Error("want copy: in-place rewrite at same size")
		}
	})
}
