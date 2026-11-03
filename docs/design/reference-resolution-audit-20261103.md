# Issue #100 — numeric reference resolution audit

Status: **VERIFIED — issue #100's numeric-shape rejection is not reproduced on the reviewed main revision.** Recommended production delta: zero lines.
This report is a consideration for issue disposition; it does not close the issue.

Reviewed revision: `0afecdfab2321abb49733746f7250e6572d5ec5e`.

## Current grammar and lookup

`resolveRef` strips one leading `ref=`, requires exactly two colon-separated parts, lowercases the message part, and accepts a nonempty hexadecimal message prefix; it has no decimal-index branch.[2]
The session part is passed to session lookup without numeric classification; neither part is required to be exactly eight characters by this parser, and the parser itself accepts an empty session part.[2]
The latter is a separate grammar edge, not evidence of ordinal routing.

`Read` locates the session first and then resolves the message UUID prefix within that session.[3]
`ResolveMessageUUID` performs a parameterized `session_id` plus `uuid LIKE prefix%` query, returning at most two IDs to distinguish missing, unique, and ambiguous matches.[5]
Numeric `42` is a valid hex prefix, not a request for the 42nd message; `#2`, nonhex text, a bare session token, and a full hyphenated message UUID are outside the current read-ref grammar.[2]

The CLI help specifies `<session8>:<uuid8>` copied from a search hit; `--more` widens the window and `--around` shifts it while retaining that reference.[6]
Session and message ambiguity errors request a longer corresponding prefix; a missing message currently receives a plain not-found error, with no numeric-index fallback.[3][4]
Existing parser tests explicitly include `a1b2c3d4:42` and `0014079c:10836404`.[7]

## History and upstream comparison

Main ancestor `3fddfa3e485a1d0efe5b17dcd70954144cfd890f` deletes both `reNumericRef` and the prelookup rejection of decimal-shaped message prefixes.[9]
A separate historical fix, `bd466e5192191d5e9b5a631c12ffcc0653a9d517`, makes a similar deletion but is **not** an ancestor of this reviewed main; do not present that branch as the landed fix.[8]
The old code rejected decimal-shaped message UUID prefixes with an old-numeric-ref error; it did not perform sequential-index resolution in that branch.[8][9]
Historical corpus percentages in commit prose were not independently remeasured for this audit.

The issue's pinned Git `get_short_oid` span (`object-name.c:455-510`) initializes disambiguation, selects object-kind hints, enumerates candidate objects, finishes disambiguation, and retries an object-store miss.[1]
It is useful prefix-lookup prior art, but it does not prescribe RawClaw's two-part session/message grammar or establish a numeric-index fallback.[1]
A strict whole-UUID validator would be a different contract from the shorter prefixes the current parser intentionally accepts.[2]

## Synthetic results

The disposable fixture below invokes the real `Read` and human/JSON render paths using temporary transcripts and an isolated HOME; no private transcript was read.
Its own messages distinguish UUID-prefix lookup from ordinal lookup:

| Input | Observed result |
| --- | --- |
| `12345678:10836404` | Numeric session and numeric message prefixes resolve the expected anchor. |
| `0123abcd:0123abcd` | Numeric-leading hex prefixes resolve the expected anchor. |
| `ref=12345678:42` | Resolves the third fixture message by UUID prefix, although the session has only five messages. |
| `12345678:2` | Message not found, although an ordinal second message exists. |
| `12345678:77` | Two message UUID matches produce `ErrAmbiguousUUID`. |
| `12345679:abcdef01` | Two session matches produce `ErrAmbiguousSession`. |
| `99999999:10836404` | Session not found, before message resolution. |

The parser-only checks additionally confirm that an empty session component and a long decimal message component reach the syntactic result without integer parsing.
They do not establish that either input resolves to a stored entity.
These findings are bounded to the pinned revision and this fixture; arbitrary store corruption, live archives, and legacy-version migration were not tested.

## Reproduction and exact receipts

Save the Go fixture below as `/tmp/issue100-ref-audit-overlay_test.go`. Create its overlay from a checkout of the reviewed revision:

```sh
python3 - <<'PY_OVERLAY'
import json
from pathlib import Path
root = Path.cwd()
Path('/tmp/issue100-ref-audit-overlay.json').write_text(json.dumps({'Replace': {
    str(root / 'internal/agentproto/issue100_ref_audit_overlay_test.go'):
    '/tmp/issue100-ref-audit-overlay_test.go'
}}))
PY_OVERLAY
```

```go
package agentproto

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestIssue100ReferenceAudit(t *testing.T) {
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	proj := t.TempDir()
	writeMultiSession(t, proj, "12345678aaaa", [][2]string{{"108364040001", "digit uuid"}, {"900000000001", "second ordinal"}, {"42abcdef0001", "forty-two prefix"}, {"77aaaaaa0001", "collision one"}, {"77bbbbbb0002", "collision two"}})
	writeMultiSession(t, proj, "0123abcd1111", [][2]string{{"0123abcd0001", "numeric-leading hex"}})
	writeSession(t, proj, "12345679aaaa", "abcdef010001", "session collision one")
	writeSession(t, proj, "12345679bbbb", "abcdef020001", "session collision two")
	scope := scopeFor(t, proj)
	for _, c := range []struct{ ref, sid, body string }{{"12345678:10836404", "12345678aaaa", "digit uuid"}, {"0123abcd:0123abcd", "0123abcd1111", "numeric-leading hex"}, {"ref=12345678:42", "12345678aaaa", "forty-two prefix"}} {
		r, err := Read(c.ref, scope, ReadOpts{})
		if err != nil {
			t.Fatalf("%s: %v", c.ref, err)
		}
		if r.SessionID != c.sid {
			t.Fatalf("%s: session %q", c.ref, r.SessionID)
		}
		found := false
		for _, m := range r.Window {
			if m.Anchor && m.Text == c.body {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: wrong anchor", c.ref)
		}
		for _, wantJSON := range []bool{false, true} {
			var out bytes.Buffer
			if err := ReadAndRender(&out, c.ref, scope, nil, ReadOpts{}, wantJSON); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), c.body) {
				t.Fatalf("missing body, json=%v", wantJSON)
			}
		}
		t.Logf("%s -> session=%s anchor=%q; human+JSON contain anchor", c.ref, r.SessionID, c.body)
	}
	for _, c := range []struct{ ref, kind string }{{"12345678:2", "message missing"}, {"12345678:77", "message ambiguous"}, {"12345679:abcdef01", "session ambiguous"}, {"99999999:10836404", "session missing"}} {
		_, err := Read(c.ref, scope, ReadOpts{})
		if err == nil {
			t.Fatalf("%s unexpectedly resolved", c.ref)
		}
		var mm *ErrMsgNotFound
		var ma *ErrAmbiguousUUID
		var sa *ErrAmbiguousSession
		var sm *ErrSessionNotFound
		ok := false
		switch c.kind {
		case "message missing":
			ok = errors.As(err, &mm)
		case "message ambiguous":
			ok = errors.As(err, &ma)
		case "session ambiguous":
			ok = errors.As(err, &sa)
		case "session missing":
			ok = errors.As(err, &sm)
		}
		if !ok {
			t.Fatalf("%s wrong error %T: %v", c.ref, err, err)
		}
		t.Logf("%s -> %s (%T)", c.ref, c.kind, err)
	}
	for _, ref := range []string{"12345678", "12345678:xyz", "12345678:10836404-0001", "12345678:#2"} {
		if _, _, err := resolveRef(ref); err == nil {
			t.Fatalf("bad grammar accepted %q", ref)
		}
		t.Logf("%s -> grammar error", ref)
	}
	for _, ref := range []string{":42", "12345678:999999999999999999999999999999"} {
		sid, u, err := resolveRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("parser-only %s -> session=%q UUID-prefix=%q", ref, sid, u)
	}

}
```

```text
$ go test -race -overlay=/tmp/issue100-ref-audit-overlay.json ./internal/agentproto -run '^TestIssue100ReferenceAudit$' -count=1 -v
=== RUN   TestIssue100ReferenceAudit
    issue100_ref_audit_overlay_test.go:48: 12345678:10836404 -> session=12345678aaaa anchor="digit uuid"; human+JSON contain anchor
    issue100_ref_audit_overlay_test.go:48: 0123abcd:0123abcd -> session=0123abcd1111 anchor="numeric-leading hex"; human+JSON contain anchor
    issue100_ref_audit_overlay_test.go:48: ref=12345678:42 -> session=12345678aaaa anchor="forty-two prefix"; human+JSON contain anchor
    issue100_ref_audit_overlay_test.go:73: 12345678:2 -> message missing (*agentproto.ErrMsgNotFound)
    issue100_ref_audit_overlay_test.go:73: 12345678:77 -> message ambiguous (*agentproto.ErrAmbiguousUUID)
    issue100_ref_audit_overlay_test.go:73: 12345679:abcdef01 -> session ambiguous (*agentproto.ErrAmbiguousSession)
    issue100_ref_audit_overlay_test.go:73: 99999999:10836404 -> session missing (*agentproto.ErrSessionNotFound)
    issue100_ref_audit_overlay_test.go:79: 12345678 -> grammar error
    issue100_ref_audit_overlay_test.go:79: 12345678:xyz -> grammar error
    issue100_ref_audit_overlay_test.go:79: 12345678:10836404-0001 -> grammar error
    issue100_ref_audit_overlay_test.go:79: 12345678:#2 -> grammar error
    issue100_ref_audit_overlay_test.go:86: parser-only :42 -> session="" UUID-prefix="42"
    issue100_ref_audit_overlay_test.go:86: parser-only 12345678:999999999999999999999999999999 -> session="12345678" UUID-prefix="999999999999999999999999999999"
--- PASS: TestIssue100ReferenceAudit (0.47s)
PASS
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	2.768s
[exit 0]

$ go test -race ./internal/agentproto -run '^(TestResolveRef|TestSearchPrintedRefRoundTrips|TestReadAmbiguousUUID|TestLocateSessionAmbiguous)$' -count=1
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	2.204s
[exit 0]

$ git merge-base --is-ancestor bd466e5192191d5e9b5a631c12ffcc0653a9d517 HEAD
[no output; exit 1]
$ git merge-base --is-ancestor 3fddfa3e485a1d0efe5b17dcd70954144cfd890f HEAD
[no output; exit 0]
```

No production or repository test files changed. The checks above use package-level CLI rendering, not a separately installed binary; no full suite, live editor, archive mutation, issue comment, merge, or installation was performed. Owning documentation was checked; the root Index already routes readers to `docs/design/`, and the report changes no contract or structure.

## Sources

[1] https://raw.githubusercontent.com/git/git/b8242b093d9e941a34460d715e3ce616a34ac3fe/object-name.c — Pinned Git short object resolution
[2] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/types.go — internal/agentproto/types.go
[3] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/read.go — internal/agentproto/read.go
[4] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/locate.go — internal/agentproto/locate.go
[5] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/store/messages.go — internal/store/messages.go
[6] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/cli_read.go — internal/cli/cli_read.go
[7] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/agentproto_test.go — internal/agentproto/agentproto_test.go
[8] https://github.com/MoonCaves/rawclaw/commit/bd466e5192191d5e9b5a631c12ffcc0653a9d517 — Historical all-digit rejection removal
[9] https://github.com/MoonCaves/rawclaw/commit/3fddfa3e485a1d0efe5b17dcd70954144cfd890f — Main ancestor removes numeric-ref rejection
