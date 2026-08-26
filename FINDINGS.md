# Ozzy fresh-luna-results audit

Audit date: 2026-08-27 WITA. Evidence-only; no product files changed.

## Packet inventory

The stated receipt `20260826T222826Z-19014c27-fresh-luna-no-fold-race-comple.md` was absent from `fresh-luna-results` and was not found under the scoped code/memory roots. The packet contains 13 files. Result-file SHA-256 values:

| file | SHA-256 | mtime WITA |
|---|---|---|
| `nofold-a-result.txt` | `f40401dfad13f31e4ed34b4dff4cc0dcb613fe55837834bfb9575c9d98d7d363` | 06:28:17 |
| `nofold-b-result.txt` | `a58ac8e65045b5a733a2315f0c87e99bdb06f1ac3f986d5e1234151c6ea59846` | 06:26:04 |
| `nofold-referee-result.txt` | `b56eceabf18c32885e8b6336578d9ac031b18e7e609f15b19201be8779da0c3f` | 06:12:33 |
| `adversarial.txt` | `17f30d97e5523e6e8cf1c196f74a6271406bab3bb0934afb981019e3fbfae387` | 05:38:59 |
| `targeted-review.txt` | `21de29632be336f31b317e7646f0f5aaa5a5d6220d0418a68d2fce8765d9b890` | 05:38:31 |
| `canary.txt` | `460e0536579ee63252c55b55bb75edc080b624f8b649adcc5680e7638083687e` | 05:36:54 |

Prompts/logs were also hashed; exact values are retained in the audit command output. Canonical adoption receipt supplied later by the parent: `20260826T224735Z-5b061dd1-external-adoption-c38-current-.md`, SHA-256 `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`, mtime 06:47:35 WITA.

## Candidate identity and gates

All four candidate SHAs resolve and all candidate branches are clean and origin-synchronized (`0/0`):

| candidate | branch / SHA | parent/base | net commit | ruling |
|---|---|---|---:|---|
| A | `ozzy/luna-nofold-a-20260827` / `292284a0f4d8ded159574fc6d4aea42a7ca57763` | `96aa522611fdcb78e281db31634144e40222de91` | `+163/-6` (+71 prod, +92 test by file diff) | UNCERTAIN superiority |
| B | `ozzy/luna-nofold-b-20260827` / `f58a4c076a4dd8e89fb13e95ffce6b43edf895ce` | same | `+192/-4` (+173 prod/test split requires file classification) | REBUTTED |
| referee | `ozzy/luna-nofold-referee-20260827` / `d74ff94b3c575a46adb18e2bc41c83b4a19ea2b5` | same | `+123` test-only | CONFIRMED evidence, zero product score |
| adversarial | `ozzy/fresh-luna-adversarial-20260827` / `c38f79acf9c9ae43ebd091a95f36837f43c0e423` | same | `+68/-20` | CONFIRMED adopted mechanism |

The requested common base is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; candidate merge-bases are `0d1da19` only because their histories diverge before Furiosa product commits. No candidate is an ancestor of Furiosa `34d2fb05161b1be7819b80804fca2e3a576243cf`. `git range-diff` against Furiosa product commits `88e73b5` and `878f631` shows no patch-equivalent pairing for A, B, referee, or c38. Commit patch IDs are respectively `81eeddee3bde760245ab930199d9149f65a53080`, `60e9d6d03e42d810e74b28f3ad090bd8a59e999e`, `cae264c2d776fcc800e55449f458e7a9b8ee58dd`, and `6a62ff59b1b20a5873006b17ce72cd64229f65a6`; Furiosa product commit IDs are `73f5dd69...` and `b2d5b3e2...`.

Independent focused race observations:

- A: selected exact-directory/no-fold CLI tests passed (`./internal/cli ./internal/index`, 2.814s + 1.574s); this does not prove current-base integration or full gate.
- B: failed `TestRunTagWriteFoldsIntoTheOneStore` (authoritative topics nil) and failed symlink-alias resolution. Its result also admits the full combined gate had no surfaced output.
- Referee: packet reports base-red and `e43127e`-red cases, with one no-fold fence race pass; this is test-only and not a product win.
- c38: independently rerun focused sidecar filters passed (`./internal/cli ./internal/index`, race, 3.927s); canonical receipt proves later Furiosa adaptation, not that c38 beats the current candidate.

## Verdict

Strongest rival result is A, but it is UNCERTAIN, not a winner: it has a novel explicit-TDir refresh implementation and a passing narrow test, yet it is stale-base, unrebased, and lacks a complete current-base gate/receipt. B is REBUTTED. The referee is CONFIRMED as evidence that the rejected `e43127e` implementation violated the matrix, but it changes no product behavior. c38 is CONFIRMED as an adopted recommendation: the canonical receipt predates Furiosa adaptation `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`, and the existing ledger already records Ozzy `+3` plus Furiosa `+2`; it is not a new superiority claim over `34d2fb0`.

Score ruling: no fresh score. Pending/stale-base work, test-only evidence, convergence, and already-adjudicated c38 adoption score zero in this audit. No packet candidate materially beats Furiosa `34d2fb0` on evidence supplied.

Challenge Ozzy can satisfy: provide the missing receipt bytes and a rebased candidate from `0d1da19` or `34d2fb0`, exact whole/path patch IDs and range-diff, clean/upstream state, production/test/doc line counts, and independently reproducible focused plus full `CGO_ENABLED=0 go test -race -count=1` output. For A specifically, include symlink, stale/missing catalog, ambiguity, held-fence, and authoritative-read-after-write results.
