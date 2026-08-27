# Tick 41 live problem census and patch-dedup referee

## Run boundary

- `run_timestamp`: `2026-08-27T02:01:33Z` (original census; no future receipt accepted).
- Required current base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
- Worktree: `worker/furiosa-t41-live-census-20260827`, isolated from the shared checkout.
- Report-only. No product, graph, scorecard, cursor, or mailbox was changed. The prior-art ledger
  was subsequently read without touching mailboxes: its last recorded valid watermark is
  `20260827T013550Z` (Tick 39 preliminary, ledger lines 844-854; ledger SHA-256 at read
  `68f74f27f700d0bc266910138878a9494c284dd220342e15f1a3157d47504e9d`). The supervisor-confirmed
  processed prefix advances this census to `20260827T020554Z` (Tick 42); no later valid receipt is
  claimed.

## Verdict

No new adoption, score, or Direction Lock invalidation is proven. Preserve totals **Furiosa +9,
Han +2, Ozzy +3**. No merge authorization. Tick 35 cursor actor remains **UNATTRIBUTED / DO NOT
GUESS**. The only score-bearing sidecar-prune event remains the locked Ozzy `c38f79a` receipt;
Furiosa `0cd00e4` and other same-effect variants do not create another event.

## Candidate census and identity

All apply checks below were personally run as `git diff ef2eebf..<sha> | git apply --check` and
returned `PASS`. Parent and direct payload are commit-scoped; stacked ancestry is not credited as
payload. Whole and path-stable patch IDs were recomputed with `git patch-id --stable`.

| candidate (provenance) | exact parent | direct payload (production / test / docs) | whole ID; path IDs | ruling |
|---|---|---|---|---|
| `c38f79a` (Ozzy locked sidecar-prune) | `96aa522` | `+20/-20` / `+48` / `0` | `6a62ff59`; Go `41b270da`, test `29b08e7f` | **CONFIRMED narrow; already Ozzy +3; duplicate** |
| `0cd00e4` (Furiosa adaptation) | `878f631` | `+20/-0` / `+55` / `0` | `57bdcd67`; Go `ab5ee7d6`, test `ac5ee690` | **CONFIRMED narrow; same effect family; no score** |
| `a78b39b` (rejected duplicate) | `9068aff` | `+20/-0` / `+50` / `0` | `b47c5d83`; Go `ac2dbbbf`, test `084fa2c3` | **duplicate; no score** |
| `96aa522` (rejected duplicate) | `fb99037` | `+20/-0` / `+51` / `0` | `d54fa759`; Go `ac2dbbbf`, test `2922ec99` | **duplicate; no score** |
| `a62ab05` (rejected duplicate) | `9068aff` | `+20/-0` / `+51` / `0` | `b0c65fc6`; Go `ac2dbbbf`, test `0499a406` | **duplicate; no score** |
| `cabab43` (Han overlay) | `9a1b53c` | `+37/-1` / `0` / `0` | `72d417eb` (same path) | **narrow committed-topic visibility; readiness uncertain; no adoption** |
| `d2315cb` (Han fixture isolation) | `7edd58d` | `0` / `+5` / `0` | `17db9874` (same path) | **fixture only; no product score** |
| `8e9c9b7` (Han integration) | `4119698` | `+12/-5` / `+88` / `0` | `4aef91de`; Go `044d7551`, test `46a21c8d` | **narrow tests confirmed; full readiness uncertain; no adoption** |
| `386ec9d` (Furiosa/Ozzy benchmark) | `0d1da19` | `+35/-36` / `+77` / `0` | `356c1cb3`; Go `6b42e87e`, bench `ca293434` | **functional path narrow; speed claim unsupported/uncertain; no score** |

The refs are not interchangeable: `c38f79a` is Ozzy provenance, while `0cd00e4` is a Furiosa
adaptation. `a78b39b`, `96aa522`, and `a62ab05` are one rejected sidecar-prune effect family,
despite distinct SHAs and tests. `386ec9d` is not Han work merely because it is visible in
`git log --all`. The Han `cabab43`/`d2315cb`/`8e9c9b7` chain is a different overlay/publication
family and does not supersede the locked sidecar contract.

## Tick 40 receipts and score ruling

- Han worker: `a260f69ffa704a9be39fca5de08b465d086f1f4b`, report
  `d0616359290141b6b75a258db2205e36a32b9c75f91d3824b0c48bcfbcfaedf5`.
  A worker-supplied full SHA was wrong; Git-proven SHA/report hash above is authoritative. The
  claim remains narrow/uncertain and has no external adopter receipt: **Han +0**.
- Ozzy worker: `27c306e7fbf0e0c6d24054e0b322a93b12dac644`, report
  `06f9bc4d3dae1e3ec1e1615c8eae91abe5e0f41d74805eaea3501f60ce3860a3`.
  Functional prune evidence does not prove the speed claim without paired baseline, benchstat,
  and a non-zero work oracle: **Ozzy +0**.
- Score referee: `77248b7249060c153462ccf3cbb5be67b2ac7e9d`, report
  `deb9ed764ac8dc0f2c0e4651ed523f079a0d8338fcd2c103b9a64a1f05798f93`; no score change.
- `prior_watermark`: `20260827T013550Z` (last entry actually present in the append-only ledger).
- `new_watermark`: `20260827T020554Z` (supervisor-confirmed Tick 42 processed prefix). This is
  reconciled as a monotonic advance from the ledger value; no adoption or score event was found.

## Direction Lock

`PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains **technical LOCKED, not merge authorization**. The
reusable schema is: stable recommendation fingerprint; exact base; candidate SHAs and parents;
whole/path patch IDs; payload-vs-ancestry line counts; exact focused selector and full gate;
immutable adopter receipt and recipient acknowledgement; mutation strength; invalidation triggers;
and monotonic valid watermark. Current status is complete for the existing locked technical
direction, but no new candidate supplies the missing independent adoption/current-base full-gate
receipt. `PA-DIRECTION-LOCK-001` remains partial for this reason.

## Next leads (at most three)

1. Obtain one independent current-base adopter receipt for `8e9c9b7`-class overlay/publication,
   including detached-child survival, fence/transaction/watermark cancellation, mutation reds,
   and the full race gate.
2. Re-run `386ec9d` against an identical parent fixture with paired samples, `benchstat`, query
   plans, and a non-zero rows-examined/deleted/committed oracle; otherwise reject the speed claim.
3. Keep future regrades append-only: start at `20260827T020554Z` and accept only conforming,
   processed receipts no later than the fresh completion timestamp.

## Completion contract

Only this report is owned. No Go files changed; `gofmt -w` is **N/A**. The initial completion was
blocked by the supervisor mailbox guard; this correction reads the ledger without mailbox access.
Commit/push this report, then capture a fresh `run_completion_utc` with `date -u`, verify clean
worktree and upstream `0/0`, and record its SHA-256. This report grants no merge authorization.

## Tick 42 ledger reconciliation correction

- `prior_watermark`: `20260827T013550Z`, verified from the ledger's final recorded `new_watermark`.
- `processed_watermark`: `20260827T020554Z`, supplied by the supervisor after Tick 42 processing.
- `new_watermark`: `20260827T020554Z`; monotonic and later than the ledger value, with no future
  receipt accepted and no mailbox read or mutation performed.
- Status and scores are unchanged: sidecar-prune remains technically locked only; totals remain
  Furiosa `+9`, Han `+2`, Ozzy `+3`; no score or merge authorization.
- `run_completion_utc`: `2026-08-27T02:12:20Z`, captured with `date -u` after the ledger
  reconciliation edit and final pre-commit validation.
