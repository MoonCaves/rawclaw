package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/MoonCaves/rawclaw/internal/paths"
	antigravitysrc "github.com/MoonCaves/rawclaw/internal/source/antigravity"
	codexsrc "github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/spf13/cobra"
)

// newSetupCmd wires `rawclaw setup`: install the discovery-hook script and
// register it in every DETECTED target's config — Claude Code always
// (paths.ConfigDir — $CLAUDE_CONFIG_DIR or ~/.claude), plus Codex
// (codexsrc.ConfigDir — $CODEX_HOME or ~/.codex) and Antigravity
// (antigravitysrc.ConfigDir — $ANTIGRAVITY_HOME or ~/.gemini/antigravity-cli) when
// they exist on this machine. Default scope is GLOBAL: rawclaw searches across
// every project by default, so a global discovery hook is the honest default rather
// than a per-project one. --project narrows the write to the CURRENT project's own
// config instead — the explicit opt-in for anyone who wants the banner in one
// project only. --eject removes exactly what setup installed, across
// whichever targets and scope this invocation names.
func newSetupCmd() *cobra.Command {
	var yes, project, eject bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Wire the rawclaw discovery hook into Claude Code, Codex, and Antigravity",
		Long: "Install the rawclaw discovery-hook script and register it as a discovery hook " +
			"in every agent runtime detected on this machine, so a session announces rawclaw " +
			"exists. The banner tells the current agent to delegate its own closeout tagging to " +
			"a background subagent when the user signals that the session is ending; it never asks " +
			"a new session to tag an older one. Rawclaw itself never calls a model. " +
			"Claude Code is always targeted; Codex and Antigravity are targeted too when their config dirs " +
			"already exist (honors $CODEX_HOME, else ~/.codex; $ANTIGRAVITY_HOME, else ~/.gemini/antigravity-cli) — " +
			"a machine with no Codex or Antigravity install is left untouched for those targets rather than having trees created for them. " +
			"By default the hook is wired at the USER level (honors $CLAUDE_CONFIG_DIR, else " +
			"~/.claude): rawclaw searches every project, so a global hook is the honest default. " +
			"--project narrows the write to the CURRENT project's own config instead. " +
			"Every other hook already registered in any of these files — whatever tool it " +
			"belongs to — is left untouched, and re-running is safe: rawclaw's own " +
			"entry is replaced, never duplicated. --yes skips the interactive y/N prompt for " +
			"non-interactive/agent use.\n\n" +
			"--eject removes exactly what setup installed, across the same targets and scope: " +
			"the hook script and its now-empty directories are removed, and rawclaw's own " +
			"entries are stripped out of each config file — deleting the file " +
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
	cmd.AddCommand(newSetupLiveCmd())
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

	antigravityDetected := antigravitysrc.ConfigDir() != "" && (isDir(antigravitysrc.ConfigDir()) || isDir(antigravitysrc.GlobalConfigDir()))
	var antigravityDir string
	if antigravityDetected {
		antigravityDir, err = scopeConfigDir(project, antigravitysrc.GlobalConfigDir(), ".agents")
		if err != nil {
			return fmt.Errorf("resolve antigravity setup scope: %w", err)
		}
	}

	piDetected := !project && (isDir(paths.ExpandHome("~/.pi")) || os.Getenv("PI_CODING_AGENT_DIR") != "")
	openCodeDetected := !project && (isDir(paths.ExpandHome("~/.config/opencode")) || isDir(paths.ExpandHome("~/.local/share/opencode")) || os.Getenv("OPENCODE_CONFIG_DIR") != "")
	gooseDetected := !project && (isDir(paths.ExpandHome("~/.config/goose")) || isDir(paths.ExpandHome("~/.local/share/goose")) || os.Getenv("GOOSE_HOME") != "")
	hermesDetected := !project && (isDir(paths.ExpandHome("~/.hermes")) || os.Getenv("HERMES_HOME") != "")

	maybePrintProjectTrustWarning(out, targetClaudeCode, project)
	if codexDetected {
		maybePrintProjectTrustWarning(out, targetCodex, project)
	}

	fmt.Fprintf(out, "rawclaw setup will:\n")
	fmt.Fprintf(out, "  install the discovery-hook script at %s\n", scriptPath)
	fmt.Fprintf(out, "  register it as a SessionStart hook in %s\n", sp)
	fmt.Fprintf(out, "  remove the legacy tagging-queue hook at %s and its SessionEnd entry (if present)\n", legacyTagQueueScriptPath(configDir))
	if codexDetected {
		fmt.Fprintf(out, "  install the discovery-hook script at %s\n", hookScriptPath(codexDir))
		fmt.Fprintf(out, "  register it as a SessionStart hook in %s\n", codexHooksPath(codexDir))
	} else {
		fmt.Fprintf(out, "  Codex not detected (no config dir at %q) — skipping that target\n", codexsrc.ConfigDir())
	}
	if antigravityDetected {
		fmt.Fprintf(out, "  install the discovery-hook script at %s\n", hookScriptPath(antigravityDir))
		fmt.Fprintf(out, "  register it as a PreInvocation hook in %s\n", antigravityHooksPath(antigravityDir))
	} else {
		fmt.Fprintf(out, "  Antigravity not detected (no config dir at %q) — skipping that target\n", antigravitysrc.ConfigDir())
	}
	if piDetected {
		fmt.Fprintf(out, "  install Pi catalog extension at %s\n", piExtensionPath())
	}
	if openCodeDetected {
		fmt.Fprintf(out, "  install OpenCode catalog plugin at %s\n", openCodePluginPath())
	}
	if gooseDetected {
		fmt.Fprintf(out, "  install Goose catalog hook in %s\n", goosePluginDir())
	}
	if hermesDetected {
		fmt.Fprintf(out, "  install Hermes catalog hook at %s\n", hermesAgentHooksScriptPath())
	}
	fmt.Fprintf(out, "  (every other hook already registered in any file is left untouched)\n\n")

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

	if codexDetected {
		if err := installRawclawCodexHook(codexDir); err != nil {
			return fmt.Errorf("install rawclaw codex hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\nRegistered SessionStart hook in %s\n", hookScriptPath(codexDir), codexHooksPath(codexDir))
	} else {
		fmt.Fprintln(out, "Codex not detected — skipped that target.")
	}

	if antigravityDetected {
		if err := installRawclawAntigravityHook(antigravityDir); err != nil {
			return fmt.Errorf("install rawclaw antigravity hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\nRegistered PreInvocation hook in %s\n", hookScriptPath(antigravityDir), antigravityHooksPath(antigravityDir))
	} else {
		fmt.Fprintln(out, "Antigravity not detected — skipped that target.")
	}

	if piDetected {
		if err := installPiBirthHook(); err != nil {
			return fmt.Errorf("install Pi hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\n", piExtensionPath())
	}
	if openCodeDetected {
		if err := installOpenCodeBirthHook(); err != nil {
			return fmt.Errorf("install OpenCode hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\n", openCodePluginPath())
	}
	if gooseDetected {
		if err := installGooseBirthHook(); err != nil {
			return fmt.Errorf("install Goose hook: %w", err)
		}
		fmt.Fprintf(out, "Installed Goose catalog hook in %s\n", goosePluginDir())
	}
	if hermesDetected {
		if err := installHermesBirthHook(); err != nil {
			return fmt.Errorf("install Hermes hook: %w", err)
		}
		fmt.Fprintf(out, "Installed %s\n", hermesAgentHooksScriptPath())
	}

	// Point at the optional cross-machine archive without provisioning it: setup
	// wires local hooks; `archive init` is a separate opt-in the user runs when
	// they want backup + sync. One non-blocking line, never a prompt.
	fmt.Fprintln(out, "\nMulti-Machine Setup (Using multiple machines?):")
	fmt.Fprintln(out, "  rawclaw archive init <your-private-repo>   # Back up & sync across machines via Git/Gitea")
	fmt.Fprintln(out, "  rawclaw setup live <user@remote-host>       # Auto-provision a remote machine for 1-step live peeks")

	return nil
}

// runSetupEject is the --eject flow: resolve the same target config dirs as
// runSetup, confirm once, then remove the hook script (and any empty parent
// dirs) and strip rawclaw's entries from the config files. Ejecting on a
// machine where rawclaw was never installed succeeds with a clean "already
// clean" message — never an error.
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

	antigravityDetected := antigravitysrc.ConfigDir() != "" && (isDir(antigravitysrc.ConfigDir()) || isDir(antigravitysrc.GlobalConfigDir()))
	var antigravityDir string
	if antigravityDetected {
		antigravityDir, err = scopeConfigDir(project, antigravitysrc.GlobalConfigDir(), ".agents")
		if err != nil {
			return fmt.Errorf("resolve antigravity eject scope: %w", err)
		}
	}

	piDetected := !project && (isDir(paths.ExpandHome("~/.pi")) || os.Getenv("PI_CODING_AGENT_DIR") != "")
	openCodeDetected := !project && (isDir(paths.ExpandHome("~/.config/opencode")) || isDir(paths.ExpandHome("~/.local/share/opencode")) || os.Getenv("OPENCODE_CONFIG_DIR") != "")
	gooseDetected := !project && (isDir(paths.ExpandHome("~/.config/goose")) || isDir(paths.ExpandHome("~/.local/share/goose")) || os.Getenv("GOOSE_HOME") != "")
	hermesDetected := !project && (isDir(paths.ExpandHome("~/.hermes")) || os.Getenv("HERMES_HOME") != "")

	fmt.Fprintf(out, "rawclaw setup --eject will:\n")
	fmt.Fprintf(out, "  remove the discovery-hook script at %s (and its parent dirs if empty)\n", scriptPath)
	fmt.Fprintf(out, "  strip rawclaw's SessionStart entry from %s\n", sp)
	fmt.Fprintf(out, "  remove the legacy tagging-queue hook at %s and its SessionEnd entry (if present)\n", legacyTagQueueScriptPath(configDir))
	if codexDetected {
		fmt.Fprintf(out, "  remove the discovery-hook script at %s (and its parent dirs if empty)\n", hookScriptPath(codexDir))
		fmt.Fprintf(out, "  strip rawclaw's SessionStart entry from %s\n", codexHooksPath(codexDir))
	} else {
		fmt.Fprintf(out, "  Codex not detected (no config dir at %q) — skipping that target\n", codexsrc.ConfigDir())
	}
	if antigravityDetected {
		fmt.Fprintf(out, "  remove the discovery-hook script at %s (and its parent dirs if empty)\n", hookScriptPath(antigravityDir))
		fmt.Fprintf(out, "  strip rawclaw's PreInvocation entry from %s\n", antigravityHooksPath(antigravityDir))
	} else {
		fmt.Fprintf(out, "  Antigravity not detected (no config dir at %q) — skipping that target\n", antigravitysrc.ConfigDir())
	}
	if piDetected {
		fmt.Fprintf(out, "  remove Pi catalog extension at %s\n", piExtensionPath())
	}
	if openCodeDetected {
		fmt.Fprintf(out, "  remove OpenCode catalog plugin at %s\n", openCodePluginPath())
	}
	if gooseDetected {
		fmt.Fprintf(out, "  remove Goose catalog hook in %s\n", goosePluginDir())
	}
	if hermesDetected {
		fmt.Fprintf(out, "  remove Hermes catalog hook at %s\n", hermesAgentHooksScriptPath())
	}
	fmt.Fprintf(out, "  (every other hook in any file is left untouched)\n\n")

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

	if antigravityDetected {
		antigravityOutcome, err := ejectRawclawAntigravityHook(antigravityDir)
		if err != nil {
			return fmt.Errorf("eject rawclaw antigravity hook: %w", err)
		}
		printEjectOutcome(out, "Antigravity", antigravityOutcome)
		anyRemoved = anyRemoved || antigravityOutcome.didAnything()
	} else {
		fmt.Fprintln(out, "Antigravity not detected — skipped that target.")
	}

	if piDetected {
		if fileExists(piExtensionPath()) {
			ejectPiBirthHook()
			fmt.Fprintf(out, "Pi: removed %s\n", piExtensionPath())
			anyRemoved = true
		} else {
			fmt.Fprintln(out, "Pi: nothing to remove (already clean).")
		}
	}
	if openCodeDetected {
		if fileExists(openCodePluginPath()) {
			ejectOpenCodeBirthHook()
			fmt.Fprintf(out, "OpenCode: removed %s\n", openCodePluginPath())
			anyRemoved = true
		} else {
			fmt.Fprintln(out, "OpenCode: nothing to remove (already clean).")
		}
	}
	if gooseDetected {
		if fileExists(goosePluginScriptPath()) || fileExists(goosePluginHookPath()) {
			ejectGooseBirthHook()
			fmt.Fprintf(out, "Goose: removed %s\n", goosePluginDir())
			anyRemoved = true
		} else {
			fmt.Fprintln(out, "Goose: nothing to remove (already clean).")
		}
	}
	if hermesDetected {
		if fileExists(hermesAgentHooksScriptPath()) {
			ejectHermesBirthHook()
			fmt.Fprintf(out, "Hermes: removed %s\n", hermesAgentHooksScriptPath())
			anyRemoved = true
		} else {
			fmt.Fprintln(out, "Hermes: nothing to remove (already clean).")
		}
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
