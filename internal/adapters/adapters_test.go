package adapters

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/embed"
)

// nullEmbedder always returns nil. It is the empty-default baseline
// (keyword-only) and must satisfy the port contract.
type nullEmbedder struct{}

func (nullEmbedder) Embed(string) []float64 { return nil }

// fakeEmbedder returns a fixed 3-dim vector.
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(string) []float64 { return []float64{0.1, 0.2, 0.3} }

var (
	_ embed.Embedder = nullEmbedder{}
	_ embed.Embedder = fakeEmbedder{}
)

// TestEmbedderPortContract verifies that every embedder returns nil OR a
// non-empty []float64, for normal and empty input, and that the nil/vector
// routing decision is stable across calls.
func TestEmbedderPortContract(t *testing.T) {
	t.Parallel()

	embedders := []struct {
		name string
		e    embed.Embedder
	}{
		{name: "null", e: nullEmbedder{}},
		{name: "fake", e: fakeEmbedder{}},
	}

	inputs := []struct {
		name string
		text string
	}{
		{name: "normal", text: "hello world"},
		{name: "empty", text: ""},
	}

	for _, em := range embedders {
		for _, in := range inputs {
			t.Run(em.name+"/"+in.name, func(t *testing.T) {
				t.Parallel()
				out := em.e.Embed(in.text)
				// Contract: nil OR non-empty []float64. Go has no per-element
				// float type-check (the slice is statically []float64).
				if out != nil && len(out) == 0 {
					t.Fatalf("Embed(%q) returned non-nil but empty slice — violates the port contract", in.text)
				}
			})
		}
	}
}

// TestEmbedderDeterminism verifies that the nil/vector routing decision and the
// vector itself are stable for identical input.
func TestEmbedderDeterminism(t *testing.T) {
	t.Parallel()

	embedders := []struct {
		name string
		e    embed.Embedder
	}{
		{name: "null", e: nullEmbedder{}},
		{name: "fake", e: fakeEmbedder{}},
	}

	const text = "determinism check"
	for _, em := range embedders {
		t.Run(em.name, func(t *testing.T) {
			t.Parallel()
			out1 := em.e.Embed(text)
			out2 := em.e.Embed(text)

			if (out1 == nil) != (out2 == nil) {
				t.Fatalf("routing decision unstable: first nil=%v, second nil=%v", out1 == nil, out2 == nil)
			}
			if out1 == nil {
				return
			}
			if len(out1) != len(out2) {
				t.Fatalf("vector lengths differ for identical input: %d vs %d", len(out1), len(out2))
			}
			for i := range out1 {
				if out1[i] != out2[i] {
					t.Fatalf("vector[%d] differs between calls: %v vs %v", i, out1[i], out2[i])
				}
			}
		})
	}
}

// newTestEmbedder builds an HTTPEmbedder pointed at srv with the given wire and
// a fast timeout so failure-path tests don't stall.
func newTestEmbedder(endpoint, wire, apiKey, inputType string, dim int) *HTTPEmbedder {
	e := NewHTTPEmbedder(endpoint, "test-model", wire, apiKey, inputType, dim)
	e.Timeout = 2 * time.Second
	e.client = &http.Client{Timeout: 2 * time.Second}
	return e
}

// TestHTTPEmbedder_WireFormats verifies the request payload shape per wire and
// the two response-resolution paths (top-level "embedding" vs data[0].embedding).
func TestHTTPEmbedder_WireFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wire        string
		inputType   string
		respBody    string
		wantVec     []float64
		wantPayload map[string]any
		wantAuth    string
		apiKey      string
	}{
		{
			name:        "ollama prompt -> embedding",
			wire:        wireOllama,
			respBody:    `{"embedding":[0.1,0.2,0.3]}`,
			wantVec:     []float64{0.1, 0.2, 0.3},
			wantPayload: map[string]any{"model": "test-model", "prompt": "hi"},
		},
		{
			name:        "openai input -> data[0].embedding",
			wire:        wireOpenAI,
			respBody:    `{"data":[{"embedding":[1,2,3,4]}]}`,
			wantVec:     []float64{1, 2, 3, 4},
			wantPayload: map[string]any{"model": "test-model", "input": "hi"},
		},
		{
			name:        "openai with input_type",
			wire:        wireOpenAI,
			inputType:   "document",
			respBody:    `{"data":[{"embedding":[5,6]}]}`,
			wantVec:     []float64{5, 6},
			wantPayload: map[string]any{"model": "test-model", "input": "hi", "input_type": "document"},
		},
		{
			name:        "openai response with both keys prefers top-level embedding",
			wire:        wireOpenAI,
			respBody:    `{"embedding":[9,9],"data":[{"embedding":[1,1]}]}`,
			wantVec:     []float64{9, 9},
			wantPayload: map[string]any{"model": "test-model", "input": "hi"},
		},
		{
			name:        "bearer header set when api key present",
			wire:        wireOllama,
			apiKey:      "secret-token",
			respBody:    `{"embedding":[0.5]}`,
			wantVec:     []float64{0.5},
			wantPayload: map[string]any{"model": "test-model", "prompt": "hi"},
			wantAuth:    "Bearer secret-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPayload map[string]any
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotPayload)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.respBody)
			}))
			defer srv.Close()

			e := newTestEmbedder(srv.URL, tt.wire, tt.apiKey, tt.inputType, 0)
			got := e.Embed("hi")

			if !floatsEqual(got, tt.wantVec) {
				t.Fatalf("vector = %v, want %v", got, tt.wantVec)
			}
			if !jsonEqual(t, gotPayload, tt.wantPayload) {
				t.Fatalf("payload = %v, want %v", gotPayload, tt.wantPayload)
			}
			if gotAuth != tt.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

// TestHTTPEmbedder_FailuresReturnNil verifies that every failure mode collapses
// to nil so the keyword path covers the gap.
func TestHTTPEmbedder_FailuresReturnNil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		respBody string
		dim      int
	}{
		{name: "non-200 status", status: http.StatusInternalServerError, respBody: `{"embedding":[1,2]}`},
		{name: "malformed json", status: http.StatusOK, respBody: `not json`},
		{name: "no embedding key", status: http.StatusOK, respBody: `{"foo":"bar"}`},
		{name: "empty embedding array", status: http.StatusOK, respBody: `{"embedding":[]}`},
		{name: "empty data array", status: http.StatusOK, respBody: `{"data":[]}`},
		{name: "dim mismatch rejects vector", status: http.StatusOK, respBody: `{"embedding":[1,2,3]}`, dim: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.respBody)
			}))
			defer srv.Close()

			e := newTestEmbedder(srv.URL, wireOllama, "", "", tt.dim)
			if got := e.Embed("hi"); got != nil {
				t.Fatalf("Embed = %v, want nil (failure should be the keyword-only signal)", got)
			}
		})
	}
}

// TestHTTPEmbedder_DimMatchKeepsVector verifies a matching dim passes the gate.
func TestHTTPEmbedder_DimMatchKeepsVector(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"embedding":[1,2,3]}`)
	}))
	defer srv.Close()

	e := newTestEmbedder(srv.URL, wireOllama, "", "", 3)
	got := e.Embed("hi")
	if !floatsEqual(got, []float64{1, 2, 3}) {
		t.Fatalf("matching dim should keep vector, got %v", got)
	}
}

// TestHTTPEmbedder_UnreachableReturnsNil verifies a transport error (closed
// server) collapses to nil rather than panicking or erroring.
func TestHTTPEmbedder_UnreachableReturnsNil(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now unreachable

	e := newTestEmbedder(url, wireOllama, "", "", 0)
	if got := e.Embed("hi"); got != nil {
		t.Fatalf("unreachable endpoint should return nil, got %v", got)
	}
}

// TestNewHTTPEmbedder_Defaults verifies constructor defaults: trailing slash
// stripped, empty wire defaults to ollama, timeout defaults to 15s.
func TestNewHTTPEmbedder_Defaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		endpoint     string
		wire         string
		wantEndpoint string
		wantWire     string
	}{
		{name: "trailing slash stripped", endpoint: "http://x/api/", wire: "ollama", wantEndpoint: "http://x/api", wantWire: "ollama"},
		{name: "no trailing slash kept", endpoint: "http://x/api", wire: "openai", wantEndpoint: "http://x/api", wantWire: "openai"},
		{name: "empty wire defaults to ollama", endpoint: "http://x", wire: "", wantEndpoint: "http://x", wantWire: "ollama"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := NewHTTPEmbedder(tt.endpoint, "m", tt.wire, "", "", 0)
			if e.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %q, want %q", e.Endpoint, tt.wantEndpoint)
			}
			if e.Wire != tt.wantWire {
				t.Errorf("Wire = %q, want %q", e.Wire, tt.wantWire)
			}
			if e.Timeout != defaultTimeout {
				t.Errorf("Timeout = %v, want %v", e.Timeout, defaultTimeout)
			}
		})
	}
}

// TestGetEmbedder verifies env-wired construction: untyped-nil when unset, wire
// auto-detection, defaults, and dim parsing.
func TestGetEmbedder(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantNil   bool
		wantWire  string
		wantModel string
		wantDim   int
		wantKey   string
		wantIType string
	}{
		{
			name:    "unset endpoint returns nil",
			env:     map[string]string{},
			wantNil: true,
		},
		{
			name:      "11434 port auto-detects ollama",
			env:       map[string]string{"RAWCLAW_EMBED_ENDPOINT": "http://localhost:11434/api/embeddings"},
			wantWire:  "ollama",
			wantModel: DefaultEmbedModel,
		},
		{
			name:      "api/embeddings path auto-detects ollama",
			env:       map[string]string{"RAWCLAW_EMBED_ENDPOINT": "http://host/api/embeddings"},
			wantWire:  "ollama",
			wantModel: DefaultEmbedModel,
		},
		{
			name:      "api/embeddings path with trailing slash auto-detects ollama",
			env:       map[string]string{"RAWCLAW_EMBED_ENDPOINT": "http://host/api/embeddings/"},
			wantWire:  "ollama",
			wantModel: DefaultEmbedModel,
		},
		{
			name:      "other endpoint auto-detects openai",
			env:       map[string]string{"RAWCLAW_EMBED_ENDPOINT": "https://api.voyageai.com/v1/embeddings"},
			wantWire:  "openai",
			wantModel: DefaultEmbedModel,
		},
		{
			name: "explicit wire overrides detection",
			env: map[string]string{
				"RAWCLAW_EMBED_ENDPOINT": "http://localhost:11434/api/embeddings",
				"RAWCLAW_EMBED_WIRE":     "openai",
			},
			wantWire:  "openai",
			wantModel: DefaultEmbedModel,
		},
		{
			name: "all env vars wired through",
			env: map[string]string{
				"RAWCLAW_EMBED_ENDPOINT":   "https://api.openai.com/v1/embeddings",
				"RAWCLAW_EMBED_MODEL":      "text-embedding-3-small",
				"RAWCLAW_EMBED_WIRE":       "openai",
				"RAWCLAW_EMBED_KEY":        "sk-xyz",
				"RAWCLAW_EMBED_INPUT_TYPE": "query",
				"RAWCLAW_EMBED_DIM":        "1536",
			},
			wantWire:  "openai",
			wantModel: "text-embedding-3-small",
			wantDim:   1536,
			wantKey:   "sk-xyz",
			wantIType: "query",
		},
		{
			name: "non-numeric dim treated as unset",
			env: map[string]string{
				"RAWCLAW_EMBED_ENDPOINT": "https://api.example.com/embed",
				"RAWCLAW_EMBED_DIM":      "notanumber",
			},
			wantWire:  "openai",
			wantModel: DefaultEmbedModel,
			wantDim:   0,
		},
	}

	// Not parallel: these mutate process env via t.Setenv (which forbids Parallel).
	allKeys := []string{
		"RAWCLAW_EMBED_ENDPOINT",
		"RAWCLAW_EMBED_MODEL",
		"RAWCLAW_EMBED_WIRE",
		"RAWCLAW_EMBED_KEY",
		"RAWCLAW_EMBED_INPUT_TYPE",
		"RAWCLAW_EMBED_DIM",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range allKeys {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := GetEmbedder()
			if tt.wantNil {
				if got != nil {
					t.Fatalf("GetEmbedder() = %v, want untyped nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("GetEmbedder() = nil, want an embedder")
			}
			he, ok := got.(*HTTPEmbedder)
			if !ok {
				t.Fatalf("GetEmbedder() returned %T, want *HTTPEmbedder", got)
			}
			if he.Wire != tt.wantWire {
				t.Errorf("Wire = %q, want %q", he.Wire, tt.wantWire)
			}
			if he.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", he.Model, tt.wantModel)
			}
			if he.Dim != tt.wantDim {
				t.Errorf("Dim = %d, want %d", he.Dim, tt.wantDim)
			}
			if he.APIKey != tt.wantKey {
				t.Errorf("APIKey = %q, want %q", he.APIKey, tt.wantKey)
			}
			if he.InputType != tt.wantIType {
				t.Errorf("InputType = %q, want %q", he.InputType, tt.wantIType)
			}
		})
	}
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func jsonEqual(t *testing.T, got, want map[string]any) bool {
	t.Helper()
	gb, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wb, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	return string(gb) == string(wb)
}
