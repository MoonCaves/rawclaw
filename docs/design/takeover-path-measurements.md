# Session Takeover Path Measurements (2026-09-09)

Measurements supporting session takeover and bounded read path hardening:

## 1. D1: All-digit hex uuid8 refs
- In `internal/agentproto/types.go`, the check `reNumericRef = regexp.MustCompile("^[0-9]+$")` was rejecting valid hex UUID prefixes.
- Measurement across corpus: 17,649 / 756,602 messages (2.33%) have an all-digit uuid8.
- Dropping `reNumericRef` eliminates this 2.33% false rejection rate (Doctrine Rule 4: Losers get deleted).

## 2. D2: System prompt anchor capping
- In `internal/view/view.go`, reading an anchor whole (`cap = -1`) causes massive context dumps when the anchor is a `system` message.
- Measurement across corpus: 134 topic segments anchor on a system runtime injection.
- Largest observed system prompt anchor is 41,669 characters in session 01a07ab9.
- Policy: cap non-conversational `system` anchors to `dispCap` (1,000 characters) unless `--with tools` explicitly requests raw tool/system data.

## 3. D3: Single-session topic spine retrieval
- In `internal/cli/cmd_topics.go`, adding `--session <id>` enables reading a single session's chronological topic spine with start refs and summaries.
- Avoids multi-turn search roundtrips when taking over a session.
