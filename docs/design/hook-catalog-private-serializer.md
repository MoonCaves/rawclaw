# Private hook catalog serialization

Source: created here. The serializer adopts selected MIT units from
[agent-deck@2e897f2a62f6dd570628d2021a0994a3a7db1f4f](https://github.com/asheshgoplani/agent-deck/tree/2e897f2a62f6dd570628d2021a0994a3a7db1f4f).
It changes only private Claude/Codex metadata construction before the existing
catalog hard-link claim. Antigravity retains the existing shell serializer.

## Source mapping

`cmd/agent-deck/hook_handler.go` supplies lenient RawMessage fields (56–65),
`rawString` (98–106), read/empty/decode error returns (370–377), JSON encoding and
writer error returns (893–912), and the complete atomic writer (968–985).
`cmd/agent-deck/codex_hooks_cmd.go:192` supplies the full stdin read call.
The donor limit, status logging, status filename derivation and unrelated guards
are omitted. The selected read call is passed the command's input reader.
The existing RawClaw CatalogEntry schema and explicit shell-selected ID/source
are field wiring. `cwd` uses the donor's lenient field pattern instead of its
typed Cwd field; invalid metadata types become omitted empty strings.

The helper emits no stdout. The existing shell group still opens the private
entry before invoking it. Successful native rename publishes complete JSON;
handled failures leave the shell-created empty marker. The original claim,
cleanup, detached ingest and discovery-banner order is retained. No new ID,
event, invalid-input or fallback rule is added. Legacy shell session/event
extraction remains outside this repair.

Successful writer files use the donor's mode0600. Empty failure markers retain
the shell creation mode. Empty metadata is omitted by existing CatalogEntry
tags. Root JSON null follows json.Unmarshal into a zero-value projected struct.

## Checks

The rawString fixture suite adapts eight existing Go JSON fixtures at
[golang/go@3901409b5d0fb7c85a3e6730a59943cc93b2835c](https://github.com/golang/go/tree/3901409b5d0fb7c85a3e6730a59943cc93b2835c)
under BSD-3-Clause: decode_test.go435–437,484,720–722,2030–2037,2103–2111,
2229,2238–2241; encode_test.go966–969,980,1004–1016; decode.go97–108.
Only fixture directions, result/error projection and helper call are adapted;
there is no copied test-only serializer.

Rendered-script hashes are measured from the candidate rather than selected
constants. The owning test records the measurement date and this note.

### Rendered-byte measurement, 2026-11-11

Command: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run
'^(TestRawString|TestPrimeScripts_RenderedBytesMatchBaseline)$'` on the candidate,
with old hash expectations, printed these SHA256 values before the expectations
were updated:

- Claude: `ffda573d45b4c245865790757f640828539f3472450d15211537b17230459daf`
- Codex: `27ca0820315f4e2b4c7de875aad8c2da154b384f090e5a284bf150017211a707`

The test still renders with the fixed `/usr/local/bin/rawclaw` baked path and
uses sha256.Sum256. These expectations pin the inspected native serializer
invocations and retained public claim sequence; they do not replace behavioral
hook tests.

### Native hook test delegation

Six existing outer hook tests build the actual CLI using the complete
`internal/live/loopback_test.go:43–64` helpers from
[RawClaw@61fac14e](https://github.com/MoonCaves/rawclaw/blob/61fac14e674095678e47cad202e4a255c381047f/internal/live/loopback_test.go#L43-L64),
with helper names changed. Their stubs delegate only `hook-catalog` to that
binary; detached ingest retains the original stub behavior and assertions.
The dispatch adopts `docker/entrypoint/docker-entrypoint.sh:36–39` from
[tarantool@ee34381e](https://github.com/tarantool/tarantool/blob/ee34381e9bf268bf9ae06ee48162137e0702f5bf/docker/entrypoint/docker-entrypoint.sh#L36-L39)
under BSD-2-Clause. The selected command and executable are substituted;
`shift` is retained and the fixed subcommand precedes the remaining arguments.
The original complete stub script follows unchanged. Stop-only and unavailable
catalog fixtures retain their existing stubs.
