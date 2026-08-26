# Furiosa composite contract tick 17

Base: `8c8216e25e22496b2e3e919fce836be49d692e25`

## Verdict

Not admissible within the assigned file fence. The two accepted patches are
textually compatible, but `5eb3a38309a5319befaa434483bbf97004312129` edits
`internal/cli/tagrefresh.go`, which is outside the fence
(`internal/cli/cmd_tag.go`, `internal/cli/tagrefresh_test.go`, `FINDINGS.md`).
Per the no-product-edits rule, this branch remains at the exact base.

## Composition evidence

- `5eb3a38` is a clean 18-line deletion in `tagrefresh.go`: it removes the
  dead derived-overlay bookkeeping and `topicSegmentKey`, preserving the
  authoritative replacement copy.
- `3b641ce9582541a60a7b37c8456bedaa9d86d29c` is a clean fenced patch: two
  production help/receipt lines and two focused regression tests.
- `git merge-tree` reports no product-content conflict between the patches.
  The only combined-tip content conflict is `FINDINGS.md`, because each
  accepted branch carries a different report; that is resolvable by a report
  author, not a product-design merge.
- Applying `3b641ce` alone to this base passed the focused race tests, but it
  was reverted so the delivered tree contains no product edits.

## Verification

- Focused base race tests (`TestTagWrite|Test.*Overlay`): PASS, 3.971s test
  time (5.332s wall).
- `git diff --check`: PASS.
- `gofmt -l internal/`: PASS (no output).
- `golangci-lint run ./internal/cli`: PASS, 0 issues, 2.5s wall.
- `dupl`: unavailable on this host; no result claimed.

## Patch identities and scope

- Shrink patch: `5eb3a38309a5319befaa434483bbf97004312129`; stable patch ID
  `73f5dd69a25ee9f6e39bcd2036397b46661d741b`.
- Best-effort contract patch: `3b641ce9582541a60a7b37c8456bedaa9d86d29c`;
  stable patch ID `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc`.
- Delivered HEAD remains `8c8216e25e22496b2e3e919fce836be49d692e25`.
- Delivered net product lines: 0 production, 0 test, 0 documentation;
  this report is the only new file.
