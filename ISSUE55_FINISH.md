# Issue #55 finish receipt

- Verdict: SHIP candidate / PATCH
- Base: `f73ee0e89c30d1c5429068708325426d5c5b5fbe`
- Commit: `15734d924dd9a37e9f023dac8011c883f0c6b29a`
- Exact transplant: `internal/cli/setup.go` copies the existing `3f1113f` flat/local shape into both `rawclawPrimeScript` and `rawclawCodexPrimeScript`: `catalog_session_id=$session_id` followed by `case "$catalog_session_id" in ''|.*|*[!A-Za-z0-9._-]*) catalog_session_id= ;; esac`. Invalid IDs ingest fail-soft without becoming path components; valid IDs retain the catalog claim path.
- Focused regression: `TestPrimeScripts_SessionStartCatalogClaimIsPathSafe`, Claude/Codex x sh/dash x FIFO/directory/traversal, passed with `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestPrimeScripts_SessionStartCatalogClaimIsPathSafe$' -v` (exit 0, 5.108s).
- Diff: 2 owning files from base, `+239/-74`; no unrelated source changes. `gofmt -w internal/cli/setup.go internal/cli/cmd_ingest_test.go` and `git diff --check` passed.
- Dedup: origin `norm/integration-wave2-currentbase` has equivalent source commit `3f1113f3ee10aa514cdbd64a3976dd907b13ee36`, but it is based on `029f60d`; this candidate is the same transplant rebased directly onto requested `f73ee0e` and contains only the fenced two files plus this receipt.
- Full `internal/cli` race gate was attempted but produced no completion output in the bounded runner; do not treat that attempt as green.
