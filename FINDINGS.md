# Issue 47 findings

- `runRoot` is the only blocker: `case o.ThisProject` rejects a valid composition.
- `runBrowse` already resolves `--this-project` with `thisScope` before calling `runBrowseScoped`.
- `runBrowseScoped` passes `o.Source` and the resolved project label to `view.BrowseScoped`; search passes both through `SearchOpts`.
- Scope: remove only the ThisProject refusal and replace its focused refusal assertion with success/filter coverage. Preserve List and Resume refusals exactly.
