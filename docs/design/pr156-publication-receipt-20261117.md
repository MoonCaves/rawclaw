# Recovered PR156 publication rejection receipt

Source: the original command-completion record dated 2026-11-13T05:43:38.223Z,
recovered on 2026-11-17. The command, exit code and aggregated output below are
copied from that record. The classification and limits are recorded here.
This is a historical receipt; it grants no publication or merge approval.

## Command and exact range

```sh
git push origin 7ece05dedb55518682bfc13122548e1dc6b20b49:refs/heads/test/topics-order-contract-tick721
```

Exit code: `1`. The existing-branch push reviews
`534d80e1036cace1b8953dcde443fcf2afe00cba..7ece05dedb55518682bfc13122548e1dc6b20b49`.
The recorded internal-patch hash prefix is `e5ff3e26a3ed`.

The separate main-relative range
`eb0529059c150cd439842b99d6624f7acbfedce9..7ece05dedb55518682bfc13122548e1dc6b20b49`
received an approval with hash prefix `1e1122d32bbb`. That narrower range does
not replace the rejected existing-branch push range.

## Recovered aggregated output

SHA-256 of the UTF-8 output inside the following code block, including its final
newline: `13deddde32985fa68a6f52bc1d7d468ca70e81cea0d16fe51b4f1772f240927d`.

```text
[provenance-gate] wiring-only declared; model still checks that it is wiring.
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. Donor command constructor pattern cited for internal/cli/cmd_hook_catalog.go
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. Wiring/AddCommand pattern cited for internal/cli/cli.go
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. Doctrine allows CLI flag definitions and help text as ours; shape mirrors cited donor command constructor
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck hook_handler.go L56-L65 cited for this file
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck hook_handler.go L98-L106 cited verbatim
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck codex_hooks_cmd.go L192 cited
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck hook_handler.go L370-L377 cited
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. rawclaw internal/paths/paths.go L101-L106 cited
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck hook_handler.go L893-L912 cited
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. agent-deck hook_handler.go L968-L985 cited verbatim
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. rawclaw internal/cli/setup.go L26-L61 cited for internal/cli/setup.go; retained lines are verbatim from donor
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. rawclaw internal/cli/setup.go L47-L61 cited verbatim for this file
  MADE None: No cited upstream line contains a pipeline that feeds the hook payload into a subcommand writing the catalog entry; the cited setup.go donor writes JSON with printf inside the braces. The substitution of an external serializer invocation (and the specific flag composition) is the agent's choice, not found in any fetched bytes.
  MADE None: tarantool docker-entrypoint.sh L36-L39 supplies `if [ "$1" = "tarantool" ]; then shift; exec tarantool "$@"` but the re-insertion of the fixed subcommand after shift plus the single-quoted binary path substitution and falling through to the original stub script is a combination not present in any cited source (composition invariant).
  MADE None: The deleted branch is lifted code: the cited rawclaw internal/cli/catalog_hook_test.go L141-L144 still contains `timeout = 15 * time.Second` inside the conditional, so removing the branch's control flow contradicts the cited source; policy labels cover numbers only, never control flow.
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. Doctrine allows help text as product shape/ours
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. rawclaw internal/live/loopback_test.go L43-L64 cited for these files; renamed copies
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. golang/go encode_test.go/decode_test.go fixtures cited per row for this file; empty-result expectation from agent-deck rawString
  MADE None: FOUND without matching quote in sources scoped to  -> MADE. golang/go encode_test.go L1004-L1016 cited
[provenance-gate] REJECT (claude-sub/claude-opus-5 via gateway): a decision above was MADE by an agent instead of FOUND on the internet. Cite the source line in a Prior-Art trailer, or label a measured constant // policy:.
error: failed to push some refs to 'github.com:MoonCaves/rawclaw.git'
```

## What this recovers

The record contains 19 `MADE` lines: 16 quote-backstop downgrades and three
semantic source-composition findings. Earlier three-finding excerpts did not
preserve the full printed rejection inventory.

All 19 printed file labels are `None`. This does not establish whether the
original provider response omitted a file field or supplied null, nor recover
its original quotations. Only three of the 16 quote-downgrade reasons explicitly
identify target paths: `internal/cli/cmd_hook_catalog.go`, `internal/cli/cli.go`
and `internal/cli/setup.go`. Other filenames in reasons identify source hints;
they do not establish a missing target association.

The three semantic findings concern the payload-to-catalog command pipeline in
`internal/cli/setup.go`, the wrapper dispatch composition in
`internal/cli/cmd_hook_catalog_test.go`, and removal of a conditional timeout in
`internal/cli/catalog_hook_test.go`. Those three files are byte-identical between
candidate `7ece05dedb55518682bfc13122548e1dc6b20b49` and main
`e0420b85a30f2b4f562d72e531bcab64f23b6aa9`. This establishes unchanged source,
not approval of the rejected compositions.

The original provider JSON and temporary fetched-source scope bundle remain
unrecovered. The printed output supports the inventory above, not a complete
root-cause determination or a replacement provenance verdict. Shape validation
alone is not evidence that the three semantic findings are resolved.

No gate was rerun to obtain this receipt. No implementation, gate, schema,
provider configuration, or branch-publication rule is changed here. The rejected
integration remains held pending a separately authorized resolution.
