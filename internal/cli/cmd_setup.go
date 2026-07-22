package cli

import (
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/paths"
	codexsrc "github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/spf13/cobra"
)

// newSetupCmd wires `rawclaw setup`: install the discovery-hook script and
// register it in every DETECTED target's config — Claude Code always
// (paths.ConfigDir — $CLAUDE_CONFIG_DIR or ~/.claude), plus Codex
// (codexsrc.ConfigDir — $CODEX_HOME or ~/.codex) when Codex exists on this
// machine. Default scope is GLOBAL: rawclaw searches across every project by
// default, so a global discovery hook is the honest default rather than a
// per-project one. --project narrows the write to the CURRENT project's own
// config instead — the explicit opt-in for anyone who wants the banner in one
// project only. --eject removes exactly what setup installed, across
// whichever targets and scope this invocation names.
func newSetupCmd() *cobra.Command {
	var yes, project, eject bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Wire the rawclaw discovery hook into Claude Code and Codex",
		Long: "Install the rawclaw discovery-hook script and register it as a SessionStart hook " +
			"in every agent runtime detected on this machine, so a session announces rawclaw " +
			"exists. For Claude Code a second, SessionEnd hook is wired too: it queues each " +
			"finished session for later topic tagging. The SessionStart discovery banner does " +
			"not surface that queue or ask a new session to tag an older one. Rawclaw itself " +
			"never calls a model. " +
			"Claude Code is always targeted; Codex is targeted too when its config dir " +
			"already exists (honors $CODEX_HOME, else ~/.codex) — a machine with no Codex install " +
			"is left untouched for that target rather than having a Codex tree created for it. " +
			"By default the hook is wired at the USER level (honors $CLAUDE_CONFIG_DIR, else " +
			"~/.claude): rawclaw searches every project, so a global hook is the honest default. " +
			"--project narrows the write to the CURRENT project's own config instead. " +
			"Every other hook already registered in any of these files — whatever tool it " +
			"belongs to — is left untouched, and re-running is safe: rawclaw's own " +
			"entry is replaced, never duplicated. --yes skips the interactive y/N prompt for " +
			"non-interactive/agent use.\n\n" +
			"--eject removes exactly what setup installed, across the same targets and scope: " +
			"the hook script and its now-empty directories are removed, and rawclaw's own " +
			"SessionStart entry is stripped out of each config file — deleting the file " +
			"entirely once nothing else is left in it. Every sibling hook is left untouched, " +
			"and a config file that still holds one survives with it intact. Ejecting on a " +
			"machine with nothing installed is a clean no-op. Known limitation: Codex may keep " +
			"a stale per-hook trust-state row in its own config after eject — that format is " +
			"undocumented and deliberately not touched here, so review it yourself if it matters.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if eject {
				return runSetupEject(cmd, yes, project)
			}
			return runSetup(cmd, yes, project)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the interactive y/N prompt (for non-interactive/agent use)")
	cmd.Flags().BoolVar(&project, "project", false,
		"wire the hook into the CURRENT project's own config instead of the user-level file")
	cmd.Flags().BoolVar(&eject, "eject", false,
		"remove exactly what setup installed (script, config entries, empty dirs) instead of installing")
	return cmd
}

// runSetup resolves each target's config dir at the requested scope (global by
// default, project-local under --project), shows the full plan, confirms once
// (unless --yes), then installs the hook into every detected target. Codex is
// gated on its USER-LEVEL config dir existing — whatever the scope, a machine
// with no Codex install never has a Codex tree created for it.
func runSetup(cmd *cobra.Command, yes, project bool) error {
	out := cmd.OutOrStdout()

	configDir, err := scopeConfigDir(project, paths.ConfigDir(), ".claude")
	if err != nil {
		return fmt.Errorf("resolve setup scope: %w", err)
	}
	scriptPath := hookScriptPath(configDir)
	sp := settingsPath(configDir)

	codexDetected := codexsrc.ConfigDir() != "" && isDir(codexsrc.ConfigDir())
	var codexDir string
	if codexDetected {
		codexDir, err = scopeConfigDir(project, codexsrc.ConfigDir(), ".codex")
		if err != nil {
			return fmt.Errorf("resolve codex setup scope: %w", err)
		}
	}

	maybePrintProjectTrustWarning(out, targetClaudeCode, project)
	if codexDetected {
		maybePrintProjectTrustWarning(out, targetCodex, project)
	}

	fmt.Fprintf(out, "rawclaw setup will:\n")
	fmt.Fprintf(out, "  install the discovery-hook script at %s\n", scriptPath)
	fmt.Fprintf(out, "  register it as a SessionStart hook in %s\n", sp)
	fmt.Fprintf(out, "  install the tagging-queue hook script at %s\n", tagQueueScriptPath(configDir))
	fmt.Fprintf(out, "  register it as a SessionEnd hook in %s (queues each finished session for topic tagging)\n", sp)
	if codexDetected {
		fmt.Fprintf(out, "  install the discovery-hook script at %s\n", hookScriptPath(codexDir))
		fmt.Fprintf(out, "  register it as a SessionStart hook in %s\n", codexHooksPath(codexDir))
	} else {
		fmt.Fprintf(out, "  Codex not detected (no config dir at %q) — skipping that target\n", codexsrc.ConfigDir())
	}
	fmt.Fprintf(out, "  (every other hook already registered in either file is left untouched)\n\n")

	if !yes {
		ok, err := confirm(cmd.InOrStdin(), out, "Proceed? [y/N]: ")
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if !ok {
			fmt.Fprintln(out, "Aborted; nothing written.")
			return nil
		}
	}

	if err := installRawclawHook(configDir); err != nil {
		return fmt.Errorf("install rawclaw hook: %w", err)
	}
	fmt.Fprintf(out, "Installed %s\nRegistered SessionStart hook in %s\n", scriptPath, sp)
	fmt.Fprintf(out, "Installed %s\nRegistered SessionEnd hook in %s\n", tagQueueScriptPath(configDir), sp)

	if codexDetected {
		if err := installRawclawCodexHook(codexDir); err != nil {
			return fmt.Errorf("install rawclaw codex hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\nRegistered SessionStart hook in %s\n", hookScriptPath(codexDir), codexHooksPath(codexDir))
	} else {
		fmt.Fprintln(out, "Codex not detected — skipped that target.")
	}

	// Point at the optional cross-machine archive without provisioning it: setup
	// wires local hooks; `archive init` is a separate opt-in the user runs when
	// they want backup + sync. One non-blocking line, never a prompt.
	fmt.Fprintln(out, "\nOptional — back up & sync your transcripts across machines:")
	fmt.Fprintln(out, "  rawclaw archive init <your-private-repo>   (see the archive section in the README)")

	return nil
}

// runSetupEject resolves each target's config dir at the requested scope
// exactly as runSetup does, shows the plan, confirms once (unless --yes), then
// ejects every detected target. Codex is gated on its USER-LEVEL config dir
// existing, same as install — nothing is created or touched for a target that
// was never wired up.
func runSetupEject(cmd *cobra.Command, yes, project bool) error {
	out := cmd.OutOrStdout()

	configDir, err := scopeConfigDir(project, paths.ConfigDir(), ".claude")
	if err != nil {
		return fmt.Errorf("resolve eject scope: %w", err)
	}
	scriptPath := hookScriptPath(configDir)
	sp := settingsPath(configDir)

	codexDetected := codexsrc.ConfigDir() != "" && isDir(codexsrc.ConfigDir())
	var codexDir string
	if codexDetected {
		codexDir, err = scopeConfigDir(project, codexsrc.ConfigDir(), ".codex")
		if err != nil {
			return fmt.Errorf("resolve codex eject scope: %w", err)
		}
	}

	fmt.Fprintf(out, "rawclaw setup --eject will:\n")
	fmt.Fprintf(out, "  remove the discovery-hook script at %s (if present)\n", scriptPath)
	fmt.Fprintf(out, "  remove the tagging-queue hook script at %s (if present)\n", tagQueueScriptPath(configDir))
	fmt.Fprintf(out, "  strip rawclaw's own entries out of %s (if present)\n", sp)
	if codexDetected {
		fmt.Fprintf(out, "  remove the discovery-hook script at %s (if present)\n", hookScriptPath(codexDir))
		fmt.Fprintf(out, "  strip rawclaw's own entry out of %s (if present)\n", codexHooksPath(codexDir))
	} else {
		fmt.Fprintf(out, "  Codex not detected (no config dir at %q) — skipping that target\n", codexsrc.ConfigDir())
	}
	fmt.Fprintf(out, "  (every sibling hook already registered in either file is left untouched)\n\n")

	if !yes {
		ok, err := confirm(cmd.InOrStdin(), out, "Proceed? [y/N]: ")
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if !ok {
			fmt.Fprintln(out, "Aborted; nothing removed.")
			return nil
		}
	}

	claudeOutcome, err := ejectRawclawHook(configDir)
	if err != nil {
		return fmt.Errorf("eject rawclaw hook: %w", err)
	}
	printEjectOutcome(out, "Claude Code", claudeOutcome)
	anyRemoved := claudeOutcome.didAnything()

	if codexDetected {
		codexOutcome, err := ejectRawclawCodexHook(codexDir)
		if err != nil {
			return fmt.Errorf("eject rawclaw codex hook: %w", err)
		}
		printEjectOutcome(out, "Codex", codexOutcome)
		anyRemoved = anyRemoved || codexOutcome.didAnything()
	} else {
		fmt.Fprintln(out, "Codex not detected — skipped that target.")
	}

	if !anyRemoved {
		fmt.Fprintln(out, "Nothing was installed for any detected target; eject is a clean no-op.")
	}

	return nil
}

// printEjectOutcome renders one target's eject outcome: a plain "already
// clean" note when there was nothing rawclaw-owned to remove, otherwise one
// line per thing actually removed.
func printEjectOutcome(out io.Writer, label string, o ejectOutcome) {
	if !o.didAnything() {
		fmt.Fprintf(out, "%s: nothing to remove (already clean).\n", label)
		return
	}
	if o.scriptRemoved {
		fmt.Fprintf(out, "%s: removed %s\n", label, o.scriptPath)
	}
	if o.tagScriptRemoved {
		fmt.Fprintf(out, "%s: removed %s\n", label, o.tagScriptPath)
	}
	switch {
	case o.fileDeleted:
		fmt.Fprintf(out, "%s: deleted %s (nothing else was left in it)\n", label, o.configFile)
	case o.entryRemoved:
		fmt.Fprintf(out, "%s: removed rawclaw's entry from %s\n", label, o.configFile)
	}
}
