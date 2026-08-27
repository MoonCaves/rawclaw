# T52 terminal-receipt prior-art findings

## Scope and ruling

Report-only lane. Exact base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`.
Production and test code are unchanged. The only failure under review is parent death after the
authoritative tag transaction commits but before child `Start` or its first byte.

Current-base ruling: `runTagWriteCmd` commits the per-project tag mutation, prints an acknowledgement,
then calls `spawnTagPublishChild`. The child is started and released; its append-only log is not a
durable, queryable job state. T7 therefore still fails: if the parent dies before `Start`, no pending
publication intent exists and no later invocation can recover it. `Wait`, `Release`, a fence, and a
cosmetic log cannot repair that ordering.

The smallest proven fit is an embedded SQLite transactional outbox/local spool: commit immutable
publication identity and `pending` state in the same SQLite transaction as the authoritative mutation;
later invocations claim with a lease, publish, and commit `completed` or `failed` with retry metadata.
This is a proposal, not implementation, adoption, score, Direction Lock, or merge authorization.

## Ponytail Review findings

- `internal/cli/cmd_tag.go:L491-522`: `yagni:` detached launch plus append-only status text cannot provide a durable terminal receipt; replace the handoff with one persisted outbox/spool state machine, or retain best-effort semantics explicitly.
- `internal/cli/tagpublish.go:L36-70`: `delete:` `Process.Release` as a completion mechanism; durable pending identity and later drain are the required replacement for parent-death recovery.
- `internal/cli/bg_ingest.go:L99-101`, `internal/cli/vectortopup.go:L43-46`, `internal/cli/autosync.go:L84-92`: `delete:` per-child lifecycle variants do not establish a terminal publication contract; share one narrow persisted handoff mechanism only if this requirement expands beyond tag publication.

Net-line implication: a real fix adds a small durable schema/state transition and removes detached
receipt assumptions; it should delete duplicated process-specific receipt plumbing rather than add a
general worker framework. No line-count claim is made until an implementation is authorized.

## Routed skill rules that changed the method

- Ponytail: applied the ladder as a prior-art filter: reuse the existing SQLite store and proven queue
  pattern before adding code; prefer deletion of detached receipt variants; do not add a framework for
  speculative future jobs.
- Ponytail Review: used only `delete`, `yagni`, and `shrink`-style findings with file/line anchors;
  correctness findings remain in the ruling, not disguised as complexity scores; ended with net-line
  implications.
- Modular Refactor: stopped at right-sizing because this is a small seam decision, not permission to
  create ports, adapters, or a worker framework; any future change needs a characterization guard and
  one tiny sequenced seam.
- Codebase Design: described the desired durable handoff as one deep module with a small interface
  (enqueue/query/drain) and explicit ordering/error invariants; rejected a shallow pass-through API.
- Graphify: oriented from the graph before targeted inspection; used vocabulary-only literal queries,
  `explain`, and `path`; recorded the `transaction`→`receipt` no-path result as a dead end rather than
  inventing an edge.
- Go concurrency: treated every detached child as a lifecycle liability; required a clear owner,
  exit, wait/cancellation story, and explicit distinction between process observation and durable
  application state.
- Go context: required context propagation through any future enqueue/drain/database path and
  rejected `context.Background()` as proof that work survives the parent; surviving work must have an
  explicit durable owner rather than an accidental detached goroutine.
- Go how-to: routed the design fork to the concurrency/context skills and to external prior art before
  proposing implementation; no implementation skill was selected because the mechanism is unresolved.

## Evidence boundary

Observed directly on the exact base: `tagpublish.go` uses `exec.Command`, `Start`, and
`Process.Release`; `cmd_tag.go` emits “publication queued” only after the authoritative write and
before publication completes; `bg_ingest.go` uses a best-effort `Wait` goroutine; vector top-up and
autosync release detached children. Historical immutable evidence `987c6a3`, `0b39b82`, and `1ddf6ba`
shows detached terminal incompleteness. `4ac774a4` is malformed and not current-base failure evidence;
`d918706` is rejected for losing acknowledged consolidated writes; `48661f4` is evaluated independently.

No test, benchmark, score, adoption, Direction Lock, or merge authorization is claimed by this report.
