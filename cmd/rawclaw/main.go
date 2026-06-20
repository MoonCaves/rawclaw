// Command rawclaw is the lightweight, headless, agent-facing search engine over
// the Claude Code transcript record (a single static pure-Go binary).
//
// Thin entry point: build the cobra command tree and execute it. All logic lives
// in internal/cli and the engine packages it composes.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/MoonCaves/rawclaw/internal/cli"
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	if err := cli.Execute(root, os.Args[1:]); err != nil {
		// ExitError carries an explicit exit code (and an optional message);
		// everything else is a generic exit 1.
		var ee cli.ExitError
		if errors.As(err, &ee) {
			if ee.Msg != "" {
				fmt.Fprintln(os.Stderr, ee.Msg)
			}
			os.Exit(ee.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
