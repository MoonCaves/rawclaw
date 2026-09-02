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
	"github.com/MoonCaves/rawclaw/internal/source/hermes"
	"github.com/MoonCaves/rawclaw/internal/source/opencode"
	"github.com/MoonCaves/rawclaw/internal/source/pi"
)

var mu sync.Mutex

// Registered wires the built-in adapters idempotently and returns every
// registered source, including any additional adapter registered by a caller.
func Registered() []source.Registration {
	mu.Lock()
	defer mu.Unlock()
	source.Register(claude.Registration())
	source.Register(codex.Registration())
	source.Register(antigravity.Registration())
	source.Register(goose.Registration())
	source.Register(pi.Registration())
	source.Register(opencode.Registration())
	source.Register(hermes.Registration())
	return source.Registered()
}

// Get returns the registration for source ID, or nil if not registered.
func Get(id string) *source.Registration {
	for _, reg := range Registered() {
		if reg.ID == id {
			return &reg
		}
	}
	return nil
}
