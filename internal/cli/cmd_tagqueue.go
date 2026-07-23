package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// newTagQueueCmd wires `rawclaw tag-queue`: the pending-tags queue behind
// automatic topic tagging. The SessionEnd hook `rawclaw setup` installs calls
// `tag-queue add` when a session finishes. The queue is available to later
// topic-tagging workflows (tag-prep → tag-write; tag-write dequeues on success).
// Bare `tag-queue` prints one 8-char session id per line — silence means an
// empty queue.
func newTagQueueCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "tag-queue",
		Short: "List sessions queued for topic tagging (add/remove to manage)",
		Long: "The pending-tags queue behind automatic topic tagging. `rawclaw setup` wires a SessionEnd " +
			"hook that queues each finished session here. The SessionStart discovery banner does not surface " +
			"this queue or ask a new session to tag an older one. A successful `tag-write` dequeues its session. " +
			"Bare `tag-queue` prints one 8-char session id per line — no output means an empty queue. " +
			"rawclaw calls NO LLM.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagQueueList(cmd.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")

	cmd.AddCommand(&cobra.Command{
		Use:   "add <session-id>",
		Short: "Queue a session for topic tagging (called by the SessionEnd hook)",
		Long: "Append a finished session's id to the tagging queue. Idempotent — re-adding a queued id " +
			"is a no-op. This is the verb the SessionEnd hook installed by `rawclaw setup` calls; it only " +
			"touches the queue file, never the index.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagQueueAdd(args[0])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <session-id>",
		Short: "Drop a session from the tagging queue without tagging it",
		Long: "Remove a queue entry by id or unambiguous prefix — for a session that won't resolve " +
			"(deleted, or never indexed) or isn't worth tagging. A successful `tag-write` dequeues " +
			"automatically, so this is only for skipping.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := tagQueueRemove(args[0])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("no queue entry matches %q", args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s from the tagging queue\n", args[0])
			return nil
		},
	})

	return cmd
}

// runTagQueueList prints the queue, one 8-char session id per line (the form
// tag-prep/tag-write take), oldest first. An empty queue prints nothing and
// exits 0.
func runTagQueueList(w io.Writer, jsonOut bool) error {
	ids, err := readTagQueue()
	if err != nil {
		return err
	}
	if jsonOut {
		type entry struct {
			SessionID string `json:"session_id"`
			Session8  string `json:"session8"`
		}
		entries := make([]entry, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, entry{SessionID: id, Session8: lastSlice8(id)})
		}
		return json.NewEncoder(w).Encode(map[string]any{"pending": entries})
	}
	for _, id := range ids {
		fmt.Fprintln(w, lastSlice8(id))
	}
	return nil
}
