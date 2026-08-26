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

## Independent arithmetic correction

The candidate is exactly the requested ancestry: `88e73b512b077aee6ce98789c366814411568eb7`
is a parent of `878f631b74e68aa76302f382e28096dc3d60b545`, whose parent is the former;
`34d2fb05161b1be7819b80804fca2e3a576243cf` is its documentation child.

Personally observed `git diff --numstat 88e73b5^ 878f631 -- internal/cli/cmd_tag.go internal/cli/tagrefresh.go internal/cli/tagrefresh_test.go`:

```text
4   2   internal/cli/cmd_tag.go
0   18  internal/cli/tagrefresh.go
10  0   internal/cli/tagrefresh_test.go
```

Therefore production is `+4 - 2 + 0 - 18 = -16`, tests are `+10`, and the combined
product-plus-tests delta is `-6`. The report above incorrectly says production `-18`
and total product `-8`; `cmd_tag.go`'s `+4/-2` hunk cannot be omitted.

The exact stable patch IDs personally observed are `73f5dd69a25ee9f6e39bcd2036397b46661d741b`
for `88e73b5` and `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc` for `878f631`.

## Located false arithmetic claims

The bounded evidence search found two direct claims for this composite:

1. This report, `FINDINGS.md:L31-L34` at candidate `34d2fb05161b1be7819b80804fca2e3a576243cf`,
   states `-18` production and total product `-8`. It is false by the numstat above.
2. `/Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox/20260826T222103Z-6b2e5b38-tick19-harvest-adopt-or-reject.md:L8`
   repeats `net product -8`. SHA-256 of that immutable receipt is
   `32592e899d2ca918cd32b6450355d584b53b0fc1be9006d27492cf9927ae1728`.

No other `34d2fb0`, `88e73b5`, or `878f631` arithmetic claim was found in the two
specified mailboxes or the bounded candidate evidence paths. Unrelated `-8` estimates
were not attributed to this composite.

## Rival claim challenge

**Claim:** Ozzy's receipt `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260826T220519Z-3f29764e-ozzy-heartbeat-no-fold-race-li.md:L8`
   says sidecar commit `96aa522` is “CLEAN under mutations/full index race/canary.”
   Receipt SHA-256: `77f8f8e44806935f5b338433de1fe2989615fde0094b1dddfe91a4944f11d2bf`.

**Verdict: UNCERTAIN.** The immutable object is
`96aa522611fdcb78e281db31634144e40222de91`, parent
`fb99037cda7c4ca80b6f5294631e5e5c0acc71b6`, changing
`internal/index/consolidated.go` by `+20` and
`internal/index/consolidated_test.go` by `+51`. But the receipt supplies no
mutation command/output, no canary command/output, no exact full-race command or
duration, and no adopter branch/upstream identity. The inspected worktree carrying
the object is at unrelated `HEAD 698852193ac2431620c76759e7c3f4e0fafa52c4` on
`feat/claude-web-import` and has no configured upstream; that is not evidence that
`96aa522` itself passed those gates.

To upgrade this claim to CONFIRMED, require an immutable receipt naming the exact
`96aa522` branch/HEAD and base, clean/upstream equality, the disposable mutation and
canary commands with observed output, and `CGO_ENABLED=0 go test -race -count=1 ./...`
run at that SHA. Until then it earns no score. This challenge is independent of the
composite arithmetic correction.

## Contest score ruling

The corrected arithmetic does not create adoption credit. The composite is a candidate
and the receipts are prose-level requests/claims. Under the contest rules, self-adoption,
pending ideas, convergence, duplicates, and dirty or unpushed work score zero. The
sidecar rival is `UNCERTAIN`, so it also scores zero until the exact gate receipt above
exists. A score change requires externally adopted behavior or an independently observed
rebuttal that prevented a bad merge.

## Minimal personally observed verification

```text
git merge-base --is-ancestor 88e73b5 34d2fb0  # exit 0
git merge-base --is-ancestor 878f631 34d2fb0 # exit 0
git show --format= --numstat 88e73b5         # tagrefresh.go 0/18
git show --format= --numstat 878f631         # cmd_tag.go 4/2; tagrefresh_test.go 10/0
git diff --numstat 88e73b5^ 878f631 -- internal/cli/{cmd_tag.go,tagrefresh.go,tagrefresh_test.go}
                                              # 4/2, 0/18, 10/0
git show 88e73b5 --format= --binary | git patch-id --stable
                                              # 73f5dd69... 000000...
git show 878f631 --format= --binary | git patch-id --stable
                                              # b2d5b3e... 000000...
git diff --check                              # to be run after this report is staged
```
