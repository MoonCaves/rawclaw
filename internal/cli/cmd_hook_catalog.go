/*
MIT License

Copyright (c) 2025 Ashesh Goplani

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/spf13/cobra"
)

// Lifted from asheshgoplani/agent-deck cmd/agent-deck/hook_handler.go L56-L65 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
// rawclaw: project the lenient field pattern onto the existing catalog's cwd and transcript_path keys.
type hookCatalogPayload struct {
	TranscriptPath json.RawMessage `json:"transcript_path"`
	CWD            json.RawMessage `json:"cwd"`
}

// Lifted from MoonCaves/rawclaw internal/cli/cmd_prewarm.go L28-L40 (MIT) @61fac14e674095678e47cad202e4a255c381047f.
// rawclaw: Donor symbol newPrewarmCmd maps to newHookCatalogCmd; this is not a whole-function copy.
// rawclaw: Hidden, SilenceUsage and SilenceErrors retain the donor's literal true values.
// rawclaw: Use/Short strings and callback are substituted; Args/ExactArgs and RunE/error return
// rawclaw: are omitted. Local flag bindings and cmd return replace the direct literal return.
/* Verbatim donor excerpt, cmd_prewarm.go L28-L40:
func newPrewarmCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "prewarm <session8>",
		Short:         "Prepare a session closeout dump",
		Hidden:        true,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrewarmCmd(cmd.OutOrStdout(), args[0], nil, nil, sources.Registered())
		},
	}
}
*/
func newHookCatalogCmd() *cobra.Command {
	var output, sessionID, source string
	cmd := &cobra.Command{
		Use:           "hook-catalog",
		Short:         "Write private session catalog metadata",
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			runHookCatalog(cmd.InOrStdin(), output, sessionID, source)
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "existing private catalog entry path")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "shell-selected session identifier")
	cmd.Flags().StringVar(&source, "source", "", "fixed hook runtime source")
	return cmd
}

func runHookCatalog(r io.Reader, output, sessionID, source string) {
	// Lifted from asheshgoplani/agent-deck cmd/agent-deck/codex_hooks_cmd.go L192-L192 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
	// rawclaw: reader pass-through and local result name only; no input limit.
	data, err := io.ReadAll(r)
	// Lifted from asheshgoplani/agent-deck cmd/agent-deck/hook_handler.go L370-L377 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
	// rawclaw: project the decoded payload type onto catalog metadata.
	if err != nil || len(data) == 0 {
		return
	}
	var payload hookCatalogPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	// Lifted from MoonCaves/rawclaw internal/paths/paths.go L101-L106 (MIT) @61fac14e674095678e47cad202e4a255c381047f.
	// rawclaw: bind existing shell-selected flags and decoded fields to the existing catalog schema.
	entry := paths.CatalogEntry{
		SessionID:      sessionID,
		TranscriptPath: rawString(payload.TranscriptPath),
		CWD:            rawString(payload.CWD),
		Source:         source,
	}
	// Lifted from asheshgoplani/agent-deck cmd/agent-deck/hook_handler.go L893-L912 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
	// rawclaw: serialize CatalogEntry, pass the complete existing path, and silently return on errors; omit status logs/path derivation.
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := atomicHookWrite(output, jsonData); err != nil {
		return
	}
}

// Lifted from asheshgoplani/agent-deck cmd/agent-deck/hook_handler.go L98-L106 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
// rawString reads a leniently decoded field as a string ("" for anything
// that is not a JSON string).
func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// Lifted from asheshgoplani/agent-deck cmd/agent-deck/hook_handler.go L968-L985 (MIT) @2e897f2a62f6dd570628d2021a0994a3a7db1f4f.
func atomicHookWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) //nolint:errcheck // upstream best-effort deferred temp cleanup
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
