# Judge calibration protocol (draft, 2026-09-08)

Status: proposal. Nothing here is applied yet. Jay asked for ten harness changes per recon finding plus an explicit thinking-level treatment. RubyHeron's fleet-doctrine msg 542 (25 controls) overlaps; items marked (RH) originate there.

## Facts established today that the protocol rests on

- **Temperature on Claude 4.7+/Sonnet 5/Fable/Opus 5:** Anthropic returns 400 `temperature is deprecated for this model` for any non-default value. Verified direct to sub2api on fable-5-1 and opus-5 (request ids req_011CepyJtQis9vuCDawSXZcf, req_011CepyKE9sNJPkfwPNXN843). LiteLLM source `litellm/llms/anthropic/common_utils.py` L281–306 (`_supports_sampling_params`) auto-drops temperature for fable/opus-4-7/opus-4-8; our gateway also has `drop_params: true`. Either way the model never sees it and the gateway returns 200. Anthropic docs (adaptive-thinking, effort pages) say the same. LiteLLM issue #26444 tracks the gap for opus-4-7. **Only `claude-subscription` (= claude-sonnet-4-6) honors temperature 0 on this gateway** (verified 200 direct).
- **Effort / thinking level:** Anthropic: `output_config.effort` ∈ low, medium, high (default), xhigh, max; availability varies per model (sonnet-4-6 rejects xhigh: `effort='xhigh' is not supported`). LiteLLM forwards `reasoning_effort` → effort: opus-5 low → 92 thinking tokens, max → 128 on one prompt. Gemini via cliproxy: all five values return 200; thinking tokens 230–354 across levels on one prompt, no monotonic trend in 6 samples; whether the param overrides the `-high` in the routed model name is unknown. agy CLI: level is part of the model name (`gemini-3.7-flash-{low,medium,high}`), same Google account as cliproxy; agy is timing out on every prompt as of 07:00. Astra: low/medium/high/xhigh/max per estate docs; locked until 2026-09-14 02:38Z.
- **The gate hardcodes `reasoning_effort: "medium"`** for every gateway judge (provenance-gate.sh L279). So every Anthropic judge row in the ledger ran at effort=medium, below Anthropic's default of high. Not disclosed in any earlier table.
- **LiteLLM response cache:** Redis, ttl 600 (config L405–411). Any fixed-prompt "identical N/N" is a cache hit unless `cache: {"no-cache": true}` was sent.

## 1. Sampling parameters (temperature / top_p / drop_params)

1. Capture the exact upstream request body per judge call (sub2api / cliproxy request id) into GATE_DEBUG_DIR alongside the response. (RH 1.1)
2. Calibration runs use a route/key with `drop_params: false` so an unsupported param fails 400 instead of vanishing. (RH 1.3)
3. If a sampling param was sent and the upstream did not honor it, the run is recorded INVALID, never APPROVE/REJECT.
4. A probe script generates `docs/design/model-capabilities.json`: per alias, accepts temperature? effort levels accepted? thinking param accepted? Dated, md5'd, cited.
5. The probe runs before every batch and aborts the batch if capabilities differ from the committed table.
6. The word "temperature 0" appears in a result only when the forwarded request shows it and the response did not error.
7. Models that reject temperature are recorded as `sampling: provider-default` on every row.
8. Seed passthrough tested and recorded per provider (OpenAI supports; Anthropic/Gemini do not).
9. `claude-subscription` (sonnet-4-6) is included as the one Anthropic row where greedy sampling is real, as a control.
10. Every capability claim carries its source: doc URL + access date, LiteLLM file + line + SHA, or our own pasted 400 body.

## 2. Caching and determinism

1. `cache: {"no-cache": true}` on every calibration call; assert no cache-hit header in the response. (RH 2.1)
2. A per-run nonce in the prompt so upstream prompt caches cannot return identical text.
3. Flush the Redis namespace for the calibration key before each batch, or use a key with caching off.
4. Replace "identical N/N" with dispersion: distinct-SHA count and Jaccard similarity of finding sets. (RH 2.3)
5. N ≥ 30 per (judge, diff) before any stability statement.
6. Log `cache_read_input_tokens` to separate LiteLLM response cache from provider prompt cache. (RH 2.5)
7. Run the fixed-prompt and nonce-prompt probes side by side; if only the fixed one is stable, record "cache".
8. Serial execution only; one judge at a time; wall time from response timestamps; host load noted.
9. A recon agent's "verified identical" is not accepted without its cache flag pasted (ReconB's was a cache artifact).
10. Ledger header records LiteLLM version, cache type and ttl, gateway config md5.

## 3. Model identity and lineage

1. Per run: upstream response `model`, `x-litellm-model-name`, `x-litellm-model-group`, provider request id. (RH 3.1)
2. Per run: sub2api usage_logs row id + account_id, or cliproxy request id.
3. Fingerprint battery (10 fixed prompts) at batch start and end; distribution shift flags a silent weight change. (RH 3.2)
4. `docs/design/alias-map.json` regenerated from `/model/info` + configs each batch and diffed; the table in recon out-C.md is the seed.
5. Gemini via cliproxy: capture `usageMetadata.modelVersion` (gotchas.md: the only cross-path equivalence proof).
6. Record the effort ACTUALLY forwarded per provider (Anthropic `output_config.effort`; Gemini the `-high` suffix vs the param).
7. No family names in results. Every row: resolved upstream ID + level + account + transport.
8. Record gateway LiteLLM version and config md5.
9. When comparing agy CLI to gateway, prove same account (OAuth file name) and record it.
10. All of the above as a JSON sidecar per run; prose tables are generated from sidecars, never typed.

## 4. Thinking level as a first-class variable

1. Sweep low / medium / high / xhigh / max for every judge that accepts them; record the accepted set per model from the probe (sonnet-4-6 has no xhigh).
2. The gate stops hardcoding medium: `GATE_EFFORT` in the env file, written into the git note and the ledger row.
3. Anthropic: assert `output_config.effort` was forwarded; record `thinking_tokens` per run from usage.
4. Gemini via cliproxy: settle whether `reasoning_effort` overrides the `-high` model suffix: same prompt, low vs max, ≥10 nonce samples each, compare thinking-token distributions. Today's 6 samples are inconclusive.
5. Gemini low/medium: reachable only via agy CLI today. Fix the agy timeout first (`--debug`, strace) or have the LiteLLM desk add `gemini-3.7-flash-low/-medium` aliases; do not compare across transports until #9 in section 3 is satisfied.
6. Plot MADE-count and total findings against thinking tokens per judge; the x-axis is tokens, not level names.
7. Cost per verdict per level (tokens × list price) in the table.
8. Astra low/medium/high/xhigh/max when it returns; the earlier Astra rows were medium only.
9. Cross-vendor comparison only at matched thinking-token budgets; "high" means different things per vendor. (RH 4.5)
10. Report the vendor's own knob name in every row: Anthropic effort, OpenAI reasoning_effort, Antigravity model-name tier.

## 5. Ground truth and classification

1. Frozen labeled set: ≥5 bad diffs (one defect class each) and ≥5 clean diffs, as commits, with the answer key committed BEFORE any judge runs. (RH 5.1)
2. Per judge per diff: precision, recall, F1, confusion matrix against the key. (RH 5.2)
3. Hypothesis and pass criterion written in this file before the run.
4. Serial, same day, same prompt md5, same gate md5, recorded in every row.
5. A second reader other than the author runs the batch independently; results diffed.
6. Findings normalized (file + line range + decision hash) so "same sites across runs" is computable, not eyeballed.
7. Every cited file:line and upstream_quote in a finding is verified to exist; misses are scored against the judge. (RH 5.4)
8. Blinded de-identified diffs; inter-rater agreement (Fleiss' kappa) across judges. (RH 5.5)
9. Rubric wording varied on a held-out subset; verdict flip rate reported. (RH 5.3)
10. No production judge change from a calibration run until the criterion in #3 is met and the second reader signs.

## Sources

- Anthropic: https://platform.claude.com/docs/en/build-with-claude/adaptive-thinking , https://platform.claude.com/docs/en/build-with-claude/thinking-steering-and-cost (effort table) — accessed 2026-09-08
- LiteLLM: `litellm/llms/anthropic/common_utils.py#L281-L306` @9d0c9b93 (`_supports_sampling_params`); issue https://github.com/BerriAI/litellm/issues/26444
- Field reports of the 400: github.com/icereed/paperless-gpt#1003, altic-dev/FluidVoice#535, sipeed/picoclaw#2939, github/copilot-sdk#1557, zeroclaw-labs/zeroclaw#6095
- Our direct probes: this repo, `docs/design/judge-rejection-ledger-2026-09-08.md`, `docs/design/recon-2026-09-08/`
