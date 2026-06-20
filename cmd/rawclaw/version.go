package main

// Build metadata, stamped at release time via -ldflags "-X main.version=..."
// (see .goreleaser.yml). The defaults make a plain `go build` / `go run` honest:
// an un-stamped binary reports "dev", not a fake tag.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)
