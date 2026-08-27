# Supervisor session closeout

This document defines what closeout means in a RawClaw supervisor or harness context. It is an
operational boundary, not a conversational sign-off.

## Trigger semantics

When the operator says **`close out this session`**, or an unambiguous equivalent, the agent must
begin operational closeout. The agent must stop accepting or launching new work and complete the
checklist below before declaring the session closed.

The phrase **`end the conversation only`**, or an equally explicit equivalent, is a separate
request. It ends the conversation without performing operational closeout. The agent must state
that distinction plainly. An ambiguous phrase must not be treated as permission to claim that
operational closeout happened.

## Operational checklist

Closeout is complete only when each item has an observed result, or is explicitly recorded as
unobserved or held in the closeout receipt.

1. **Stop new work.** Do not launch additional workers or accept new work after closeout begins.
2. **Drain supervisor mail.** Read every unread supervisor message, act on applicable directives,
   and record anything that could not be acted on.
3. **Inspect live and persisted work state.** Inspect every worker process, branch, dirty
   worktree, upstream state, and worker output.
4. **Preserve owned work.** Commit and push completed owned work. For incomplete work, preserve it
   and report its exact state; never silently discard it.
5. **Stop scheduled activity.** Stop the session ticker, watchdog, and related scheduled task,
   then verify that each one is actually stopped.
6. **Reconcile external state safely.** Reconcile integration candidates, abandoned branches,
   temporary worktrees, and remaining external state. Do not delete ambiguous work.
7. **Run final gates honestly.** Run the required final gates, or state exactly which gates remain
   unobserved and why.
8. **Emit one closeout receipt.** Produce one evidence-backed receipt listing what stopped, what
   was preserved, what remains active, and every explicit hold.

## Completion and safety rules

The receipt is the completion marker. The agent must not say **`session closed`** until the one
closeout receipt exists. A partial checklist is a partial closeout, never a successful closeout;
the agent must report the missing checks and remain explicit about any active work.

Closeout is idempotent and non-destructive. Repeating it rechecks the current state and does not
delete or overwrite ambiguous work. If a stop, inspection, push, or gate cannot be verified, the
receipt must say so rather than implying success.

For a conversation-only request, no operational mutation is required or implied. The response
must say that the conversation ended only and must not call that state **`session closed`**.
