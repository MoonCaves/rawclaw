// Package redact is a deterministic, LLM-free secret scrubber for transcript
// text. It detects API keys, tokens, private keys, basic-auth credentials, and
// secret-shaped assignments, replacing each match with a stable placeholder.
//
// STAGED — NOT WIRED INTO THE DEFAULT PATH. This package exists for a
// post-public commit (issue #7: tool results are indexed verbatim, so secrets
// can be returned). It is intentionally self-contained and is NOT called from
// the index or read path yet. Wiring it in is a separate, deliberate change.
//
// Design (adopted from prior art): a cheap substring prefilter gates
// the expensive regex pass — for the common case of clean prose the scrubber
// does near-zero work and returns the input unchanged. Each detector is a named
// regex; a match is replaced by a [REDACTED:<name>] placeholder. The scrubber
// errs toward NOT redacting ordinary prose: bare mentions of "token"/"key" do
// not match — only key-shaped values and explicit secret assignments do.
//
// Pure Go, stdlib only (regexp + strings). No network, no allocations on the
// clean path beyond the returned string.
package redact

import (
	"regexp"
	"strings"
)

// Placeholder is the literal substituted for the body of a basic-auth or
// url-credential match where only the credential (not the surrounding
// structure) is replaced. Full-token detectors use [REDACTED:<name>] instead.
const Placeholder = "[REDACTED]"

// detector pairs a named regex with the lowercase substrings that must be
// present for the regex to have any chance of matching. The prefilter lets us
// skip the regex entirely when none of its anchors appear. An empty anchors
// slice means "no cheap prefilter" — the regex runs whenever any detector with
// anchors fired, or unconditionally if it is the only matchless detector (see
// scan).
type detector struct {
	name    string
	re      *regexp.Regexp
	anchors []string // lowercased; ANY present → run the regex
	// repl, when non-nil, computes the replacement for a single match. When
	// nil, the whole match is replaced by "[REDACTED:" + name + "]".
	repl func(match string) string
}

// labeled returns the standard "[REDACTED:<name>]" placeholder for a detector.
func labeled(name string) string { return "[REDACTED:" + name + "]" }

// detectors is the ordered detector set. Order matters: more specific patterns
// (private-key blocks, vendor-prefixed keys) run before broad ones (generic
// secret assignments) so a value is labeled with the most precise name.
//
// Patterns are deliberately conservative about length and surrounding context
// to avoid redacting ordinary prose. They are NOT exhaustive — this is a
// best-effort scrubber, documented as such, not a guarantee.
var detectors = buildDetectors()

func buildDetectors() []detector {
	return []detector{
		// PEM private-key blocks: the whole armored block, any key type.
		{
			name:    "private_key",
			anchors: []string{"-----begin"},
			re: regexp.MustCompile(
				`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
		},
		// Anthropic keys: sk-ant-... (check before the generic sk- key).
		{
			name:    "anthropic_key",
			anchors: []string{"sk-ant-"},
			re:      regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`),
		},
		// OpenAI-style keys: sk-... (and sk-proj-...). Require a long body so a
		// hyphenated word like "sk-something-short" cannot match.
		{
			name:    "openai_key",
			anchors: []string{"sk-"},
			re:      regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9]{20,}`),
		},
		// AWS access key ids: AKIA / ASIA + 16 uppercase alphanumerics.
		{
			name:    "aws_access_key",
			anchors: []string{"akia", "asia"},
			re:      regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		},
		// GitHub tokens: gh[pousr]_ + base62 body.
		{
			name:    "github_token",
			anchors: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"},
			re:      regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
		},
		// GitHub fine-grained PATs: github_pat_ + body.
		{
			name:    "github_pat",
			anchors: []string{"github_pat_"},
			re:      regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`),
		},
		// npm tokens: npm_ + 36 base62.
		{
			name:    "npm_token",
			anchors: []string{"npm_"},
			re:      regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`),
		},
		// Slack tokens: xoxb-/xoxa-/xoxp-/xoxr-/xoxs- + dotted/hyphenated body.
		{
			name:    "slack_token",
			anchors: []string{"xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-"},
			re:      regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
		},
		// JWTs: three base64url segments separated by dots, starting eyJ
		// (the base64url of `{"`, i.e. a JSON header).
		{
			name:    "jwt",
			anchors: []string{"eyj"},
			re:      regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		},
		// Bearer tokens in an Authorization-style context: replace only the
		// credential, keep the "Bearer " keyword so the structure is visible.
		{
			name:    "bearer",
			anchors: []string{"bearer "},
			re:      regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`),
			repl: func(m string) string {
				// Preserve the literal keyword + its trailing space.
				kw := m[:len("bearer")]
				rest := m[len("bearer"):]
				lead := rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
				return kw + lead + Placeholder
			},
		},
		// Basic-auth header: replace the base64 credential, keep "Basic ".
		{
			name:    "basic_auth",
			anchors: []string{"basic "},
			re:      regexp.MustCompile(`(?i)\bbasic\s+[A-Za-z0-9+/]{16,}={0,2}`),
			repl: func(m string) string {
				kw := m[:len("basic")]
				rest := m[len("basic"):]
				lead := rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
				return kw + lead + Placeholder
			},
		},
		// Credentials embedded in a URL: scheme://user:pass@host. Replace only
		// the user:pass run, keep scheme and host. Password must be non-empty.
		{
			name:    "url_credentials",
			anchors: []string{"://"},
			re:      regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s:/@]+:[^\s:/@]+@`),
			repl: func(m string) string {
				// m is "<scheme>://user:pass@"; keep "<scheme>://" and "@".
				i := strings.Index(m, "://")
				scheme := m[:i+len("://")]
				return scheme + Placeholder + "@"
			},
		},
		// Generic secret assignments: key/secret/password/token = "value".
		// Require an assignment operator and a value of meaningful length so a
		// bare "the api key" sentence does not match. Keep the key name and the
		// operator; replace the value.
		{
			name:    "secret_assignment",
			anchors: []string{"key", "secret", "token", "password", "passwd", "pwd", "api_key", "apikey", "auth"},
			re:      reSecretKV,
			repl: func(m string) string {
				// Recompute the key + operator prefix to preserve it.
				loc := reSecretKV.FindStringSubmatchIndex(m)
				if loc == nil || loc[4] < 0 {
					return labeled("secret_assignment")
				}
				// loc[4]:loc[5] is the value group. Keep everything before it,
				// minus any opening quote, then emit the placeholder.
				prefix := m[:loc[4]]
				prefix = strings.TrimRight(prefix, `"'`)
				return prefix + Placeholder
			},
		},
	}
}

// reSecretKV is a standalone compile of the secret-assignment pattern, used by
// its repl to locate the value group within a single match.
var reSecretKV = regexp.MustCompile(
	`(?i)\b([a-z0-9_.-]*(?:api[_-]?key|secret|password|passwd|pwd|token|auth[_-]?token|access[_-]?token|client[_-]?secret)[a-z0-9_.-]*)["']?\s*[:=]\s*["']?([A-Za-z0-9_./+=~-]{8,})["']?`)

// Result reports the outcome of a Scrub: the cleaned text and which detectors
// fired (deduplicated, in detector order). Hits is empty when nothing matched.
type Result struct {
	Text string
	Hits []string
}

// Changed reports whether any redaction was applied.
func (r Result) Changed() bool { return len(r.Hits) > 0 }

// Scrub returns s with every detected secret replaced by a stable placeholder.
// It is always safe to call on non-secret text: clean prose is returned
// unchanged, and ordinary mentions of "token"/"key" do not match. The returned
// string is deterministic for a given input and detector set.
func Scrub(s string) string {
	return ScrubResult(s).Text
}

// ScrubResult is Scrub with detector-hit reporting. The clean path (no prefilter
// anchor present) returns s and a nil Hits slice without running any regex.
func ScrubResult(s string) Result {
	if s == "" {
		return Result{Text: s}
	}
	lower := strings.ToLower(s)
	out := s
	var hits []string
	for i := range detectors {
		d := &detectors[i]
		if !prefilter(lower, d.anchors) {
			continue
		}
		var fired bool
		if d.repl != nil {
			out = d.re.ReplaceAllStringFunc(out, func(m string) string {
				fired = true
				return d.repl(m)
			})
		} else {
			repl := labeled(d.name)
			replaced := d.re.ReplaceAllString(out, repl)
			if replaced != out {
				fired = true
				out = replaced
			}
		}
		if fired {
			hits = append(hits, d.name)
		}
		// Re-lower only if the text changed and later detectors share anchors
		// with the replacement text — placeholders are lowercase-stable
		// ("[redacted:...]") and contain none of our anchor tokens, so a
		// refresh is unnecessary. We keep the original `lower` for prefilter.
	}
	return Result{Text: out, Hits: hits}
}

// prefilter reports whether the regex for a detector should run: true when the
// detector has no anchors (always run) or when any anchor substring is present
// in the lowercased haystack. This is the cheap gate that keeps clean text fast.
func prefilter(lower string, anchors []string) bool {
	if len(anchors) == 0 {
		return true
	}
	for _, a := range anchors {
		if strings.Contains(lower, a) {
			return true
		}
	}
	return false
}
