# PR #81 Command Timing Reproduction Report (Round 2)

- **Worker:** `lenny-pr81-timing-r2`
- **Date:** 2026-08-28T07:14:45Z
- **Candidate Commit:** `8323fd9f69c06669f0ad529686008b775e783052`
- **Main Commit:** `719243b6005153c99fef571176c7e6dd6e3a2876`
- **Target Session ID:** `87783881-b4b8-4694-8095-12c180e13643`
- **Verdict:** **ACCEPT (Narrowed: Semantic Correctness & Modest Frozen-Fixture Latency Improvement)**

---

## 1. Executive Summary

An independent product-level timing and correctness reproduction was conducted to verify PR #81 (`8323fd9f69c06669f0ad529686008b775e783052`) against public `main` (`719243b6005153c99fef571176c7e6dd6e3a2876`).

The evaluation tested the exact literal product command under a frozen HOME snapshot:
```bash
HOME=/tmp/frozen-home-lenny-pr81-timing-r2 RAWCLAW_BACKGROUND_INGEST=off /usr/bin/time -l <binary> --resume 87783881-b4b8-4694-8095-12c180e13643
```

**Narrowed Findings & Scope:**
1. **Frozen-Fixture Timing:** In the controlled frozen snapshot, Candidate executes in **8.8ms – 22.8ms** (mean **13.67ms**, median **9.36ms**) compared to Main at **24.2ms – 26.3ms** (mean **24.99ms**, median **24.52ms**). This represents a modest speedup of **~1.83x (mean)** and **~2.62x (median)**.
2. **Memory Footprint:** Peak RSS is comparable across both binaries in the frozen snapshot: Candidate at **18,432,000 – 18,530,304 bytes** (~17.6 MiB) versus Main at **17,776,640 – 18,300,928 bytes** (~17.2 MiB). There is **no memory reduction or unbounded allocation benefit** in the frozen benchmark.
3. **Semantic Correctness:** Candidate correctly resolves and identifies the retained non-local session fixture via durable metadata (`Session 87783881-b4b8-4694-8095-12c180e13643 is known, but RawClaw cannot produce a safe local resume command for it.`). Main fails to inspect durable metadata, emitting a negative search notice (`No session id starts with...`).
4. **Ineligible Exploratory Data:** Prior unconstrained live-HOME probes (where Main took 3.75s – 5.10s) are exploratory and ineligible for the frozen timing comparison; they cannot be used to claim asymptotic scaling benefits.

Verdict is **ACCEPTED** narrowly for semantic correctness and modest latency reduction on the retained fixture.

---

## 2. Build Information

| Artifact | Commit SHA | Build Command | Binary SHA-256 |
| :--- | :--- | :--- | :--- |
| **Candidate** | `8323fd9f69c06669f0ad529686008b775e783052` | `CGO_ENABLED=0 go build -o /tmp/rawclaw-candidate-8323fd9 ./cmd/rawclaw` | `3796e005984f29aead252fad792dde3123f94dedaed4fcd9a53b25614b824f10` |
| **Main** | `719243b6005153c99fef571176c7e6dd6e3a2876` | `(git archive 719243b... \| tar -x) && CGO_ENABLED=0 go build -o /tmp/rawclaw-main-719243b ./cmd/rawclaw` | `9d333cdab6874ebcca5763c2372d7c226434d99da4e31129d777167d1a70ae61` |

---

## 3. Frozen HOME Snapshot & Fixture Manifest

- **Frozen Snapshot Path:** `/tmp/frozen-home-lenny-pr81-timing-r2`
- **Included Trees:**
  - `.local/share/rawclaw/` (durable catalog and transcript store)
  - `.claude/` (claude project metadata)
  - `.config/goose/` (goose config)
- **Omission Rationale:** `.codex/` (~4.6 GiB) was omitted to bound snapshot creation time and disk footprint.
- **Results Artifact:** `frozen_home_results.json` (SHA-256: `2b2c624c6d8c5b153d64024eda03a2c629cf23b92c20211410b282de0b6887b2`)

### Fixture SHA-256 Manifest
Location: `/tmp/frozen-home-lenny-pr81-timing-r2/.local/share/rawclaw/transcripts/`

| Fixture File | Size (bytes) | SHA-256 |
| :--- | :--- | :--- |
| `87783881-b4b8-4694-8095-12c180e13643.jsonl` | 57,335 | `368b424ba8aa9b9a9fc835228b7612fb722cee520600de2766a7ecdbccc17b0c` |
| `87783881-b4b8-4694-8095-12c180e13643.meta.json` | 453 | `5e0f4c677887c840daf4470d9bf0007a5882cd7ee1fb5a597f2cc0c8f115d09b` |
| `87783881.../agent-a90deb5ef116a8324.jsonl` | 16,260 | `6c84e7d012687b1ce8628f919d767701ea178c01c1c3b316b20d35fd365cb7c1` |
| `87783881.../agent-a90deb5ef116a8324.meta.json` | 582 | `d7cd9fa79dab4c2844aa35463a8a922772822ae2b47f9d0064da6e418a4ee7f2` |
| `87783881.../agent-ab4cc116847ba4562.jsonl` | 14,951 | `394c983eb01e33e077991e784ab720157d43beb7b700fdd5c1535f57433f9d92` |
| `87783881.../agent-ab4cc116847ba4562.meta.json` | 580 | `f6341216bf07e7c36e1269b86b9af00ae245306bb6778879acee4f0489afeb14` |

---

## 4. Counted Interleaved Run Table (Frozen HOME)

All runs were executed serially with `RAWCLAW_BACKGROUND_INGEST=off` and `HOME=/tmp/frozen-home-lenny-pr81-timing-r2`. Peak RSS was measured via macOS `/usr/bin/time -l` (maximum resident set size in bytes).

| Run | Target / Binary | Exit Code | Wall Time (s) | `/usr/bin/time` Real (s) | Peak RSS (Bytes) | Peak RSS (MiB) | Stdout Bytes | Content Class |
| :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :--- |
| **1** | Candidate (`8323fd9`) | 0 | 0.0094 | 0.00 | 18,464,768 | 17.61 | 118 | Retained Non-Local Explanation |
| **1** | Main (`719243b`) | 0 | 0.0263 | 0.01 | 18,137,088 | 17.30 | 135 | Negative ID Match Notice |
| **2** | Candidate (`8323fd9`) | 0 | 0.0088 | 0.00 | 18,530,304 | 17.67 | 118 | Retained Non-Local Explanation |
| **2** | Main (`719243b`) | 0 | 0.0242 | 0.01 | 18,300,928 | 17.45 | 135 | Negative ID Match Notice |
| **3** | Candidate (`8323fd9`) | 0 | 0.0228 | 0.01 | 18,432,000 | 17.58 | 118 | Retained Non-Local Explanation |
| **3** | Main (`719243b`) | 0 | 0.0245 | 0.01 | 17,776,640 | 16.95 | 135 | Negative ID Match Notice |

### Statistical Summary (Frozen HOME)
- **Candidate:** Mean: `13.67 ms` | Median: `9.36 ms` | RSS Mean: `18.48 MiB`
- **Main:** Mean: `24.99 ms` | Median: `24.52 ms` | RSS Mean: `17.23 MiB`
- **Speedup Factor:** Mean: `1.83x` | Median: `2.62x`

---

## 5. Output Verification & Semantic Comparison

### Candidate Stdout (118 bytes)
```text
Session 87783881-b4b8-4694-8095-12c180e13643 is known, but RawClaw cannot produce a safe local resume command for it.
```
- **Analysis:** Candidate's `resumeExactMetadata` parses full 36-character UUIDs and consults local consolidated/durable metadata. It recognizes that `87783881-b4b8-4694-8095-12c180e13643` is a retained non-local claude transcript (`only_copy_since` set), immediately returning a safe non-local explanation without triggering unnecessary scope discovery.

### Main Stdout (135 bytes)
```text
No session id starts with '87783881-b4b8-4694-8095-12c180e13643'. Use the 8-char id from search output, e.g. [… · a1b2c3d4 · …].
```
- **Analysis:** Main lacks direct metadata inspection for exact IDs, falling back to prefix discovery across candidate adapter scopes. Because the session is archived/retained rather than active in a live project directory, main fails to find it and outputs an erroneous negative result.

---

## 6. Environmental Notes & Gate Compliance

- **OS / Hardware:** macOS Darwin (arm64, Apple Silicon M4).
- **RSS Units:** Reported in bytes directly by `/usr/bin/time -l` on Darwin.
- **Process Isolation:** `RAWCLAW_BACKGROUND_INGEST=off` prevented background indexing workers.
- **File Fence:** Only `docs/notes/lenny-pr81-command-timing-r2.md` is modified/committed.
