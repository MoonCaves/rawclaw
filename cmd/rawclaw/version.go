package main

import (
	"runtime/debug"
)

// Build metadata, stamped at release time via -ldflags "-X main.version=..."
// (see .goreleaser.yml). If unstamped, runtime/debug.ReadBuildInfo() populates
// the VCS revision and timestamp automatically from Go toolchain metadata.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "unknown" && len(setting.Value) >= 7 {
					commit = setting.Value[:7]
				}
			case "vcs.time":
				if date == "unknown" {
					date = setting.Value
				}
			}
		}
	}
}
