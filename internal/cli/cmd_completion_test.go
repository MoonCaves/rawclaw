package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionShellGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell    string
		contains []string
	}{
		{
			shell: "bash",
			contains: []string{
				"__rawclaw",
				"complete",
			},
		},
		{
			shell: "zsh",
			contains: []string{
				"#compdef rawclaw",
			},
		},
		{
			shell: "fish",
			contains: []string{
				"complete -c rawclaw",
			},
		},
		{
			shell: "powershell",
			contains: []string{
				"Register-ArgumentCompleter",
				"rawclaw",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			cmd := NewRootCmd(BuildInfo{})
			var out, errb bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errb)
			cmd.SetArgs([]string{"completion", tc.shell})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("completion %s failed: %v", tc.shell, err)
			}

			output := out.String()
			if len(output) == 0 {
				t.Fatalf("completion %s produced empty output", tc.shell)
			}

			for _, sub := range tc.contains {
				if !strings.Contains(output, sub) {
					t.Errorf("completion %s output missing %q", tc.shell, sub)
				}
			}
		})
	}
}

func TestCompletionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no args",
			args:    []string{"completion"},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "too many args",
			args:    []string{"completion", "bash", "extra"},
			wantErr: "accepts 1 arg(s), received 2",
		},
		{
			name:    "unsupported shell",
			args:    []string{"completion", "elvish"},
			wantErr: `unsupported shell type "elvish"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewRootCmd(BuildInfo{})
			var out, errb bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errb)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestFlagCompletions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		flag        string
		toComplete  string
		wantEntries []string
	}{
		{
			name:        "role all options",
			flag:        "role",
			toComplete:  "",
			wantEntries: []string{"user", "assistant"},
		},
		{
			name:        "role filtered prefix",
			flag:        "role",
			toComplete:  "u",
			wantEntries: []string{"user"},
		},
		{
			name:        "sort all options",
			flag:        "sort",
			toComplete:  "",
			wantEntries: []string{"newest", "oldest"},
		},
		{
			name:        "sort filtered prefix",
			flag:        "sort",
			toComplete:  "o",
			wantEntries: []string{"oldest"},
		},
		{
			name:        "source all options",
			flag:        "source",
			toComplete:  "",
			wantEntries: []string{"claude", "codex", "antigravity"},
		},
		{
			name:        "source filtered prefix",
			flag:        "source",
			toComplete:  "anti",
			wantEntries: []string{"antigravity"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewRootCmd(BuildInfo{})
			var out, errb bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errb)
			cmd.SetArgs([]string{"__complete", "--" + tc.flag, tc.toComplete})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("__complete failed: %v", err)
			}

			output := out.String()
			lines := strings.Split(strings.TrimSpace(output), "\n")
			// The last line of __complete output is the directive (e.g. :4 for ShellCompDirectiveNoFileComp)
			if len(lines) == 0 {
				t.Fatalf("no completion output returned")
			}
			directive := lines[len(lines)-1]
			if !strings.HasPrefix(directive, ":") {
				t.Errorf("expected completion directive on last line, got %q", directive)
			}

			var gotItems []string
			for _, line := range lines[:len(lines)-1] {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					gotItems = append(gotItems, trimmed)
				}
			}

			for _, want := range tc.wantEntries {
				found := false
				for _, got := range gotItems {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("flag --%s completion missing %q in %v", tc.flag, want, gotItems)
				}
			}
		})
	}
}

func TestCompletionValidArgs(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(BuildInfo{})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"__complete", "completion", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("__complete completion failed: %v", err)
	}

	output := out.String()
	for _, expectedShell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(output, expectedShell) {
			t.Errorf("completion valid args missing shell %q in output:\n%s", expectedShell, output)
		}
	}
}
