package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

type tagPrepLatencySample struct {
	foreground  time.Duration
	publication time.Duration
}

func runTagPrepLatencySample(t *testing.T, held bool) tagPrepLatencySample {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	t.Setenv("RAWCLAW_BACKGROUND_INGEST", "on")

	const sid = "latency-session-0001"
	path := filepath.Join(home, "session.jsonl")
	if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := source.Container{ID: sid, Path: path, CWD: home}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages:   []model.Message{{Role: "user", Text: "latency fixture", UUID: "11111111-latency"}},
	}
	reg := tagTestRegistration("latency-test", src)

	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("create consolidated store: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		con.Close()
		t.Fatalf("rebuild consolidated store: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		con.Close()
		t.Fatalf("ensure topic schema: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	lock := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	if held {
		locked, err := lock.TryLock()
		if err != nil || !locked {
			t.Fatalf("hold consolidated.lock: locked=%t err=%v", locked, err)
		}
		defer lock.Unlock()
	}

	dbp := index.RefreshDBPath(reg.ID, sid, path)
	publicationDone := make(chan time.Time, 1)
	oldSpawn := spawnIngest
	spawnIngest = func(string) {
		go func() {
			started := time.Now()
			_ = index.SyncConsolidatedFrom(dbp)
			publicationDone <- started
		}()
	}
	t.Cleanup(func() { spawnIngest = oldSpawn })

	started := time.Now()
	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("tag-prep held=%t: %v", held, err)
	}
	foreground := time.Since(started)
	if !strings.Contains(out.String(), "11111111 [user] latency fixture") {
		t.Fatalf("tag-prep output missing fixture: %q", out.String())
	}
	if held {
		select {
		case <-publicationDone:
			t.Fatal("publication completed while consolidated.lock was held")
		default:
		}
		// Keep the lock held after the foreground return so publication latency
		// includes a controlled fence wait rather than a coincidental handoff.
		time.Sleep(250 * time.Millisecond)
		if err := lock.Unlock(); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-publicationDone:
	case <-time.After(5 * time.Second):
		t.Fatal("detached publication did not complete")
	}
	return tagPrepLatencySample{foreground: foreground, publication: time.Since(started)}
}

func summarizeTagPrepLatency(samples []tagPrepLatencySample) string {
	foreground := make([]time.Duration, len(samples))
	publication := make([]time.Duration, len(samples))
	for i, sample := range samples {
		foreground[i], publication[i] = sample.foreground, sample.publication
	}
	sort.Slice(foreground, func(i, j int) bool { return foreground[i] < foreground[j] })
	sort.Slice(publication, func(i, j int) bool { return publication[i] < publication[j] })
	p := func(values []time.Duration, q int) time.Duration { return values[(len(values)-1)*q/100] }
	return fmt.Sprintf("n=%d foreground p50=%s p95=%s max=%s publication p50=%s p95=%s max=%s", len(samples), p(foreground, 50), p(foreground, 95), foreground[len(foreground)-1], p(publication, 50), p(publication, 95), publication[len(publication)-1])
}

// TestTagPrepLatencyAndHeldConsolidatedFence measures foreground return and
// detached fold completion separately across ten fresh stores per condition.
func TestTagPrepLatencyAndHeldConsolidatedFence(t *testing.T) {
	if testing.Short() {
		t.Skip("latency evidence test")
	}
	for _, held := range []bool{false, true} {
		name := "unlocked"
		if held {
			name = "held-consolidated-lock"
		}
		t.Run(name, func(t *testing.T) {
			samples := make([]tagPrepLatencySample, 10)
			for i := range samples {
				samples[i] = runTagPrepLatencySample(t, held)
			}
			t.Logf("%s", summarizeTagPrepLatency(samples))
		})
	}
}
