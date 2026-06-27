// Package tagger holds the opt-in topic-tagger: an endpoint-agnostic chat client
// that labels WHERE topics were discussed in a session transcript, mirroring the
// adapters.Embedder pattern (env-configured, opt-in, no bundled keys).
//
// Default behavior: RawClaw tags NOTHING with nothing configured here. Tagging is
// opt-in, wired entirely through environment variables so the public CLI bundles
// no keys, no service, no dependency:
//
//	RAWCLAW_TAG_ENDPOINT  full OpenAI-compatible chat/completions URL
//	                      (e.g. a LiteLLM /v1/chat/completions). Empty = disabled.
//	RAWCLAW_TAG_MODEL     model name (default: claude-haiku-4-5)
//	RAWCLAW_TAG_KEY       bearer token; falls back to LITELLM_KEY if empty
//
// The wire is a single OpenAI chat/completions shape:
//
//	POST {endpoint} {"model","temperature":0,"messages":[{system},{user}]}
//	-> {"choices":[{"message":{"content": "<a JSON array of segments>"}}]}
//
// The model is asked for a JSON array; the parser is defensive (see parseSegments)
// because Haiku sometimes wraps the array in a ```json fence and/or trails prose.
package tagger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultTagModel is the model used when RAWCLAW_TAG_MODEL is unset.
const DefaultTagModel = "claude-haiku-4-5"

// defaultTimeout is the HTTP timeout applied to a single tag request (one window).
const defaultTimeout = 60 * time.Second

// sysPrompt and userPrompt are the VERBATIM judgment for the tagger: descriptive,
// inconclusive topic labels — NOT verdicts. Topic labels say "where X was
// discussed", never that X was settled. the sibling tool is the source of truth for
// importance/decisions; this only points at WHERE context lives.
const sysPrompt = "You label WHERE TOPICS were discussed in a raw chat transcript, to help an agent find context later. You do NOT judge importance, decisions, or conclusions — a separate system (the sibling tool) is the source of truth for those. Stay descriptive and INCONCLUSIVE."

const userPrompt = "Split this transcript into a small number of contiguous TOPIC segments (a topic = one coherent thread of discussion; treat a distinct sidequest as its own short segment). For each segment output: start_id (the #id where that topic begins), a short topic label (3–7 words naming the subject/concept — NOT a verdict or claim), and a one-line INCONCLUSIVE summary of what was explored (use 'discussed/explored/debated/left open'; NEVER 'decided', 'concluded', 'the answer is', or any finality). Topic labels must be universally accurate ('where X was discussed'), never a claim of importance or that something was settled.\nReturn ONLY a JSON array, no prose: [{\"start_id\": <int>, \"topic\": \"...\", \"summary\": \"...\"}]"

// Segment is one tagged topic span: the message id where the topic begins, a
// short label, and an inconclusive one-line summary.
type Segment struct {
	StartID int
	Topic   string
	Summary string
}

// Tagger labels a condensed transcript into topic Segments. The interface keeps
// the populate path mockable — a test injects a canned Tagger; the real one
// (HTTPTagger) speaks to an OpenAI-compatible chat endpoint.
type Tagger interface {
	TagSession(condensed string) ([]Segment, error)
}

// HTTPTagger tags over an OpenAI-compatible chat/completions endpoint. Unlike the
// embedder (which fails soft to nil), tagging is an explicit user-invoked verb,
// so TagSession returns an error on failure — the `tag` command surfaces it.
type HTTPTagger struct {
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
	client   *http.Client
}

// Compile-time check: HTTPTagger satisfies the Tagger port.
var _ Tagger = (*HTTPTagger)(nil)

// NewHTTPTagger constructs an HTTPTagger with defaults (model claude-haiku-4-5,
// 60s timeout) filled in for zero-valued fields. The endpoint has any trailing
// slash stripped.
func NewHTTPTagger(endpoint, model, apiKey string) *HTTPTagger {
	if model == "" {
		model = DefaultTagModel
	}
	return &HTTPTagger{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Model:    model,
		APIKey:   apiKey,
		Timeout:  defaultTimeout,
		client:   &http.Client{Timeout: defaultTimeout},
	}
}

// chatResponse is the OpenAI chat/completions response shape we read from:
// choices[0].message.content carries the model's text (a JSON array, here).
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// rawSeg is the on-the-wire segment shape the model emits inside the JSON array.
type rawSeg struct {
	StartID int    `json:"start_id"`
	Topic   string `json:"topic"`
	Summary string `json:"summary"`
}

// TagSession posts the condensed transcript to the chat endpoint and parses the
// returned JSON array of segments. The condensed view is windowed by the caller
// (cmd_tag) so a single TagSession sees one window's worth of text.
func (t *HTTPTagger) TagSession(condensed string) ([]Segment, error) {
	content, err := t.complete(condensed)
	if err != nil {
		return nil, err
	}
	return parseSegments(content)
}

// complete POSTs the system+user messages and returns choices[0].message.content.
func (t *HTTPTagger) complete(condensed string) (string, error) {
	payload := map[string]any{
		"model":       t.Model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt + "\n\n" + condensed},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal tag request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), t.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build tag request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("tag request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read tag response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tag endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode tag response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("tag response had no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (t *HTTPTagger) timeout() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return defaultTimeout
}

func (t *HTTPTagger) httpClient() *http.Client {
	if t.client != nil {
		return t.client
	}
	return &http.Client{Timeout: t.timeout()}
}

// parseSegments defensively extracts the segment array from a model reply. Haiku
// sometimes wraps the array in a ```json fence and/or trails prose after it, so:
// strip a leading/trailing code fence, then slice from the first '[' to the last
// matching ']', and Unmarshal that. Tolerant of trailing prose. An empty/garbage
// reply yields an error so the caller knows tagging produced nothing usable.
func parseSegments(content string) ([]Segment, error) {
	s := stripFence(content)

	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < 0 || end < start {
		return nil, fmt.Errorf("no JSON array in tagger reply: %q", trunc(content, 200))
	}
	arr := s[start : end+1]

	var raws []rawSeg
	if err := json.Unmarshal([]byte(arr), &raws); err != nil {
		return nil, fmt.Errorf("parse tagger JSON array: %w (got %q)", err, trunc(arr, 200))
	}

	out := make([]Segment, 0, len(raws))
	for _, r := range raws {
		out = append(out, Segment{StartID: r.StartID, Topic: r.Topic, Summary: r.Summary})
	}
	return out, nil
}

// stripFence removes a single leading ```json (or ```) fence and a trailing ```
// fence, returning the inner body trimmed of surrounding whitespace. A reply with
// no fence is returned trimmed but otherwise unchanged.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (```json, ```JSON, or a bare ```).
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	} else {
		s = strings.TrimPrefix(s, "```")
	}
	// Drop a trailing closing fence if present.
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// trunc returns the first n runes of s with an ellipsis when it was cut, used to
// keep error messages bounded.
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// GetTagger builds the configured Tagger from the environment, or nil if
// RAWCLAW_TAG_ENDPOINT is unset (the disabled-by-default state). Mirrors
// adapters.GetEmbedder: callers MUST guard with `if t != nil`. The model defaults
// to claude-haiku-4-5; the key falls back to LITELLM_KEY when RAWCLAW_TAG_KEY is
// empty.
func GetTagger() Tagger {
	ep := os.Getenv("RAWCLAW_TAG_ENDPOINT")
	if ep == "" {
		return nil // untyped nil: tagging disabled
	}

	model := os.Getenv("RAWCLAW_TAG_MODEL")
	if model == "" {
		model = DefaultTagModel
	}

	key := os.Getenv("RAWCLAW_TAG_KEY")
	if key == "" {
		key = os.Getenv("LITELLM_KEY")
	}

	return NewHTTPTagger(ep, model, key)
}
