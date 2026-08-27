# Issue #59 findings

- `internal/agentproto.outline` builds the goal/resolution arc with `store.BookendMessages` and renders it through `renderOutline`.
- The existing `store.LastMessages` tail reader already returns the newest stored event role, newest first. Reuse it rather than adding transcript parsing or sentiment detection.
- Add an optional outline result flag populated from that tail. Render one note only for an assistant tail with no later user event; leave the existing closed-session output unchanged.
- Graphify orientation was attempted with the supplied graph file; the checkout has no local `graphify-out/`, so source tracing was verified directly in the current tree.
