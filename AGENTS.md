# RawClaw — agent handbook

You're an agent or contributor working on RawClaw. Read this first: it's the north star, the
core-vs-seam shape, and the invariants that must not regress. Depth lives in the docs the Index
points at — this file is the map, not the territory.

## North star (do not drift)

**A single static binary, keyword search by default, zero runtime dependencies, no LLM, no API key.**
Pure Go, `CGO_ENABLED=0`. Anything that would drag a service, a model, or cgo into the *default*
path stays **optional and opt-in** — or it doesn't land. Every change is weighed against this; see
[ROADMAP.md](ROADMAP.md) for the standing constraint and the forward plan.

## Sovereign core, everything else on a seam

The core is small and self-contained: read local agent transcripts, index them (SQLite FTS5),
`search` / `read` / `outline`. Capability that isn't core rides a **seam**, so the core stays
sovereign and dependency-free:

- **Sources are adapters.** Claude Code and Codex today; more are new *readers*, not changes to
  search. A source teaches RawClaw to parse one transcript shape — nothing downstream should care
  which source a session came from.
- **The archive is any git remote — there is no RawClaw server.** Backup and cross-machine sync are
  a plain private git repo you point at (GitHub, GitLab, self-hosted, or a bare repo over SSH).
  RawClaw shells out to the system `git`; it stores no credentials and runs no daemon.
- **Transport and encryption ride git's own layers, not RawClaw code.** SSH/HTTPS carry the bytes;
  for at-rest encryption a repo-level tool (e.g. git-crypt) rides git's clean/smudge filters
  transparently under RawClaw's existing `git` calls. The core reimplements neither crypto nor
  transport. If a seam needs a hook (e.g. unlocking an encrypted clone after a rebuild), it's a thin
  adapter at the edge — never crypto in the core.
- **Discovery rides editor hooks.** `rawclaw setup` wires a POSIX-sh SessionStart/SessionEnd hook
  into Claude Code / Codex so a session learns RawClaw exists. It changes no editor behavior and is
  fully removable with `--eject`.

## Invariants (don't regress these)

- **Hooks fire regardless of PATH.** A generated hook resolves the binary by the absolute path
  `setup` bakes in (`os.Executable`), with a `command -v` fallback — a SessionStart/SessionEnd hook
  does **not** inherit an interactive login PATH, so a bare `command -v rawclaw` gate silently dies
  on any machine whose hook PATH lacks the binary's dir. Never gate a hook on a bare `command -v`.
- **`setup` wires; `archive init` provisions.** `setup` is local, surgical, and fully ejectable.
  Provisioning a remote is a *separate*, opt-in step the user runs. `setup` **points at** it; it
  never performs it.
- **Generated hooks are POSIX `sh` only** — no bash-isms; a SessionStart hook has no guaranteed
  bash. (This is why native Windows `setup` is currently unaddressed: the hook body assumes a POSIX
  shell.)
- **The intelligence lives in the help menus.** RawClaw is agent-native because `--help` on every
  verb carries the guidance an agent needs to use it right. When you add a capability or a choice
  (e.g. which kind of remote to back up to), put the steering in the **help text**, not only the
  README.
- **No silent truncation, no silent failure.** Search reports which scopes were searched / empty /
  skipped / stale; a trimmed `read` prints its own recovery command; a hook that can't resolve the
  binary exits 0 rather than erroring a session. An agent must never mistake partial for complete.

## Working rules

- Build, test, lint: see [CONTRIBUTING.md](CONTRIBUTING.md) — Go 1.24+, `CGO_ENABLED=0`,
  `go test -race`. Tests pass with the race detector before a change merges.
- Weigh every change against the north star above. Optional-and-opt-in is the price of admission for
  anything that isn't a single-binary, zero-dep, no-LLM default.

## Index

- [README.md](README.md) — what RawClaw is and its full verb/flag surface, from the user's side.
- [ROADMAP.md](ROADMAP.md) — the north-star constraints and the forward plan (planned / exploring / speculative).
- [CONTRIBUTING.md](CONTRIBUTING.md) — clean checkout → green build: prerequisites, build, test, lint.
- [design/](design/) — design notes for specific mechanisms.
