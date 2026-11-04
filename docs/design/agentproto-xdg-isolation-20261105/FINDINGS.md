# Agentproto fixture XDG isolation

At `e47a138a0df3099024dabc54e45180296d2e0cbf`, `TestTopicsCollapsesRepeatedLabel` gives itself a temporary `HOME` but inherits `XDG_DATA_HOME`. With `XDG_DATA_HOME` set to a fresh temporary directory, the focused test exits successfully while creating two synthetic files under that directory:

Reproduction with a fresh temporary XDG directory:

```sh
xdg_guard=$(mktemp -d /tmp/agentproto-xdg-guard.XXXXXX)
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 XDG_DATA_HOME="$xdg_guard" go test -count=1 ./internal/agentproto -run '^TestTopicsCollapsesRepeatedLabel$'
find "$xdg_guard" -type f -print | sed "s#^$xdg_guard/##" | sort
```

Observed output:

```text
ok  	github.com/MoonCaves/rawclaw/internal/agentproto	1.427s
rawclaw/transcripts/sessdup.jsonl
rawclaw/transcripts/sessdup.meta.json
```

The fixture should set its own temporary `XDG_DATA_HOME` alongside its temporary `HOME`, keeping synthetic transcript writes inside the test's temporary roots. The probe used only a newly created temporary XDG directory and did not inspect or access a real user vault. This is a test-isolation finding; it does not indicate a runtime defect.
