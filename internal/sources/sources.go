// Package sources is the explicit composition root for built-in transcript
// adapters. Behavioral code consumes the source registry; adding a runtime is
// one registration here, not another branch in every command.
package sources

import (
	"sync"

	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/source/claude"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/source/goose"
)

var registerBuiltins sync.Once

// Registered wires the built-in adapters idempotently and returns every
// registered source, including any additional adapter registered by a caller.
func Registered() []source.Registration {
	registerBuiltins.Do(func() {
		source.Register(claude.Registration())
		source.Register(codex.Registration())
		source.Register(antigravity.Registration())
		source.Register(goose.Registration())
	})
	return source.Registered()
}
