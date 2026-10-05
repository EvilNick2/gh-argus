package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/store"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return now }

var ciRuns = []runs.Run{{ID: 7, RunNumber: 7, Name: "ci", Status: "completed", Conclusion: "success"}}

func TestEmptyHasNoSeed(t *testing.T) {
	s, err := Load(store.New(t.TempDir()), clock)
	if err != nil {
		t.Fatal(err)
	}
	if seed := s.Seed("o/r"); seed != nil {
		t.Errorf("got seed %+v", seed)
	}
}

func TestRecordFlushLoadRoundTrips(t *testing.T) {
	st := store.New(t.TempDir())
	s, _ := Load(st, clock)

	s.Record(watch.Event{Repo: "o/r", Initial: true, Runs: ciRuns, ETag: `W/"a"`})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(st, clock)
	if err != nil {
		t.Fatal(err)
	}
	seed := again.Seed("o/r")
	if seed == nil || seed.ETag != `W/"a"` || len(seed.Runs) != 1 || seed.Runs[0].Name != "ci" {
		t.Errorf("got seed %+v", seed)
	}
}

func TestFlushWithoutChangesDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(store.New(dir), clock)

	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("snapshots.json written with nothing recorded: %v", err)
	}
}

func TestRecordIgnoresErrorsAndEventsWithoutETag(t *testing.T) {
	s, _ := Load(store.New(t.TempDir()), clock)

	s.Record(watch.Event{Repo: "o/a", Runs: ciRuns, ETag: `W/"a"`, Err: errors.New("jobs failed")})
	s.Record(watch.Event{Repo: "o/b", Runs: ciRuns})
	if s.Seed("o/a") != nil || s.Seed("o/b") != nil {
		t.Error("recorded an event it should have ignored")
	}
}

func TestFlushPrunesReposNotSeenFor30Days(t *testing.T) {
	st := store.New(t.TempDir())
	old, _ := Load(st, func() time.Time { return now.Add(-31 * 24 * time.Hour) })
	old.Record(watch.Event{Repo: "o/old", Initial: true, Runs: ciRuns, ETag: `"x"`})
	old.Flush()
	recent, _ := Load(st, func() time.Time { return now.Add(-29 * 24 * time.Hour) })
	recent.Record(watch.Event{Repo: "o/recent", Initial: true, Runs: ciRuns, ETag: `"y"`})
	recent.Flush()

	s, _ := Load(st, clock)
	s.Record(watch.Event{Repo: "o/today", Initial: true, Runs: ciRuns, ETag: `"z"`})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	after, _ := Load(st, clock)
	if after.Seed("o/old") != nil {
		t.Error("31 day old repo not pruned")
	}
	if after.Seed("o/recent") == nil || after.Seed("o/today") == nil {
		t.Error("recent repos pruned")
	}
}

func TestConcurrentRecordAndFlush(t *testing.T) {
	s, _ := Load(store.New(t.TempDir()), clock)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			s.Record(watch.Event{Repo: "o/r" + string(rune('a'+i)), Initial: true, Runs: ciRuns, ETag: `"e"`})
			s.Flush()
		})
	}
	wg.Wait()
}

func TestSeedOnceSeedsOnlyTheFirstWatchOfARepo(t *testing.T) {
	st := store.New(t.TempDir())
	s, _ := Load(st, clock)
	s.Record(watch.Event{Repo: "o/r", Initial: true, Runs: ciRuns, ETag: `"e"`})

	if s.SeedOnce("o/r") == nil {
		t.Fatal("first watch got no seed")
	}
	if seed := s.SeedOnce("o/r"); seed != nil {
		t.Errorf("second watch in the same session got seed %+v", seed)
	}
	if s.SeedOnce("o/other") != nil {
		t.Error("repo with nothing saved got a seed")
	}
}
