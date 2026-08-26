# Furiosa composite contract tick 17b

Base: `8c8216e25e22496b2e3e919fce836be49d692e25`

## Verdict

Compatible. Both accepted commits apply cleanly in order, with no invented
third design:

1. `5eb3a38309a5319befaa434483bbf97004312129` → `88e73b5`
2. `3b641ce9582541a60a7b37c8456bedaa9d86d29c` → `878f631`

The first removes dead overlay bookkeeping and `topicSegmentKey`; the second
states detached publication is best effort and pins the queued/help contract.

## Verification

- Focused race tests (`TestTagWrite|Test.*Overlay`): PASS, 4.252s test time
  (8.006s wall).
- Full `CGO_ENABLED=0 go test -race -count=1 ./...`: PASS, 2m34.84s wall.
- `git diff --check`: PASS.
- `gofmt -l internal/`: PASS (no output).
- `golangci-lint run ./internal/cli`: PASS, 0 issues, 5.0s wall.
- `dupl`: unavailable on this host; no result claimed.

## Patch and net-line evidence

- Shrink stable patch ID: `73f5dd69a25ee9f6e39bcd2036397b46661d741b`.
- Best-effort contract stable patch ID:
  `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc`.
- Composite versus base: `internal/cli/tagrefresh.go` 0/+18 deleted;
  `cmd_tag.go` +4/-2; `tagrefresh_test.go` +10/-0.
- Net: -18 production lines, +10 test lines, 0 documentation lines before
  this report; total product delta is -8 lines.
- Final product HEAD: `878f631`; report commit follows separately.
