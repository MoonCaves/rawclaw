# Outline system-segment audit — 2026-11-03

**Disposition: do not apply issue #101's proposed production patch to this revision.** This is a report-only audit at `0afecdfab2321abb49733746f7250e6572d5ec5e`; no production or test file was edited.

## Verified current paths

- **VERIFIED:** `store.BookendMessages` selects only `user` and `assistant` roles (`internal/store/messages.go:56`). Outline calls it for both opening and closing bookends (`internal/agentproto/outline.go:60-64`), so actual `system` rows do not enter either bookend, including with `IncludeTools=true`.[3][2]
- **VERIFIED:** All opening, closing, and last-human message previews pass `outlineDispCap = 300` to `view.RenderMsgsWith` (`outline.go:14,85-86,114`). Human output prints those already-shaped values; JSON serializes the same result object.[2][6]
- **VERIFIED:** `view.displayText` delegates plain content to `text.CapRunes`, which caps runes without splitting UTF-8; generated-material paths pass the same cap to `parse.DispWith` (`view.go:186-190`). This bounds displayed message previews, not every metadata field in a serialized outline.[4][5]
- **VERIFIED:** Outline's `--include-tools` is documented as including tool calls in the arc, not as removing the outline cap; `--with tools` maps to the same boolean. No `--raw` flag exists for this command (`internal/cli/cli_read.go:141-215`).[6]

## Synthetic execution

The fixture creates a production-schema SQLite store with `store.ConnectRW`/`store.Rebuild`, seeds two synthetic sessions (ASCII and multibyte Unicode), and calls the exported API and Cobra CLI in-process. Each session has twelve rows: opening `system,user,assistant`; six short assistant rows; closing `system,user,assistant`. Each long row contains 41,669 runes. CLI human and JSON output run with and without tools. All data/cache roots used by the probe are temporary; no private transcript corpus is scanned.

The following is the exact probe output (the `has_ellipsis` column tests any ellipsis glyph; human output has a middle-gap indicator, not a per-preview clipping marker):

```text
API outline ascii tools=false rows=8 max_runes=300 system_runes=[] valid_utf8=true
CLI outline ascii tools=false json=false bytes=2183 max_json_runes=300 has_ellipsis=true
CLI outline ascii tools=false json=true bytes=2972 max_json_runes=300 has_ellipsis=false
API outline ascii tools=true rows=8 max_runes=300 system_runes=[] valid_utf8=true
CLI outline ascii tools=true json=false bytes=2183 max_json_runes=300 has_ellipsis=true
CLI outline ascii tools=true json=true bytes=2972 max_json_runes=300 has_ellipsis=false
API read ascii tools=false more=0 anchor_runes=200 truncated=false next=""
API read ascii tools=false more=16 anchor_runes=200 truncated=false next=""
API read ascii tools=true more=0 anchor_runes=41669 truncated=false next=""
API read ascii tools=true more=16 anchor_runes=41669 truncated=false next=""
CLI outline --raw error="unknown flag: --raw"
API outline unicode tools=false rows=8 max_runes=300 system_runes=[] valid_utf8=true
CLI outline unicode tools=false json=false bytes=5183 max_json_runes=300 has_ellipsis=true
CLI outline unicode tools=false json=true bytes=5980 max_json_runes=300 has_ellipsis=false
API outline unicode tools=true rows=8 max_runes=300 system_runes=[] valid_utf8=true
CLI outline unicode tools=true json=false bytes=5183 max_json_runes=300 has_ellipsis=true
CLI outline unicode tools=true json=true bytes=5980 max_json_runes=300 has_ellipsis=false
API read unicode tools=false more=0 anchor_runes=200 truncated=false next=""
API read unicode tools=false more=16 anchor_runes=200 truncated=false next=""
API read unicode tools=true more=0 anchor_runes=41669 truncated=false next=""
API read unicode tools=true more=16 anchor_runes=41669 truncated=false next=""
CLI outline --raw error="unknown flag: --raw"
```

This fixture reproduces **no unbounded system-message expansion through outline**. `system_runes=[]` means excluded rows, not zero-length system previews. Displayed conversational message previews stay at or below 300 runes in both output forms and both tools modes; this does not establish a total envelope size bound for arbitrary topic/project metadata.

## Separate residual: clipping disclosure on explicit reads

**VERIFIED, separate from the issue's outline-flood allegation:** An explicitly referenced system row is capped at 200 runes by `BuildAnchoredView` unless `IncludeTools` is true. The fixture observes `Read.Truncated=false` and an empty `NextCommand` after that clipping; increasing the read window with `--more` still leaves the same 200-rune anchor. `IncludeTools=true` returns all 41,669 runes in this fixture. `Read.Truncated` is populated from `applyBudget` after the view has already capped content (`internal/agentproto/read.go:73,100`); the view's earlier cap does not set that budget statistic.[4][7]

This is a disclosure/recovery consideration for a separate fix, not evidence that outline currently floods. The current report neither changes cap semantics nor adds flags. The report also makes no claim that all possible generated markup or grapheme clusters were exhaustively tested: rune safety preserves UTF-8 code points, not necessarily whole visual graphemes.[5]

## Historical evidence and the proposed 1,200 cap

- **VERIFIED source discrepancy; historical corpus values remain unverified:** The issue describes 41,669 characters as an average across 134 segments. The measurement note calls 41,669 the **largest observed anchor**, and historical commit `a0f9fe7da27f84e45b36a9bec76845cf5189c76d` says "system 134 segments (one anchor 41,669 chars)". Current `view.go:290` says average, contradicting those historical authored records. No underlying measurement artifact was supplied or rescanned here, so none of the corpus statistics is independently re-established.[10][8][9]
- **VERIFIED:** The measurement note describes a 1,000-character `dispCap`; current source is 200. The issue proposes 1,200. These are distinct values; the cited note is not proof of the current constant or of a newly desired 1,200 limit.[8][4][10]
- **VERIFIED:** The issue's immutable ripgrep citation documents configurable `max_columns` omission of oversized byte-measured lines, a default of **no limit**, and optional grapheme-cluster previews. It provides neither a 1,200 constant nor a role-aware system policy nor an `include-tools` escape hatch.[1]
- **VERIFIED historical intent:** The historical system-only cap commit explicitly retained full requested tool/user/assistant anchor payloads after reviewers caught tool-anchor truncation. A broad cap increase/change in `view.go` would touch explicit `read` behavior as well as shared shaping; it is not an outline-only patch.[9][4]

## Recommendation and limits

**Recommendation:** Treat #101's exact unbounded-outline allegation as not reproduced at this pinned revision; retain the existing outline renderer and cap. The existing `text.CapRunes` helper and the already-applied outline cap mean no new truncation implementation is needed for that claim. If a current counterexample exists, obtain its exact command, source runtime, row role, and output shape before choosing another limit or repurposing a flag.[2][5][6]

**OPEN:** The distinct missing clipping disclosure and stale measurement wording merit a separate scoped followup. Arbitrary metadata size, live runtime source normalization, and all generated-content forms were not exhaustively tested. No production patch, full repository suite, issue close/comment, merge, install, or deployment is claimed by this audit.

## Reproduce the probe without editing repository files

Save the following Go source as `probe.go` in a temporary directory. Save an overlay JSON mapping the checkout's absolute `cmd/rawclaw/main.go` path to that `probe.go`. Run from the pinned checkout:

```sh
# AUDIT_DIR is a fresh temporary directory containing probe.go and overlay.json.
go run -overlay "$AUDIT_DIR/overlay.json" ./cmd/rawclaw
```

For example, generate the mapping with Python's `Path.cwd()` and `json.dump({"Replace": {str(Path.cwd()/"cmd/rawclaw/main.go"): str(Path(os.environ["AUDIT_DIR"])/"probe.go")}}, ...)`. The overlay substitutes only the compilation input; it does not overwrite the checkout.

```go
package main
import (
 "bytes"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "unicode/utf8"
 "github.com/MoonCaves/rawclaw/internal/agentproto"
 "github.com/MoonCaves/rawclaw/internal/cli"
 "github.com/MoonCaves/rawclaw/internal/index"
 "github.com/MoonCaves/rawclaw/internal/store"
 "github.com/MoonCaves/rawclaw/internal/view"
)
func must(err error) {if err!=nil {panic(err)}}
func main(){
 root,err:=os.MkdirTemp("","rawclaw-issue101-fixture-");must(err)
 for k,v:=range map[string]string{"XDG_CACHE_HOME":filepath.Join(root,"cache"),"XDG_DATA_HOME":filepath.Join(root,"data"),"XDG_CONFIG_HOME":filepath.Join(root,"config"),"RAWCLAW_ARCHIVE_AUTOSYNC":"off","RAWCLAW_BACKGROUND_INGEST":"off","RAWCLAW_ACTIVE_INGEST":"off"} {must(os.Setenv(k,v))}
 con,err:=store.ConnectRW(index.ConsolidatedPath());must(err);must(store.Rebuild(con))
 for _,variant:=range []string{"ascii","unicode"}{
 ch:="X"; if variant=="unicode" {ch="界"}
 sid:="aaaa0101-1111-2222-3333-"+variant
 payload:=strings.Repeat(ch,41669)
 roles:=[]string{"system","user","assistant","assistant","assistant","assistant","assistant","assistant","assistant","system","user","assistant"}
 _,err=con.Exec("INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,project,source_tool) VALUES(?,1,12,12,0,'audit','claude')",sid);must(err)
 for i,role:=range roles {
  content:=payload
  if i>=3&&i<9 {content="short intermediate"}
  _,err=con.Exec("INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,'2026-11-03T00:00:00Z',?)",sid,role,content,i+1,fmt.Sprintf("%08x-1111-2222-3333-444444444444",i+1));must(err)
 }
 scope:=[]view.Scope{{Project:"audit",DBP:index.ConsolidatedPath(),Source:"claude"}}
 for _,tools:=range []bool{false,true}{
  res,err:=agentproto.OutlineWith(sid,scope,agentproto.OutlineOpts{IncludeTools:tools});must(err)
  rows:=append(append([]view.ViewMsg{},res.Start...),res.End...)
  max:=0;valid:=true;systems:=[]int{}
  for _,m:=range rows{n:=utf8.RuneCountInString(m.Text);if n>max{max=n};valid=valid&&utf8.ValidString(m.Text);if m.Role=="system"{systems=append(systems,n)}}
  if res.LastHumanDirective!=nil&&utf8.RuneCountInString(res.LastHumanDirective.Text)>max{max=utf8.RuneCountInString(res.LastHumanDirective.Text)}
  fmt.Printf("API outline %s tools=%t rows=%d max_runes=%d system_runes=%v valid_utf8=%t\n",variant,tools,len(rows),max,systems,valid)
  for _,jsonMode:=range []bool{false,true}{
   var out bytes.Buffer;cmd:=cli.NewRootCmd(cli.BuildInfo{});cmd.SetOut(&out);cmd.SetErr(&out)
   args:=[]string{"outline",sid};if tools{args=append(args,"--include-tools")};if jsonMode{args=append(args,"--json")};cmd.SetArgs(args);must(cmd.Execute())
   if jsonMode{var obj agentproto.OutlineResult;must(json.Unmarshal(out.Bytes(),&obj));max=0;for _,m:=range append(obj.Start,obj.End...){if n:=utf8.RuneCountInString(m.Text);n>max{max=n}}}
   fmt.Printf("CLI outline %s tools=%t json=%t bytes=%d max_json_runes=%d has_ellipsis=%t\n",variant,tools,jsonMode,out.Len(),max,strings.Contains(out.String(),"…"))
  }
 }
 for _,tools:=range []bool{false,true}{
  for _,more:=range []int{0,agentproto.MoreWindow(1)}{
   r,err:=agentproto.Read(sid+":00000001",scope,agentproto.ReadOpts{IncludeTools:tools,Window:more});must(err)
   for _,m:=range r.AnchoredView.Window{if m.Anchor{fmt.Printf("API read %s tools=%t more=%d anchor_runes=%d truncated=%t next=%q\n",variant,tools,more,utf8.RuneCountInString(m.Text),r.Truncated,r.NextCommand)}}
  }
 }
 var out bytes.Buffer;cmd:=cli.NewRootCmd(cli.BuildInfo{});cmd.SetOut(&out);cmd.SetErr(&out);cmd.SetArgs([]string{"outline",sid,"--raw"});err=cmd.Execute();fmt.Printf("CLI outline --raw error=%q\n",err)
 }
 must(con.Close())
}
```

## Audit checks

Exact focused existing check command and output:

```text
$ go test ./internal/view ./internal/agentproto ./internal/cli -run '^(TestSystemAnchorIsCapped|TestToolAnchorStaysWhole|TestUserAnchorStaysWhole|TestRenderMsgsWithPreservesMultilineCode|TestRenderOutline|TestOutline_UntaggedSessionTailFallback|TestOutline_TaggedSessionSkipsFallback|TestOutlineWithThinkingAndTools|TestOutlinePrintsValidReadRefs)$' -count=1
ok  	github.com/MoonCaves/rawclaw/internal/view	0.771s
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	1.478s
ok  	github.com/MoonCaves/rawclaw/internal/cli	1.856s
[exit 0]
```

The independently constructed synthetic fixture compiled through Go's overlay and exited 0; its full source and output are preserved above. These are focused behavioral checks, not a claimed full-repository suite.

The citation ledger strict/evidence check reported ten cited sources with verified evidence quotes and `citations OK`; final verification is repeated before the report commit.

 Owning `AGENTS.md`/`CONTRIBUTING.md` were read; a report-only finding changes no product contract, so their content is intentionally unchanged. Graphify's `Outline` node helped locate the path; exact source was used to verify the role filter and caps. No code changes mean no graph rebuild is required.

## Sources

[1] https://raw.githubusercontent.com/BurntSushi/ripgrep/3fce3b5bb0236da2df6d99672afb8a719642eca7/crates/printer/src/standard.rs — ripgrep immutable standard printer source
[2] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/outline.go — internal/agentproto/outline.go
[3] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/store/messages.go — internal/store/messages.go
[4] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/view/view.go — internal/view/view.go
[5] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/text/helpers.go — internal/text/helpers.go
[6] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/cli/cli_read.go — internal/cli/cli_read.go
[7] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/read.go — internal/agentproto/read.go
[8] https://raw.githubusercontent.com/MoonCaves/rawclaw/0afecdfab2321abb49733746f7250e6572d5ec5e/docs/design/takeover-path-measurements.md — docs/design/takeover-path-measurements.md
[9] https://github.com/MoonCaves/rawclaw/commit/a0f9fe7da27f84e45b36a9bec76845cf5189c76d — Historical system-only cap decision
[10] https://github.com/MoonCaves/rawclaw/issues/101 — Issue101 allegation and proposed remediation
