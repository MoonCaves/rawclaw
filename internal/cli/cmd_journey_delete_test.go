package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestCLIJourney_MirrorRetentionDeleteInvariant pins the complete end-to-end delete
// semantics across the compiled Cobra command tree:
//
// 1. Isolation & Hermetic verification:
//   - Isolates HOME, CLAUDE_CONFIG_DIR, ANTIGRAVITY_HOME, XDG_DATA_HOME,
//     XDG_CACHE_HOME, XDG_CONFIG_HOME, GOOSE_HOME, CODEX_HOME into t.TempDir().
//   - Hard-fails if any resolved store/cache/vault/projects path escapes the sandbox.
//
// 2. LIVE session deletion invariant:
//   - Live session is indexed and backed by a real on-disk transcript file.
//   - Non-interactive delete with --yes alone is refused (exit 2) when original files
//     would be deleted.
//   - Delete with --yes --files removes the provider's original transcript file,
//     evicts rawclaw's durable vault copy, appends the session ID to the tombstone
//     sidecar (.deleted), and prunes the session from search results.
//   - Verifies the live provenance receipt note in output.
//
// 3. RETAINED session deletion invariant:
//   - Retained session has had its provider source file purged upstream, but its
//     history remains searchable in rawclaw.
//   - Delete with --yes alone succeeds (no original files exist to delete).
//   - Evicts rawclaw's durable vault copy, appends to the tombstone sidecar, and
//     prunes the session from search results.
//   - Verifies the retained provenance receipt note in output.
//
// 4. Tombstone invariant & re-index resurrection refusal:
//   - Dropping a transcript back onto disk with a tombstoned session ID does NOT
//     resurrect the session during --reindex.
//   - Search continues to return 0 hits for the tombstoned session.
//
// 5. Mirror / Archive propagation & Foreign protection invariant:
//   - Local delete propagates to the remote git archive upon `rawclaw archive push`:
//     the deleted session is removed from the archive remote repository.
//   - Foreign session from another machine (machine-b) in the archive is protected:
//     local delete targeting the foreign session is refused (exit 1) with an origin
//     machine pointer.
//   - Foreign session transcript files in the local clone and remote archive remain
//     intact and searchable.
func TestCLIJourney_MirrorRetentionDeleteInvariant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}

	sandbox := t.TempDir()

	// 1. Strict hermetic isolation across all runtime environments
	t.Setenv("HOME", sandbox)
	t.Setenv("USERPROFILE", sandbox)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sandbox, ".claude"))
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(sandbox, ".gemini", "antigravity-cli"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sandbox, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(sandbox, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, ".config"))
	t.Setenv("GOOSE_HOME", filepath.Join(sandbox, ".goose"))
	t.Setenv("CODEX_HOME", filepath.Join(sandbox, ".codex"))
	t.Setenv("RAWCLAW_ARCHIVE", "")
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "off")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")

	// Hard-fail guard assertions: no path must escape the temporary sandbox
	cacheDir := store.CacheDir()
	if !strings.HasPrefix(cacheDir, sandbox) {
		t.Fatalf("CRITICAL: store.CacheDir() = %q is not isolated inside sandbox %q", cacheDir, sandbox)
	}
	transcriptsRoot := paths.TranscriptsRoot()
	if !strings.HasPrefix(transcriptsRoot, sandbox) {
		t.Fatalf("CRITICAL: paths.TranscriptsRoot() = %q is not isolated inside sandbox %q", transcriptsRoot, sandbox)
	}
	projectsRoot := paths.ProjectsRoot()
	if !strings.HasPrefix(projectsRoot, sandbox) {
		t.Fatalf("CRITICAL: paths.ProjectsRoot() = %q is not isolated inside sandbox %q", projectsRoot, sandbox)
	}
	tombstonePath := lifecycle.TombstonePath("")
	if !strings.HasPrefix(tombstonePath, sandbox) {
		t.Fatalf("CRITICAL: lifecycle.TombstonePath(\"\") = %q is not isolated inside sandbox %q", tombstonePath, sandbox)
	}

	// 2. Setup Identities, Projects, and Beacons
	const (
		liveSessID     = "11112222-0000-0000-0000-000000000001"
		liveBeacon     = "quantum_flux_stabilizer"
		retainedSessID = "22223333-0000-0000-0000-000000000002"
		retainedBeacon = "hyperdrive_core_manifold"
		foreignSessID  = "33334444-0000-0000-0000-000000000003"
		foreignBeacon  = "stellar_warp_matrix"
		foreignMachine = "machine-beta"
	)

	projDir := filepath.Join(sandbox, ".claude", "projects", "-local-subspace")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	livePath := filepath.Join(projDir, liveSessID+".jsonl")
	liveJSONL := `{"type":"user","uuid":"u-live-1","timestamp":"2026-08-01T10:00:00Z","cwd":"/work/subspace","message":{"role":"user","content":"calibrate ` + liveBeacon + ` in subspace engine room"}}` + "\n"
	if err := os.WriteFile(livePath, []byte(liveJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	retainedPath := filepath.Join(projDir, retainedSessID+".jsonl")
	retainedJSONL := `{"type":"user","uuid":"u-ret-1","timestamp":"2026-08-01T11:00:00Z","cwd":"/work/subspace","message":{"role":"user","content":"diagnose ` + retainedBeacon + ` in subspace telemetry"}}` + "\n"
	if err := os.WriteFile(retainedPath, []byte(retainedJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	// Setup Bare Git Remote for Archive Mirror
	bareDir := filepath.Join(t.TempDir(), "archive-remote.git")
	runGit(t, "", "init", "--bare", "--initial-branch=main", bareDir)

	// Initialize archive on local machine via CLI
	outInit, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "archive", "init", bareDir, "--name", "machine-alpha")
	if err != nil {
		t.Fatalf("archive init failed: %v\nout: %s", err, outInit)
	}

	// Ingest sessions into rawclaw index + durable vault via search --reindex
	outSearchInit, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", "subspace")
	if err != nil {
		t.Fatalf("initial ingest search failed: %v\nout: %s", err, outSearchInit)
	}
	var envInit agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchInit), &envInit); err != nil {
		t.Fatalf("unmarshal search output: %v\nout: %s", err, outSearchInit)
	}
	if envInit.Count < 2 {
		t.Fatalf("initial ingest found %d sessions, want at least 2; out: %s", envInit.Count, outSearchInit)
	}

	// Verify both sessions were stored into rawclaw durable vault
	if !durable.Has(liveSessID) {
		t.Fatalf("live session %s missing from durable vault after ingest", liveSessID)
	}
	if !durable.Has(retainedSessID) {
		t.Fatalf("retained session %s missing from durable vault after ingest", retainedSessID)
	}

	// Push local sessions to the archive via CLI
	outPush1, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "archive", "push")
	if err != nil {
		t.Fatalf("archive push failed: %v\nout: %s", err, outPush1)
	}
	if !strings.Contains(outPush1, "Pushed") {
		t.Fatalf("expected Pushed confirmation in archive push output: %s", outPush1)
	}

	// Simulate foreign machine (machine-beta) pushing a session to the bare remote
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	runGit(t, "", "clone", bareDir, cloneB)
	bMachineDir := filepath.Join(cloneB, foreignMachine)
	if err := os.MkdirAll(filepath.Join(bMachineDir, "claude", "-remote-subspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestB := `{"machine_id":"bbbb9999bbbb9999bbbb9999bbbb9999","name":"` + foreignMachine + `","hostname":"beta-node","updated_at":"2026-08-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(bMachineDir, ".rawclaw-machine.json"), []byte(manifestB), 0o644); err != nil {
		t.Fatal(err)
	}
	foreignJSONL := `{"type":"user","uuid":"u-for-1","timestamp":"2026-08-01T12:00:00Z","cwd":"/remote/subspace","message":{"role":"user","content":"analyze ` + foreignBeacon + ` coordinates"}}` + "\n"
	if err := os.WriteFile(filepath.Join(bMachineDir, "claude", "-remote-subspace", foreignSessID+".jsonl"), []byte(foreignJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, cloneB, "add", "-A")
	runGit(t, cloneB, "commit", "-m", "machine-beta: initial sync")
	runGit(t, cloneB, "push", "origin", "HEAD")

	// Pull foreign archive content into local machine via CLI
	outPull, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "archive", "pull")
	if err != nil {
		t.Fatalf("archive pull failed: %v\nout: %s", err, outPull)
	}

	// Search foreign beacon with --reindex to ingest foreign replica into search index
	outSearchForeign, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", foreignBeacon)
	if err != nil {
		t.Fatalf("search foreign beacon failed: %v\nout: %s", err, outSearchForeign)
	}
	var envForeign agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchForeign), &envForeign); err != nil {
		t.Fatalf("unmarshal foreign search: %v", err)
	}
	if envForeign.Count != 1 || envForeign.Results[0].SessionID != foreignSessID {
		t.Fatalf("foreign search want session %s, got %+v\nout: %s", foreignSessID, envForeign.Results, outSearchForeign)
	}

	// Transition retained session: purge provider source file upstream, then re-index
	if err := os.Remove(retainedPath); err != nil {
		t.Fatal(err)
	}
	outSearchRetainCheck, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", retainedBeacon)
	if err != nil {
		t.Fatalf("reindex after upstream purge failed: %v\nout: %s", err, outSearchRetainCheck)
	}
	var envRetainCheck agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchRetainCheck), &envRetainCheck); err != nil {
		t.Fatalf("unmarshal retained search: %v", err)
	}
	if envRetainCheck.Count != 1 || envRetainCheck.Results[0].SessionID != retainedSessID {
		t.Fatalf("retained session %s should still be searchable, got %+v", retainedSessID, envRetainCheck.Results)
	}
	if durable.Has(retainedSessID) == false {
		t.Fatalf("retained session %s must remain in durable vault before delete", retainedSessID)
	}

	// =========================================================================
	// 3. TEST INVARIANT: LIVE SESSION DELETION
	// =========================================================================
	t.Run("LiveSessionDelete", func(t *testing.T) {
		// Non-interactive delete with --yes alone MUST REFUSE when live files would be removed
		outRefuse, errRefuse := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", liveSessID[:8])
		if errRefuse == nil {
			t.Fatalf("delete --yes alone on live session should fail; out:\n%s", outRefuse)
		}
		var ee ExitError
		if !asExit(errRefuse, &ee) || ee.Code != 2 {
			t.Fatalf("expected ExitError Code 2 for live delete without --files, got err=%v", errRefuse)
		}
		if !strings.Contains(ee.Msg, "--yes --files") {
			t.Errorf("refusal message missing advice to pass --yes --files; got: %q", ee.Msg)
		}
		// Provider file and vault must still exist after refusal
		if _, serr := os.Stat(livePath); serr != nil {
			t.Fatalf("live transcript deleted despite refusal: %v", serr)
		}
		if !durable.Has(liveSessID) {
			t.Fatalf("durable vault copy deleted despite refusal")
		}

		// Real live delete with --yes --files
		outDelLive, errDelLive := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", "--files", liveSessID[:8])
		if errDelLive != nil {
			t.Fatalf("delete --yes --files on live session failed: %v\nout:\n%s", errDelLive, outDelLive)
		}

		// (a) Provenance receipt verification
		if !strings.Contains(outDelLive, "Deleted 1 session") {
			t.Errorf("output missing deletion count summary; out: %s", outDelLive)
		}
		if !strings.Contains(outDelLive, "Removed rawclaw's copy (index and archive) and the original session transcript files.") {
			t.Errorf("output missing live provenance note; out: %s", outDelLive)
		}

		// (b) Provider transcript file MUST be removed from disk
		if _, serr := os.Stat(livePath); !os.IsNotExist(serr) {
			t.Errorf("provider live transcript file still present after delete: %v", serr)
		}

		// (c) Rawclaw durable vault copy MUST be evicted
		if durable.Has(liveSessID) {
			t.Errorf("session %s still present in durable vault after delete", liveSessID)
		}
		vaultTransPath, _ := durable.PathFor(liveSessID)
		if _, serr := os.Stat(vaultTransPath); !os.IsNotExist(serr) {
			t.Errorf("vault transcript file still exists at %s", vaultTransPath)
		}

		// (d) Tombstone sidecar MUST contain the session ID
		tombContent, rerr := os.ReadFile(tombstonePath)
		if rerr != nil {
			t.Fatalf("read tombstone file %s: %v", tombstonePath, rerr)
		}
		if !strings.Contains(string(tombContent), liveSessID) {
			t.Errorf("tombstone sidecar does not contain deleted live session ID %s; content:\n%s", liveSessID, string(tombContent))
		}

		// (e) Search for the live beacon MUST return 0 results
		outSearchLiveAfter, errSearch := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", liveBeacon)
		if errSearch != nil {
			t.Fatalf("search after live delete failed: %v\nout: %s", errSearch, outSearchLiveAfter)
		}
		var envLiveAfter agentproto.SearchEnvelope
		if err := json.Unmarshal([]byte(outSearchLiveAfter), &envLiveAfter); err != nil {
			t.Fatalf("unmarshal search output: %v", err)
		}
		if envLiveAfter.Count != 0 {
			t.Errorf("search returned %d hits for deleted live session; want 0\nout: %s", envLiveAfter.Count, outSearchLiveAfter)
		}
	})

	// =========================================================================
	// 4. TEST INVARIANT: RETAINED SESSION DELETION
	// =========================================================================
	t.Run("RetainedSessionDelete", func(t *testing.T) {
		// Retained-only delete with --yes alone MUST SUCCEED (no provider file to remove)
		outDelRet, errDelRet := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", retainedSessID[:8])
		if errDelRet != nil {
			t.Fatalf("delete --yes on retained session failed: %v\nout:\n%s", errDelRet, outDelRet)
		}

		// (a) Provenance receipt verification
		if !strings.Contains(outDelRet, "Deleted 1 session(s) (1 retained)") {
			t.Errorf("output missing retained deletion summary; out: %s", outDelRet)
		}
		if !strings.Contains(outDelRet, "Removed rawclaw's copy (index + archive). Claude Code / Codex transcript files are untouched.") {
			t.Errorf("output missing retained provenance note; out: %s", outDelRet)
		}

		// (b) Provider transcript file remains absent
		if _, serr := os.Stat(retainedPath); !os.IsNotExist(serr) {
			t.Errorf("unexpected file at retained path: %v", serr)
		}

		// (c) Rawclaw durable vault copy MUST be evicted
		if durable.Has(retainedSessID) {
			t.Errorf("retained session %s still present in durable vault after delete", retainedSessID)
		}

		// (d) Tombstone sidecar MUST contain the retained session ID
		tombContent, rerr := os.ReadFile(tombstonePath)
		if rerr != nil {
			t.Fatalf("read tombstone file: %v", rerr)
		}
		if !strings.Contains(string(tombContent), retainedSessID) {
			t.Errorf("tombstone sidecar does not contain deleted retained session ID %s", retainedSessID)
		}

		// (e) Search for retained beacon MUST return 0 results
		outSearchRetAfter, errSearch := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", retainedBeacon)
		if errSearch != nil {
			t.Fatalf("search after retained delete failed: %v\nout: %s", errSearch, outSearchRetAfter)
		}
		var envRetAfter agentproto.SearchEnvelope
		if err := json.Unmarshal([]byte(outSearchRetAfter), &envRetAfter); err != nil {
			t.Fatalf("unmarshal search output: %v", err)
		}
		if envRetAfter.Count != 0 {
			t.Errorf("search returned %d hits for deleted retained session; want 0\nout: %s", envRetAfter.Count, outSearchRetAfter)
		}
	})

	// =========================================================================
	// 5. TEST INVARIANT: TOMBSTONE RE-INDEX RESURRECTION REFUSAL
	// =========================================================================
	t.Run("TombstoneReindexResurrectionRefusal", func(t *testing.T) {
		// Restore the live session transcript file on disk with the same session ID
		if err := os.WriteFile(livePath, []byte(liveJSONL), 0o644); err != nil {
			t.Fatalf("restore live transcript file: %v", err)
		}

		// Force reindex
		outReindex, errReindex := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", liveBeacon)
		if errReindex != nil {
			t.Fatalf("reindex with restored file failed: %v\nout: %s", errReindex, outReindex)
		}
		var envReindex agentproto.SearchEnvelope
		if err := json.Unmarshal([]byte(outReindex), &envReindex); err != nil {
			t.Fatalf("unmarshal reindex search output: %v", err)
		}

		// Tombstone MUST prevent resurrection: search count must be 0
		if envReindex.Count != 0 {
			t.Errorf("tombstoned session was resurrected by --reindex: got %d hits, want 0\nout: %s", envReindex.Count, outReindex)
		}

		// Clean up the restored file so it does not interfere with later checks
		_ = os.Remove(livePath)
	})

	// =========================================================================
	// 6. TEST INVARIANT: ARCHIVE MIRROR PROPAGATION & FOREIGN PROTECTION
	// =========================================================================
	t.Run("ArchiveMirrorPropagationAndForeignProtection", func(t *testing.T) {
		// Run archive push to propagate the local deletions
		outPush2, errPush2 := runCmd(t, NewRootCmd(BuildInfo{}), "", "archive", "push")
		if errPush2 != nil {
			t.Fatalf("archive push after delete failed: %v\nout: %s", errPush2, outPush2)
		}
		if !strings.Contains(strings.ToLower(outPush2), "removed") {
			t.Errorf("archive push output missing removed report; out: %s", outPush2)
		}

		// Verify remote archive repository: local deleted sessions MUST be removed
		verifyRemote := filepath.Join(t.TempDir(), "verify-remote")
		runGit(t, "", "clone", bareDir, verifyRemote)

		deletedRemoteLive := filepath.Join(verifyRemote, "machine-alpha", "claude", "-local-subspace", liveSessID+".jsonl")
		if _, serr := os.Stat(deletedRemoteLive); !os.IsNotExist(serr) {
			t.Errorf("deleted live session still present in remote archive: %s", deletedRemoteLive)
		}

		deletedRemoteRet := filepath.Join(verifyRemote, "machine-alpha", "claude", "-local-subspace", retainedSessID+".jsonl")
		if _, serr := os.Stat(deletedRemoteRet); !os.IsNotExist(serr) {
			t.Errorf("deleted retained session still present in remote archive: %s", deletedRemoteRet)
		}

		// Foreign session MUST remain intact in the remote archive
		foreignRemoteFile := filepath.Join(verifyRemote, foreignMachine, "claude", "-remote-subspace", foreignSessID+".jsonl")
		if _, serr := os.Stat(foreignRemoteFile); serr != nil {
			t.Errorf("foreign session missing from remote archive: %v", serr)
		}

		// Foreign session local deletion attempt by ID MUST BE REFUSED (exit 1)
		outForeignDelID, errForeignDelID := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", foreignSessID[:8])
		if errForeignDelID == nil {
			t.Fatalf("delete of foreign session by ID succeeded, want refusal; out:\n%s", outForeignDelID)
		}
		var eeID ExitError
		if !asExit(errForeignDelID, &eeID) || eeID.Code != 1 {
			t.Fatalf("want ExitError Code 1 on foreign delete by ID, got %v", errForeignDelID)
		}
		if !strings.Contains(eeID.Msg, "read-only") || !strings.Contains(eeID.Msg, foreignMachine) {
			t.Errorf("foreign refusal message should state read-only and name origin machine %s; got: %q", foreignMachine, eeID.Msg)
		}

		// Foreign session deletion attempt by filter matching only foreign scope MUST BE REFUSED (exit 1)
		outForeignDelProj, errForeignDelProj := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--project", foreignMachine, "--yes")
		if errForeignDelProj == nil {
			t.Fatalf("delete matching foreign-only filter succeeded, want refusal; out:\n%s", outForeignDelProj)
		}
		var eeProj ExitError
		if !asExit(errForeignDelProj, &eeProj) || eeProj.Code != 1 {
			t.Fatalf("want ExitError Code 1 on foreign-only filter delete, got %v", errForeignDelProj)
		}
		if !strings.Contains(eeProj.Msg, "read-only") || !strings.Contains(eeProj.Msg, foreignMachine) {
			t.Errorf("foreign filter refusal should state read-only and name origin machine %s; got: %q", foreignMachine, eeProj.Msg)
		}

		// Foreign session file in local clone MUST still exist
		localArchive, aerr := archive.Load()
		if aerr != nil || localArchive == nil {
			t.Fatalf("archive.Load: %v", aerr)
		}
		localCloneForeign := filepath.Join(localArchive.ClonePath(), foreignMachine, "claude", "-remote-subspace", foreignSessID+".jsonl")
		if _, serr := os.Stat(localCloneForeign); serr != nil {
			t.Errorf("foreign session in local clone was touched/removed: %v", serr)
		}

		// Foreign session ID MUST NOT be in the tombstone sidecar
		tombContent, _ := os.ReadFile(tombstonePath)
		if strings.Contains(string(tombContent), foreignSessID) {
			t.Errorf("foreign session ID %s was wrongly tombstoned", foreignSessID)
		}

		// Search for foreign beacon MUST still return the foreign result
		outSearchForeignAfter, errSearchForeign := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", foreignBeacon)
		if errSearchForeign != nil {
			t.Fatalf("search foreign beacon after attempted delete failed: %v\nout: %s", errSearchForeign, outSearchForeignAfter)
		}
		var envForeignAfter agentproto.SearchEnvelope
		if err := json.Unmarshal([]byte(outSearchForeignAfter), &envForeignAfter); err != nil {
			t.Fatalf("unmarshal foreign search output: %v", err)
		}
		if envForeignAfter.Count != 1 || envForeignAfter.Results[0].SessionID != foreignSessID {
			t.Errorf("foreign session not searchable after attempted delete; got %+v", envForeignAfter.Results)
		}
	})
}

// runGit executes git in the given directory with deterministic identity config.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test-operator",
		"-c", "user.email=test@rawclaw.invalid",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.CommandContext(context.Background(), "git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q failed: %v\noutput:\n%s", args, dir, err, string(out))
	}
	return string(out)
}
