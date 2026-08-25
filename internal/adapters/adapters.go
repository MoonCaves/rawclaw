// Package adapters holds concrete Embedder adapters for the embed.Embedder port.
//
// Default behavior: RawClaw runs keyword-only with NOTHING
// configured here. An embedder is opt-in, wired entirely through environment
// variables so the public CLI bundles no keys, no service, no dependency:
//
//	RAWCLAW_EMBED_ENDPOINT   full embeddings URL (presence = vectors enabled)
//	RAWCLAW_EMBED_MODEL      model name            (default: nomic-embed-text)
//	RAWCLAW_EMBED_WIRE       ollama | openai       (auto-detected if unset)
//	RAWCLAW_EMBED_KEY        bearer token          (optional)
//	RAWCLAW_EMBED_INPUT_TYPE query|document        (optional; Voyage-style)
//	RAWCLAW_EMBED_DIM        expected dim          (optional; rejects a mismatch)
//
// Two wire formats cover every common backend:
//   - ollama : POST {endpoint} {"model","prompt"}                -> {"embedding":[...]}
//   - openai : POST {endpoint} {"model","input"[,"input_type"]}  -> {"data":[{"embedding":[...]}]}
//     (OpenAI, Voyage, and LiteLLM all speak this shape.)
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/embed"
)

// DefaultEmbedModel is the model used when RAWCLAW_EMBED_MODEL is unset.
const DefaultEmbedModel = "nomic-embed-text"

// defaultTimeout is the HTTP timeout applied to embedding requests.
const defaultTimeout = 15 * time.Second

// wireOllama and wireOpenAI are the two supported wire formats. Any value other
// than "ollama" takes the openai branch.
const (
	wireOllama = "ollama"
	wireOpenAI = "openai"
)

// HTTPEmbedder embeds over an HTTP embeddings endpoint. Embed returns nil on ANY
// failure (unreachable, timeout, non-200, malformed, wrong dim) so the keyword
// path always covers the gap — a missing/flaky embedder is never fatal.
type HTTPEmbedder struct {
	Endpoint  string
	Model     string
	Wire      string // "ollama" | "openai" (openai shape also serves voyage/litellm)
	APIKey    string
	InputType string
	Dim       int // 0 = no dim check
	Timeout   time.Duration
	client    *http.Client
}

// Compile-time check: HTTPEmbedder satisfies the Embedder port.
var _ embed.Embedder = (*HTTPEmbedder)(nil)

// Compile-time check: HTTPEmbedder satisfies the BatchEmbedder port.
var _ embed.BatchEmbedder = (*HTTPEmbedder)(nil)

// NewHTTPEmbedder constructs an HTTPEmbedder with sensible defaults (wire
// "ollama", timeout 15s) filled in for zero-valued fields. The endpoint has any
// trailing slash stripped.
func NewHTTPEmbedder(endpoint, model, wire, apiKey, inputType string, dim int) *HTTPEmbedder {
	if wire == "" {
		wire = wireOllama
	}
	return &HTTPEmbedder{
		Endpoint:  strings.TrimRight(endpoint, "/"),
		Model:     model,
		Wire:      wire,
		APIKey:    apiKey,
		InputType: inputType,
		Dim:       dim,
		Timeout:   defaultTimeout,
		client:    &http.Client{Timeout: defaultTimeout},
	}
}

// embedResponse covers the two response shapes: a top-level "embedding"
// (ollama) and a "data" array whose first element carries an "embedding"
// (openai/voyage/litellm). Both are parsed regardless of wire, so either shape
// resolves.
type embedResponse struct {
	Embedding []float64 `json:"embedding"`
	Data      []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed posts text to the endpoint using ctx and returns the dense vector, or nil on any
// failure. It builds the wire-shaped payload, posts it, and resolves the vector
// from "embedding" (ollama) or data[0].embedding (openai). Any error, non-200,
// empty vector, or dim mismatch yields nil so the keyword path covers the gap.
func (e *HTTPEmbedder) Embed(ctx context.Context, text string) []float64 {
	if ctx == nil {
		ctx = context.Background()
	}
	payload := e.payload(text, "")

	vec := e.post(ctx, payload)
	if len(vec) == 0 {
		return nil
	}
	if e.Dim != 0 && len(vec) != e.Dim {
		return nil
	}
	return vec
}

// batchEmbedResponse covers the batch response shape for the "openai" wire:
// {"data":[{"embedding":[...],"index":N},...]}.
type batchEmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// EmbedBatch embeds multiple texts in a single HTTP request using the "openai"
// wire format. If e.Wire is not wireOpenAI, or if any network, HTTP, or format
// error occurs, or if len(data) != len(texts), EmbedBatch returns nil so the
// caller falls back to per-item Embed. Results are positionally aligned with the
// input by ordering on the response's "index" field rather than array position.
// Individual vectors with a dim mismatch or empty data become nil.
func (e *HTTPEmbedder) EmbedBatch(ctx context.Context, texts []string) [][]float64 {
	if e.Wire != wireOpenAI || len(texts) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	payload := map[string]any{
		"model": e.Model,
		"input": texts,
	}
	if e.InputType != "" {
		payload["input_type"] = e.InputType
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	timeout := e.batchTimeout()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, e.Endpoint, bytes.NewReader(data))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	client := e.batchClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var parsed batchEmbedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}

	if len(parsed.Data) != len(texts) {
		return nil
	}

	res := make([][]float64, len(texts))
	seen := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) || seen[item.Index] {
			return nil
		}
		seen[item.Index] = true

		vec := item.Embedding
		if len(vec) == 0 {
			continue
		}
		if e.Dim != 0 && len(vec) != e.Dim {
			continue
		}
		res[item.Index] = vec
	}

	return res
}

// payload builds the request body for the configured wire. For the openai wire
// it attaches input_type from the call-site override, falling back to the
// embedder default.
func (e *HTTPEmbedder) payload(text, inputType string) map[string]any {
	if e.Wire == wireOllama {
		return map[string]any{"model": e.Model, "prompt": text}
	}

	body := map[string]any{"model": e.Model, "input": text}
	it := inputType
	if it == "" {
		it = e.InputType
	}
	if it != "" {
		body["input_type"] = it
	}
	return body
}

// post sends the payload and returns the resolved embedding, or nil on any
// failure. All failures collapse to nil (no error): unreachable, timeout,
// non-200, and malformed bodies are all the graceful keyword-only signal.
func (e *HTTPEmbedder) post(ctx context.Context, payload map[string]any) []float64 {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, e.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, e.Endpoint, bytes.NewReader(data))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := e.httpClient().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Treat any non-200 as the nil signal so the keyword path covers the gap.
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var parsed embedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	return resolveVector(parsed)
}

// resolveVector applies the resolution order: prefer the top-level "embedding";
// otherwise fall back to data[0].embedding. An empty result is the nil signal.
func resolveVector(r embedResponse) []float64 {
	if len(r.Embedding) > 0 {
		return r.Embedding
	}
	if len(r.Data) > 0 && len(r.Data[0].Embedding) > 0 {
		return r.Data[0].Embedding
	}
	return nil
}

func (e *HTTPEmbedder) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return defaultTimeout
}

func (e *HTTPEmbedder) httpClient() *http.Client {
	if e.client != nil {
		return e.client
	}
	return &http.Client{Timeout: e.timeout()}
}

// batchTimeout returns max(e.timeout(), 120s) because a multi-text batch request
// requires a longer timeout allowance than a single text embedding.
func (e *HTTPEmbedder) batchTimeout() time.Duration {
	t := e.timeout()
	if t < 120*time.Second {
		return 120 * time.Second
	}
	return t
}

// batchClient returns an http.Client configured with the batch timeout,
// preserving any custom Transport settings from e.client.
func (e *HTTPEmbedder) batchClient(timeout time.Duration) *http.Client {
	if e.client != nil {
		c := *e.client
		c.Timeout = timeout
		return &c
	}
	return &http.Client{Timeout: timeout}
}

// GetEmbedder builds the configured Embedder from the environment, or nil if
// RAWCLAW_EMBED_ENDPOINT is unset (the graceful keyword-only default).
//
// Returns an untyped nil embed.Embedder when unconfigured — callers MUST guard
// with `if e != nil`. Wire auto-detection keys off the endpoint (an "11434"
// port or an "/api/embeddings" path => ollama, else openai); model defaults to
// nomic-embed-text; a non-numeric DIM is treated as unset (the parse error is
// ignored and the dim check is left off).
func GetEmbedder() embed.Embedder {
	ep := os.Getenv("RAWCLAW_EMBED_ENDPOINT")
	if ep == "" {
		return nil // untyped nil: keyword-only default
	}

	model := os.Getenv("RAWCLAW_EMBED_MODEL")
	if model == "" {
		model = DefaultEmbedModel
	}

	wire := os.Getenv("RAWCLAW_EMBED_WIRE")
	if wire == "" {
		wire = detectWire(ep)
	}

	key := os.Getenv("RAWCLAW_EMBED_KEY")
	inputType := os.Getenv("RAWCLAW_EMBED_INPUT_TYPE")

	dim := 0
	if d := os.Getenv("RAWCLAW_EMBED_DIM"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			dim = v
		}
	}

	return NewHTTPEmbedder(ep, model, wire, key, inputType, dim)
}

// detectWire auto-detects the wire format: ollama when the endpoint mentions
// port 11434 or ends with /api/embeddings (after trimming a trailing slash);
// openai otherwise.
func detectWire(endpoint string) string {
	if strings.Contains(endpoint, "11434") {
		return wireOllama
	}
	if strings.HasSuffix(strings.TrimRight(endpoint, "/"), "/api/embeddings") {
		return wireOllama
	}
	return wireOpenAI
}
