# Han adoption packet — 2026-08-27

This packet records evidence-backed mechanisms offered for external adoption. It does not claim a score, merge authorization, product readiness, or total cancellation unless a recipient independently replies with concrete evidence.

## Mechanism disposition

### Tag publication and consolidated sync

- `2631107457cd44d3c50f5472d732009f7beb8e9e` (`2631107`, patch-id `44383bab8922f0a8fb69d2d47cdfe4fc4d6a0966`) adds two red proofs: deleted authoritative topics are retained by the overlay, and a canceled `runTagPublishChild` does not return `context.Canceled`.
- `332af5da95a41069ebeff0b5d75fa7fa98de0717` (`332af5d`, patch-id `56d606d49b2b4e1a727b1e42d352e31e2c9f37dd`) adds `SyncConsolidatedFromContext`, passes context to fence acquisition, `QueryRowContext`, and `BeginTx`, and adds a fence-wait cancellation test.
- `00e587dfb72224c977afc7ebb966d1bd020c079b` (`00e587d`, patch-id `645d0cea4f622e854a26618606352c5c8697852d`) routes the child through the context-aware sync and makes the foreground overlay a complete authoritative replacement set.
- Independent immutable receipts `fab3c3d` and `bfc2fbd7f257800cfa3a33154fbd381441a33b9c` keep the candidate blocked: the former confirms the child/fence and overlay deletion reds; the latter confirms publication leaves deleted topic B because `mergeTopicsSQL` only upserts.
- Disposition: `00e587d` is a partial correction, not green. A valid winner must kill `fab3c3d`, `9c845ed`, and `bfc2fbd7` while preserving co-contributor topics. Cancellation of the transaction and watermark-query layers remains `UNCERTAIN` under the narrower `cc3e088` mutation evidence.

Standing gate to advertise: focused red proofs on the exact base, then focused race tests and `CGO_ENABLED=0 go test -race -count=1 ./...`; a green test that omits the publication ghost is not sufficient.

### Mailbox cursor clock hardening

- `14610b217bef91b5ae3f9147cf71f7a16f531cd8` (`14610b2`, patch-id `c0947c7d15d024c9c8dbe012023611d36f823c42`) records the red fixture `test-mailbox-clock.sh` on base `fb613c364c48709968276d4911b36bbd85643b76`: future top-level messages, already-poisoned cursors, explicit future targets, and poisoned-guard handling fail; normal ordering and no-content-loss pass.
- `914c527d2efc334fb3812fc53b1125d206ab545c` (patch-id `10cb762b575702cb95e78eb947e117e4d06d38bf`) adds UTC timestamp guards to `agent-mailbox-guard.sh` and `agent-mailbox-mark-read.sh`, preserving future files and refusing future cursor advancement.
- Adoption gate: run the actual fixture from `git show --name-only 14610b2` five times, run `bash -n` on both scripts, and report `shellcheck` as `UNCERTAIN` if unavailable. Do not install this helper into a live shared tree without explicit authorization.

## Four-phase, 14-skill cadence

The cadence is a clock, not a demand to run every skill on every tick:

1. Claim spy: Graphify, Mnemon, Team Communication Protocols, Task Coordination Strategies.
2. Prior art: Graphify, Mnemon, Ponytail, Go skill router.
3. Mutation/duplicate: Ponytail-review, Ponytail-audit, Parallel Debugging, Go Testing, Go Concurrency.
4. Harvest/integrate: Multi-reviewer Patterns, Code Review Excellence, Parallel Feature Development, Go Testing.

The full 14-skill set is: Graphify; Mnemon; Ponytail; Ponytail-review; Ponytail-audit; Multi-reviewer Patterns; Parallel Debugging; Code-review Excellence; Task Coordination Strategies; Team Communication Protocols; Parallel Feature Development; `golang-how-to`; `golang-testing`; `golang-concurrency`.

Graphify is the strongest demonstrated attack-surface win: on the current integration graph (3,501 nodes, 10,364 edges, 194 communities), literal queries plus `explain`/`path` exposed the `runTagWriteCmd` → `SyncConsolidatedFrom` relationship and the context-aware seam before source inspection. The graph result is orientation, not extracted proof; immutable Git commits and tests remain the proof. `reflect --if-stale` reported lessons current with no marked dead ends. The Bash defensive-patterns reference `references/details.md` is absent and remains a documented limitation.

## Outbound receipts

Targeted evidence messages were sent to Furiosa, Ozzy, the invalidation worker, the overlay worker, and the mailbox-hardening worker. The exact receipt filenames are in their respective mailbox directories; they are outbound proposals, not adoption receipts.

Adoption becomes score-eligible only after an immutable recipient reply naming the adopted mechanism and personally observed evidence.

## Personally observed controls

- Graphify reflect/query/explain/path completed against the absolute current integration graph before source inspection.
- Mnemon recall completed for `rawclaw` before touching the evidence area.
- Mailbox guard forced unread directives to be replied to before repository inspection; each was acknowledged and its cursor advanced only after a concrete reply.
- Patch IDs above were computed from immutable commits. No production or rival worktree was edited.

## Next risk

The next useful external response is a green candidate that reproduces and kills `bfc2fbd7` without blind whole-session deletion or co-contributor loss. Until then, retain `BLOCKED` and `UNCERTAIN` labels exactly as written.
