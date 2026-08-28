# PR81 Mutation Strength Audit Report

- **Target Head**: `8323fd9f69c06669f0ad529686008b775e783052`
- **Auditor**: `lenny-pr81-mutation`
- **Worktree**: `/Users/jay-m4/code/rawclaw-lenny-pr81-mutation-20260828`
- **Verdict**: **ACCEPT** (Test suite exhibits 100% kill rate against targeted mutations with zero false-greens)

---

## 1. Executive Summary

This audit independently attacked the PR81 head with five disposable source code mutations targeting the four mandatory correctness invariants:
1. **Catalog & Consolidated Metadata Fast Path**: Bypassing consolidated metadata check in `resumeExactMetadata`.
2. **Prefix Ambiguity Protection**: Weakening `isFullResumeID` to allow 32-character prefixes to act as exact full IDs.
3. **Recorded-Source-Only Fallback**: Removing the `namedSources` constraint during adapter exact lookup.
4. **Unsafe Parent/Subagent Rejection**: Weakening the container filter in `resumeExactMetadata` to admit child/subagent containers.
5. **Metadata Guard & Safe Command Explanation**: Bypassing `metadataGuard` handling in `runResume`.

All five mutations were caught immediately by dedicated regression tests in `./internal/cli` under race detection.

---

## 2. Test Filter Verification

- **Command**: `go test -list 'TestRunResume|TestResume' ./internal/cli`
- **Matched Tests** (16 total):
  1. `TestResume_ForeignSessionDegrades`
  2. `TestResume_ForeignUnsearchedMissesCheaply`
  3. `TestResume_LocalStillWins`
  4. `TestRunResumeDoesNotEmitRetainedLocalCommand`
  5. `TestRunResumePreservesCrossSourceAmbiguity`
  6. `TestRunResumeFindsUncatalogedLiveExactHitWithEmptyStore`
  7. `TestRunResumeAuthoritativeMetadataSkipsAdapterLookup`
  8. `TestRunResumeParentMetadataSkipsAdapterLookup`
  9. `TestRunResumeStaleMetadataProbesOnlyNamedSource`
  10. `TestRunResumeIncompleteExactLookupDeduplicatesCatalogFallback`
  11. `TestRunResumeTreats32CharacterUUIDPrefixAsPrefix`
  12. `TestRunResumeRetainedMetadataNeverBecomesRunnable`
  13. `TestRunResumeLiveMetadataMissingSourcePathIsNotRunnable`
  14. `TestRunResumeRetainedMetadataDoesNotProbeAnotherSource`
  15. `TestRunResumeRejectsChildExactAdapterContainers`
  16. `TestRunResumeResolvesCatalogSession`

---

## 3. Mutation Attack Log

### Mutation 1: Bypassing Consolidated Metadata Query Before Discovery
- **Target**: `internal/cli/cli.go:1022-1030` (`resumeExactMetadata`)
- **Diff**:
```diff
--- a/internal/cli/cli.go
+++ b/internal/cli/cli.go
@@ -1022,9 +1022,10 @@ func resumeExactMetadata(id string) ([]resumeCandidate, bool, bool) {
-	conMeta := resumeConsolidatedMetadata(id, supported)
-	meta.blocked = meta.blocked || conMeta.blocked
-	meta.unknown = meta.unknown || conMeta.unknown
-	for src := range conMeta.namedSources {
-		meta.namedSources[src] = struct{}{}
-	}
-	for _, candidate := range conMeta.matches {
-		meta.matches = appendResumeCandidate(meta.matches, candidate)
-	}
+	// MUTATION: Skip consolidated metadata query
```
- **Command**: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'`
- **Expected**: RED on `TestRunResumeParentMetadataSkipsAdapterLookup`
- **Observed Result**: RED
  ```
  --- FAIL: TestRunResumeParentMetadataSkipsAdapterLookup (0.03s)
      tagrefresh_test.go:184: parent metadata should not invoke adapter Lookup
  FAIL
  ```
- **Analysis**: Verified test resistance against bypassing consolidated store lookup.

---

### Mutation 2: Weakening Prefix Ambiguity Protection
- **Target**: `internal/cli/cli.go:991-1006` (`isFullResumeID`)
- **Diff**:
```diff
--- a/internal/cli/cli.go
+++ b/internal/cli/cli.go
@@ -991,17 +991,4 @@ func isFullResumeID(id string) bool {
-	if len(id) != 36 {
-		return false
-	}
-	for i, r := range id {
-		if i == 8 || i == 13 || i == 18 || i == 23 {
-			if r != '-' {
-				return false
-			}
-			continue
-		}
-		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
-			return false
-		}
-	}
-	return true
+	if len(id) < 32 {
+		return false
+	}
+	return true
```
- **Command**: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'`
- **Expected**: RED on `TestRunResumeTreats32CharacterUUIDPrefixAsPrefix`
- **Observed Result**: RED
  ```
  --- FAIL: TestRunResumeTreats32CharacterUUIDPrefixAsPrefix (0.00s)
      tagrefresh_test.go:269: 32-character UUID prefix treated as a full ID
  FAIL
  ```
- **Analysis**: Verified that 32-character prefixes cannot silently bypass prefix disambiguation.

---

### Mutation 3: Removing Recorded-Source-Only Fallback Restriction
- **Target**: `internal/cli/cli.go:1040-1044` (`resumeExactMetadata`)
- **Diff**:
```diff
--- a/internal/cli/cli.go
+++ b/internal/cli/cli.go
@@ -1040,5 +1040,2 @@ func resumeExactMetadata(id string) ([]resumeCandidate, bool, bool) {
-		if len(meta.namedSources) > 0 {
-			if _, ok := meta.namedSources[r.ID]; !ok {
-				continue
-			}
-		}
+		// MUTATION: remove recorded-source-only filter
```
- **Command**: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'`
- **Expected**: RED on `TestRunResumeStaleMetadataProbesOnlyNamedSource`
- **Observed Result**: RED
  ```
  --- FAIL: TestRunResumeStaleMetadataProbesOnlyNamedSource (0.01s)
      tagrefresh_test.go:220: stale metadata probes: named=true unrelated=true
  FAIL
  ```
- **Analysis**: Proves that stale metadata cannot trigger probing of unrelated source adapters.

---

### Mutation 4: Weakening Unsafe Parent / Subagent Rejection
- **Target**: `internal/cli/cli.go:1058` (`resumeExactMetadata`)
- **Diff**:
```diff
--- a/internal/cli/cli.go
+++ b/internal/cli/cli.go
@@ -1058,1 +1058,2 @@ func resumeExactMetadata(id string) ([]resumeCandidate, bool, bool) {
-			if c.ID == id && !c.IsSubagent && c.ParentID == "" {
+			// MUTATION: allow child / subagent containers
+			if c.ID == id {
```
- **Command**: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'`
- **Expected**: RED on `TestRunResumeRejectsChildExactAdapterContainers`
- **Observed Result**: RED
  ```
  --- FAIL: TestRunResumeRejectsChildExactAdapterContainers (0.00s)
      tagrefresh_test.go:371: child exact container emitted a runnable command:
          Resume this session (subagent):
          
            cd /subagent && claude --resume 12345678-1234-4234-8234-123456789009
  FAIL
  ```
- **Analysis**: Proves that subagents and forked child containers are strictly prevented from emitting runnable resume commands.

---

### Mutation 5: Bypassing Metadata Guard Explanation
- **Target**: `internal/cli/cli.go:918-925` (`runResume`)
- **Diff**:
```diff
--- a/internal/cli/cli.go
+++ b/internal/cli/cli.go
@@ -918,8 +918,4 @@ func runResume(w io.Writer, o *Options) error {
-		matches, complete, metadataGuard = resumeExactMetadata(o.Resume)
-		if complete || metadataGuard {
-			if metadataGuard && len(matches) == 0 {
-				fmt.Fprintf(w, "Session %s is known, but RawClaw cannot produce a safe local resume command for it.\n", o.Resume)
-				return nil
-			}
+		matches, complete, _ = resumeExactMetadata(o.Resume)
+		if complete {
 			return emitResumeMatches(w, o, matches, false)
 		}
```
- **Command**: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'`
- **Expected**: RED on `TestRunResumeDoesNotEmitRetainedLocalCommand`
- **Observed Result**: RED
  ```
  --- FAIL: TestRunResumeDoesNotEmitRetainedLocalCommand (0.03s)
      tagrefresh_test.go:49: retained session did not explain why resume is unavailable: No session id starts with '87783881-b4b8-4694-8095-12c180e13643'. Use the 8-char id from search output, e.g. [… · a1b2c3d4 · …].
  FAIL
  ```
- **Analysis**: Ensures that unavailable / retained sessions receive clear explanations rather than misleading fallback messages.

---

## 4. Final Verification and Conclusion

- All disposable source mutations were reverted and verified clean via `git status` and `git diff --check`.
- The full focused race suite `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunResume|TestResume'` passed cleanly in 4.2s across all 16 test cases.
- Source code in `internal/` was untouched in final working tree.

**Final Verdict**: **ACCEPT**
