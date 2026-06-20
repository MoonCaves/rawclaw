package redact

import (
	"strings"
	"testing"
)

// --- Positive cases: a real secret-shaped value must be redacted. ----------

func TestScrub_RedactsSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// mustGone is a substring that must NOT survive in the output.
		mustGone string
		// wantHit is the detector name expected to fire.
		wantHit string
	}{
		{
			name:     "anthropic key",
			in:       "export ANTHROPIC_API_KEY=sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWx",
			mustGone: "sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWx",
			wantHit:  "anthropic_key",
		},
		{
			name:     "openai key",
			in:       "key is sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123 here",
			mustGone: "sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123",
			wantHit:  "openai_key",
		},
		{
			name:     "openai proj key",
			in:       "sk-proj-AbCdEfGhIjKlMnOpQrStUvWxYz",
			mustGone: "sk-proj-AbCdEfGhIjKlMnOpQrStUvWxYz",
			wantHit:  "openai_key",
		},
		{
			name:     "aws access key AKIA",
			in:       "aws_access_key_id = AKIAIOSFODNN7EXAMPLE",
			mustGone: "AKIAIOSFODNN7EXAMPLE",
			wantHit:  "aws_access_key",
		},
		{
			name:     "aws access key ASIA",
			in:       "ASIAY34FZKBOKMUTVV7A is temporary",
			mustGone: "ASIAY34FZKBOKMUTVV7A",
			wantHit:  "aws_access_key",
		},
		{
			name:     "github classic token",
			in:       "token: ghp_1234567890abcdefghijklmnopqrstuvwx",
			mustGone: "ghp_1234567890abcdefghijklmnopqrstuvwx",
			wantHit:  "github_token",
		},
		{
			name:     "github oauth token gho_",
			in:       "gho_16C7e42F292c6912E7710c838347Ae178B4a",
			mustGone: "gho_16C7e42F292c6912E7710c838347Ae178B4a",
			wantHit:  "github_token",
		},
		{
			name:     "github fine-grained pat",
			in:       "github_pat_11ABCDEFG0AbcdefghIjklmn_OpQrStUvWxYz0123456789AbCdEfGhIjKl",
			mustGone: "github_pat_11ABCDEFG0",
			wantHit:  "github_pat",
		},
		{
			name:     "npm token",
			in:       "//registry.npmjs.org/:_authToken=npm_abcdefghijklmnopqrstuvwxyz0123456789",
			mustGone: "npm_abcdefghijklmnopqrstuvwxyz0123456789",
			wantHit:  "npm_token",
		},
		{
			name:     "slack bot token",
			in:       "SLACK=xoxb-1234567890-abcdefghijklmnop",
			mustGone: "xoxb-1234567890-abcdefghijklmnop",
			wantHit:  "slack_token",
		},
		{
			name:     "jwt",
			in:       "Authorization eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N",
			mustGone: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantHit:  "jwt",
		},
		{
			name:     "bearer token",
			in:       "Authorization: Bearer abcDEF1234567890ghIJKLmnop",
			mustGone: "abcDEF1234567890ghIJKLmnop",
			wantHit:  "bearer",
		},
		{
			name:     "basic auth",
			in:       "Authorization: Basic dXNlcjpzdXBlcnNlY3JldHBhc3N3b3Jk",
			mustGone: "dXNlcjpzdXBlcnNlY3JldHBhc3N3b3Jk",
			wantHit:  "basic_auth",
		},
		{
			name:     "url credentials",
			in:       "git clone https://alice:s3cr3tP4ss@github.com/org/repo.git",
			mustGone: "alice:s3cr3tP4ss",
			wantHit:  "url_credentials",
		},
		{
			name:     "private key block",
			in:       "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
			mustGone: "MIIEpAIBAAKCAQEA",
			wantHit:  "private_key",
		},
		{
			name:     "secret assignment equals",
			in:       `DATABASE_PASSWORD="hunter2hunter2"`,
			mustGone: "hunter2hunter2",
			wantHit:  "secret_assignment",
		},
		{
			name:     "secret assignment colon json",
			in:       `{"client_secret": "abcd1234EFGH5678"}`,
			mustGone: "abcd1234EFGH5678",
			wantHit:  "secret_assignment",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ScrubResult(tc.in)
			if strings.Contains(res.Text, tc.mustGone) {
				t.Errorf("secret survived redaction\n in:  %q\n out: %q\n leaked: %q",
					tc.in, res.Text, tc.mustGone)
			}
			if !res.Changed() {
				t.Errorf("expected Changed()=true, got false for %q", tc.in)
			}
			if !contains(res.Hits, tc.wantHit) {
				t.Errorf("expected hit %q, got %v", tc.wantHit, res.Hits)
			}
			if !strings.Contains(res.Text, "[REDACTED") {
				t.Errorf("expected a [REDACTED placeholder in output, got %q", res.Text)
			}
		})
	}
}

// --- Negative cases: ordinary prose / non-secret text must be untouched. ----

func TestScrub_LeavesProseUntouched(t *testing.T) {
	cases := []string{
		"Please rotate the API key before Friday.",
		"The access token expired so the request was rejected.",
		"I lost my house key and my password is hard to remember.",
		"We store the secret in the vault, not in the repo.",
		"The bearer of this letter is authorized.",
		"Use basic authentication for the legacy endpoint.",
		"sk- is a short prefix, not a key.",
		"He said the token economy is overrated.",
		"ghp_ alone without a body is not a token.",
		"Visit https://github.com/org/repo for the source.",
		"Set your key to a memorable phrase.",
		"AKIA is a prefix; the full id has sixteen more chars.",
		"The JWT spec defines three segments.",
		"password rules: minimum eight characters.",
	}
	for _, in := range cases {
		t.Run(short(in), func(t *testing.T) {
			res := ScrubResult(in)
			if res.Changed() {
				t.Errorf("ordinary prose was redacted\n in:  %q\n out: %q\n hits: %v",
					in, res.Text, res.Hits)
			}
			if res.Text != in {
				t.Errorf("clean text mutated\n in:  %q\n out: %q", in, res.Text)
			}
		})
	}
}

// --- Structural / determinism checks. --------------------------------------

func TestScrub_PreservesStructure(t *testing.T) {
	// Bearer keeps the keyword.
	got := Scrub("Authorization: Bearer abcDEF1234567890ghIJKLmnop")
	if !strings.Contains(got, "Bearer ") {
		t.Errorf("bearer keyword should survive: %q", got)
	}
	// URL credentials keep scheme and host.
	got = Scrub("https://alice:s3cr3tP4ss@github.com/org/repo.git")
	if !strings.HasPrefix(got, "https://") || !strings.Contains(got, "@github.com/org/repo.git") {
		t.Errorf("url scheme/host should survive: %q", got)
	}
	// Secret assignment keeps the key name and operator.
	got = Scrub(`DATABASE_PASSWORD="hunter2hunter2"`)
	if !strings.Contains(got, "DATABASE_PASSWORD") {
		t.Errorf("assignment key name should survive: %q", got)
	}
}

func TestScrub_Deterministic(t *testing.T) {
	in := "key=sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123 and ghp_1234567890abcdefghijklmnopqrstuvwx"
	a := Scrub(in)
	b := Scrub(in)
	if a != b {
		t.Errorf("non-deterministic output:\n a: %q\n b: %q", a, b)
	}
	if strings.Contains(a, "sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123") {
		t.Errorf("secret survived: %q", a)
	}
}

func TestScrub_Empty(t *testing.T) {
	if got := Scrub(""); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
	res := ScrubResult("")
	if res.Changed() {
		t.Error("empty input should not report a change")
	}
}

func TestScrub_MultipleSecretsOneString(t *testing.T) {
	in := "a sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWx b AKIAIOSFODNN7EXAMPLE c"
	res := ScrubResult(in)
	if strings.Contains(res.Text, "sk-ant-api03") || strings.Contains(res.Text, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("a secret survived: %q", res.Text)
	}
	if !contains(res.Hits, "anthropic_key") || !contains(res.Hits, "aws_access_key") {
		t.Errorf("expected both detectors to fire, got %v", res.Hits)
	}
}

// --- helpers ---------------------------------------------------------------

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func short(s string) string {
	if len(s) > 24 {
		return s[:24]
	}
	return s
}
