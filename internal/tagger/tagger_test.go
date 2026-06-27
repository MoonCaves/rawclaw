package tagger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSegmentsCleanArray(t *testing.T) {
	in := `[{"start_id": 1, "topic": "vector fusion", "summary": "RRF blending explored"},
	        {"start_id": 5, "topic": "schema gating", "summary": "sidecar tables debated"}]`
	segs, err := parseSegments(in)
	if err != nil {
		t.Fatalf("parseSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("len = %d, want 2", len(segs))
	}
	if segs[0].StartID != 1 || segs[0].Topic != "vector fusion" || segs[0].Summary != "RRF blending explored" {
		t.Errorf("seg[0] = %+v", segs[0])
	}
	if segs[1].StartID != 5 || segs[1].Topic != "schema gating" {
		t.Errorf("seg[1] = %+v", segs[1])
	}
}

func TestParseSegmentsJSONFenced(t *testing.T) {
	// The Haiku ```json-fence bug: the array is wrapped in a code fence.
	in := "```json\n[{\"start_id\": 3, \"topic\": \"fence handling\", \"summary\": \"left open\"}]\n```"
	segs, err := parseSegments(in)
	if err != nil {
		t.Fatalf("parseSegments fenced: %v", err)
	}
	if len(segs) != 1 || segs[0].StartID != 3 || segs[0].Topic != "fence handling" {
		t.Fatalf("fenced parse = %+v", segs)
	}
}

func TestParseSegmentsBareFence(t *testing.T) {
	// A bare ``` fence (no language tag) must also be stripped.
	in := "```\n[{\"start_id\": 2, \"topic\": \"bare fence\", \"summary\": \"discussed\"}]\n```"
	segs, err := parseSegments(in)
	if err != nil {
		t.Fatalf("parseSegments bare fence: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "bare fence" {
		t.Fatalf("bare fence parse = %+v", segs)
	}
}

func TestParseSegmentsTrailingProse(t *testing.T) {
	// Tolerate prose before and after the array.
	in := "Here are the segments you asked for:\n" +
		`[{"start_id": 7, "topic": "trailing prose", "summary": "explored"}]` +
		"\nHope that helps! Let me know if you need anything else."
	segs, err := parseSegments(in)
	if err != nil {
		t.Fatalf("parseSegments trailing prose: %v", err)
	}
	if len(segs) != 1 || segs[0].StartID != 7 || segs[0].Topic != "trailing prose" {
		t.Fatalf("trailing prose parse = %+v", segs)
	}
}

func TestParseSegmentsNoArray(t *testing.T) {
	if _, err := parseSegments("I could not find any topics, sorry."); err == nil {
		t.Fatal("expected an error when the reply has no JSON array")
	}
}

func TestGetTaggerDisabledByDefault(t *testing.T) {
	t.Setenv("RAWCLAW_TAG_ENDPOINT", "")
	if tg := GetTagger(); tg != nil {
		t.Fatalf("GetTagger with no endpoint = %v, want nil", tg)
	}
}

func TestGetTaggerConfigured(t *testing.T) {
	t.Setenv("RAWCLAW_TAG_ENDPOINT", "http://example.test/v1/chat/completions")
	t.Setenv("RAWCLAW_TAG_MODEL", "")
	t.Setenv("RAWCLAW_TAG_KEY", "")
	t.Setenv("LITELLM_KEY", "fallback-key")

	tg := GetTagger()
	if tg == nil {
		t.Fatal("GetTagger with an endpoint = nil, want a tagger")
	}
	ht, ok := tg.(*HTTPTagger)
	if !ok {
		t.Fatalf("GetTagger returned %T, want *HTTPTagger", tg)
	}
	if ht.Model != DefaultTagModel {
		t.Errorf("Model = %q, want default %q", ht.Model, DefaultTagModel)
	}
	if ht.APIKey != "fallback-key" {
		t.Errorf("APIKey = %q, want LITELLM_KEY fallback", ht.APIKey)
	}
}

// TestTagSessionRoundTrip drives HTTPTagger against a mock OpenAI-compatible
// chat endpoint: it asserts the request shape (model, temperature 0, system+user
// messages, bearer auth) and that the reply's JSON array is parsed into Segments.
func TestTagSessionRoundTrip(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		// Reply in the OpenAI chat shape, with the array fenced to exercise the
		// defensive parser end-to-end.
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+
			"```json\\n[{\\\"start_id\\\": 1, \\\"topic\\\": \\\"round trip\\\", \\\"summary\\\": \\\"explored\\\"}]\\n```"+
			`"}}]}`)
	}))
	defer srv.Close()

	tg := NewHTTPTagger(srv.URL, "", "secret-token")
	segs, err := tg.TagSession("[#1 user] hello\n[#2 assistant] hi")
	if err != nil {
		t.Fatalf("TagSession: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "round trip" || segs[0].StartID != 1 {
		t.Fatalf("segments = %+v", segs)
	}

	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want Bearer secret-token", gotAuth)
	}
	if gotBody["model"] != DefaultTagModel {
		t.Errorf("request model = %v, want %q", gotBody["model"], DefaultTagModel)
	}
	if temp, ok := gotBody["temperature"].(float64); !ok || temp != 0 {
		t.Errorf("request temperature = %v, want 0", gotBody["temperature"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("request messages = %v, want 2", gotBody["messages"])
	}
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || !strings.Contains(sys["content"].(string), "INCONCLUSIVE") {
		t.Errorf("system message = %v", sys)
	}
	usr := msgs[1].(map[string]any)
	if usr["role"] != "user" || !strings.Contains(usr["content"].(string), "[#1 user] hello") {
		t.Errorf("user message missing condensed transcript: %v", usr)
	}
}

// TestTagSessionNon200 confirms a non-200 from the endpoint surfaces as an error
// (tagging is an explicit verb, not a soft-fail path like the embedder).
func TestTagSessionNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	tg := NewHTTPTagger(srv.URL, "m", "")
	if _, err := tg.TagSession("[#1 user] hi"); err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}
