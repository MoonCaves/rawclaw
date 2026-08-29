# Sovereign Agent Bus: Interactive Lockscreen Actions

All autonomous agents, background workers, supervisor harnesses, and CI gates must emit rich interactive notifications with tactile action buttons whenever human attention, approval, or triage is requested.

---

## 1. Invariants

- **Never Send Flat Messages**: A notification must never be raw unformatted text. Always include a clear **Title**, contextual **Tags/Emojis**, appropriate **Priority**, and **Action Buttons**.
- **Tactile Approvals**: Give the human direct 1-tap resolution options on their iPhone lockscreen and Apple Watch (e.g. `🟢 Approve & Land`, `🔴 Hold & Reject`, `🔍 View Diff`, `📋 Copy`).
- **Private Mesh Routing**: Broadcast exclusively via the private NetBird WireGuard instance at `https://ntfy.remotelyhuman.com/rh-jay`.

---

## 2. The 5 Standard Action Types

| Action Type | Syntax Example | Behavior |
| :--- | :--- | :--- |
| **1. HTTP Callback (`http`)** | `http, 🟢 Land PR, https://.../approve?pr=84, method=POST, clear=true` | Emits HTTP request to server in background and auto-dismisses card. |
| **2. Deep-Link (`view`)** | `view, 🔍 View PR, https://github.com/MoonCaves/rawclaw/pull/84` | Opens web URL or native app scheme in Safari/browser. |
| **3. Clipboard Copy (`copy`)** | `copy, 📋 Copy Command, rawclaw --stats, clear=true` | Copies text directly to iOS clipboard on tap. |
| **4. iOS Shortcut (`view`)** | `view, 🎙️ Voice Reply, shortcuts://run-shortcut?name=AskFleet` | Triggers a native Siri Shortcut for ambient voice response. |
| **5. Rich Attachment (`Attach`)** | `-H "Attach: https://.../graph.png"` | Renders expandable image/artifact directly on lockscreen. |

---

## 3. Production Payloads

### Pattern A: PR Ready / Code Merge Approval
```bash
curl -s \
  -H "Title: 🚀 PR #84 Passed Checks: Ready to Land" \
  -H "Priority: 4" \
  -H "Tags: white_check_mark,robot" \
  -H "Actions: http, 🟢 Land PR, https://coolify.remotelyhuman.com/api/agent/approve?pr=84, method=POST, clear=true; \
               view, 🔍 View PR #84, https://github.com/MoonCaves/rawclaw/pull/84; \
               http, 🔴 Reject & Hold, https://coolify.remotelyhuman.com/api/agent/reject?pr=84, method=POST, clear=true" \
  -d "Adapter passed all race and conformance tests. Tap to merge into main." \
  "https://ntfy.remotelyhuman.com/rh-jay"
```

### Pattern B: Supervisor Session Resume String (1-Tap Copy)
```bash
curl -s \
  -H "Title: 🔑 Supervisor: Fresh Resume String" \
  -H "Priority: 4" \
  -H "Tags: key,rocket,clipboard" \
  -H "Actions: copy, 📋 Copy Resume Command, opencode --session ses_17591f41dffeWhwY35U2RuhERJ, clear=true; \
               view, 🔍 View Session, https://github.com/MoonCaves/rawclaw/pull/83" \
  -d "Tap to copy the exact CLI resume command to your clipboard." \
  "https://ntfy.remotelyhuman.com/rh-jay"
```

### Pattern C: Long-Running Task Completion Hook
```bash
notify_on_complete() {
  local start=$(date +%s)
  "$@"
  local status=$?
  local duration=$(( $(date +%s) - start ))
  if [ $duration -gt 45 ]; then
    local title="✅ Task Finished (${duration}s)"
    [ $status -ne 0 ] && title="❌ Task Failed (${duration}s)"
    curl -s \
      -H "Title: $title" \
      -H "Priority: 3" \
      -H "Tags: hourglass_flowing_sand,gear" \
      -d "Command: $* (Exit: $status)" \
      "https://ntfy.remotelyhuman.com/rh-jay" >/dev/null
  fi
  return $status
}
```
