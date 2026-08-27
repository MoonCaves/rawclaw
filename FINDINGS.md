# Tick 31 harvest-payload referee

Base requested for this referee: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Verdict for all three Tick 30 commits: **EVIDENCE-ONLY**. Report text is not
product payload and there is no transplant candidate.

## `b3813cbb352492551e9d9387edc7fa4039165cd6`

Parent/base is exactly `0d1da19`. The commit changes only root `FINDINGS.md`:

```text
git show -s --format='%H%n%P%n%s' b3813cb
b3813cbb352492551e9d9387edc7fa4039165cd6
0d1da19c4c21961b86cb3ca84ed047d941c83ed3
dedupe WAL prior-art recommendations
git diff --stat b3813cb^ b3813cb
 FINDINGS.md | 77 +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
 1 file changed, 77 insertions(+)
git diff --name-status b3813cb^ b3813cb
A FINDINGS.md
git diff b3813cb^ b3813cb | git patch-id --stable
e193272c4e1c9b057a2946f3b659963f6bc6c6bb 0000000000000000000000000000000000000000
```

It contains no `internal/` or other product path. Its WAL duplicate ruling is
review evidence only. No production/test/doc payload is transplantable:
production `0`, tests `0`, docs `0`, report-only `+77`.

## `8b6c0c3d89cb4d0a0efe78cd1a6d5844c42970c0`

Parent/base is exactly `0d1da19`. The commit changes only root `FINDINGS.md`:

```text
git show -s --format='%H%n%P%n%s' 8b6c0c3
8b6c0c3d89cb4d0a0efe78cd1a6d5844c42970c0
0d1da19c4c21961b86cb3ca84ed047d941c83ed3
audit: classify weighted semaphore as duplicate
git diff --stat 8b6c0c3^ 8b6c0c3
 FINDINGS.md | 74 +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
 1 file changed, 74 insertions(+)
git diff --name-status 8b6c0c3^ 8b6c0c3
A FINDINGS.md
git diff 8b6c0c3^ 8b6c0c3 | git patch-id --stable
4e84b71dcd304d8bcba4c38b4ea8e7f26be5e8e9 0000000000000000000000000000000000000000
```

It contains no product path, dependency edit, or test file. The semaphore
duplicate/minimalism analysis is evidence only. No production/test/doc payload
is transplantable: production `0`, tests `0`, docs `0`, report-only `+74`.

## `5878c48064a797314986a884e10163b086a84c5c`

This tip is based on `386ec9d`, not the requested `0d1da19` base. Its parent is
the batch-prune implementation, but the commit itself still changes only root
`FINDINGS.md`:

```text
git show -s --format='%H%n%P%n%s' 5878c48
5878c48064a797314986a884e10163b086a84c5c
386ec9d03bc4b4ae77ef8238d06e0f8b0782de21
test(index): audit live-ID prune benchmark evidence
git diff --stat 5878c48^ 5878c48
 FINDINGS.md | 70 +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
 1 file changed, 70 insertions(+)
git diff --name-status 5878c48^ 5878c48
A FINDINGS.md
git diff 5878c48^ 5878c48 | git patch-id --stable
582c9468e3fddd4aa0a1a5f303d0ab7d4b369d0e 0000000000000000000000000000000000000000
```

The report records a disposable mutation of the existing benchmark and says
the source was restored byte-for-byte. That is useful evidence about
`386ec9d`, but it is not a source/test mutation committed in this tip and is
not a transplantable product payload. Its base mismatch further prevents
current-base integration credit. Payload lines: production `0`, tests `0`,
docs `0`, report-only `+70`.

The report's stated range comparison is therefore evidence only; no
current-base `range-diff` or product patch exists to apply.

## State and accounting

All three inspected worktrees were clean and upstream-parity:

```text
worker/furiosa-t30-wal-dedupe-20260827 b3813cbb... status=0 parity=0/0
worker/furiosa-t30-semaphore-audit-20260827 8b6c0c3d... status=0 parity=0/0
worker/furiosa-t30-386-bench-replacement-20260827 5878c480... status=0 parity=0/0
```

`range-diff` cannot produce a product comparison for any tip because each
payload is a single report-file addition. No score, adoption, or transplant
authorization follows. Exact test-list count for a focused gate is `0`: no
gate was needed for report-only commits.

