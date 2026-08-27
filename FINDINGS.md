# Issue #47 findings

Lean already. `runRoot` rejects `--source` with `--this-project`, but the existing browse and search paths already pass the source filter through their scoped interfaces. Remove only that refusal; retain the `--list` and `--resume` refusals. Do not use the Issue #45 regex workaround.

net: 0 lines before implementation.
