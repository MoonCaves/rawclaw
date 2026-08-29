# Sovereign Agent Bus: Interactive Lockscreen Actions

All autonomous agents, background workers, supervisor harnesses, and CI gates must emit rich interactive notifications with tactile action buttons whenever human attention, approval, or triage is requested.

---

## 1. Invariants

- **Never Send Flat Messages**: A notification must never be raw unformatted text. Always include a clear **Title**, contextual **Tags/Emojis**, appropriate **Priority**, and **Action Buttons**.
- **Tactile Approvals**: Give the human direct 1-tap resolution options on their iPhone lockscreen and Apple Watch (e.g. \`🟢 Approve & Land\`, \`🔴 Hold & Reject\`, \`🔍 View Diff\`).
- **Private Mesh Routing**: Broadcast exclusively via the private NetBird WireGuard instance at \`https://ntfy.remotelyhuman.com/rh-jay\`.

---

## 2. Standard Interactive Payloads

### Pattern A: PR Ready / Code Merge Approval
\`\`\`bash
curl -s \
  -H "Title: 🚀 PR #84 Passed Checks: Ready to Land" \
  -H "Priority: 4" \
  -H "Tags: white_check_mark,robot" \
  -H "Actions: http, 🟢 Land PR, https://coolify.remotelyhuman.com/api/agent/approve?pr=84, method=POST, clear=true; \
               view, 🔍 View PR #84, https://github.com/MoonCaves/rawclaw/pull/84; \
               http, 🔴 Reject & Hold, https://coolify.remotelyhuman.com/api/agent/reject?pr=84, method=POST, clear=true" \
  -d "Adapter passed all race and conformance tests. Tap to merge into main." \
  "https://ntfy.remotelyhuman.com/rh-jay"
\`\`\`

### Pattern B: Supervisor / Agent Task Stalled (Human Gate)
\`\`\`bash
curl -s \
  -H "Title: ⚠️ Agent Gate: Task Blocked" \
  -H "Priority: 5" \
  -H "Tags: warning,rotating_light" \
  -H "Actions: http, ⚡ Relaunch Worker, https://coolify.remotelyhuman.com/api/agent/relaunch?task=task-123, method=POST, clear=true; \
               http, 🛑 Kill & Abort, https://coolify.remotelyhuman.com/api/agent/kill?task=task-123, method=POST, clear=true; \
               view, 📄 View Log, https://coolify.remotelyhuman.com/logs/task-123" \
  -d "Task task-123 exceeded silence threshold (400s). Choose an action." \
  "https://ntfy.remotelyhuman.com/rh-jay"
\`\`\`

### Pattern C: System Health / Uptime Alert
\`\`\`bash
curl -s \
  -H "Title: 🔴 Service Degradation: Muppet Server" \
  -H "Priority: 4" \
  -H "Tags: red_circle,server" \
  -H "Actions: http, 🔄 Restart Service, https://coolify.remotelyhuman.com/api/restart?service=glances, method=POST, clear=true; \
               view, 📊 Open Dashboard, https://glances.remotelyhuman.com" \
  -d "Connection timeout on 100.78.145.132:443. Tap to restart container." \
  "https://ntfy.remotelyhuman.com/rh-jay"
\`\`\`

---

## 3. Action Syntax Reference

Format: \`http, <Label>, <URL>, [method=POST], [headers.<Name>=<Value>], [body=<JSON>], [clear=true]\`
- \`http\`: Emits an HTTP request on tap (background execution).
- \`view\`: Opens a URL in mobile browser or deep-linked app.
- \`clear=true\`: Clears the notification immediately after successful action execution.
