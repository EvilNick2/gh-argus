// Package snapshot keeps each watched repo's latest runs and their ETag
// between sessions, so the next launch can show them at once and start with a
// conditional request.
package snapshot

import (
	"sync"
	"time"

	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/store"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

const fileName = "snapshots"

// maxAge is how long a repo that is no longer watched keeps its entry.
const maxAge = 30 * 24 * time.Hour

type entry struct {
	ETag string
	Runs []runs.Run
	Seen time.Time
}

type Set struct {
	st  *store.Store
	now func() time.Time

	mu      sync.Mutex
	entries map[string]entry
	dirty   bool
	seeded  map[string]bool
}

func Load(st *store.Store, now func() time.Time) (*Set, error) {
	s := &Set{st: st, now: now, entries: map[string]entry{}}
	if _, err := st.Load(fileName, &s.entries); err != nil {
		return nil, err
	}
	return s, nil
}

// Seed returns the saved runs of repo, or nil if there are none.
func (s *Set) Seed(repo string) *watch.Seed {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[repo]
	if !ok {
		return nil
	}
	return &watch.Seed{ETag: e.ETag, Runs: e.Runs}
}

// Record keeps the runs of a successful event that carries them.
func (s *Set) Record(ev watch.Event) {
	if ev.Err != nil || ev.ETag == "" || ev.Runs == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[ev.Repo] = entry{ETag: ev.ETag, Runs: ev.Runs, Seen: s.now()}
	s.dirty = true
}

// Flush writes the set if anything was recorded since the last write,
// dropping repos not seen for maxAge.
func (s *Set) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	for repo, e := range s.entries {
		if s.now().Sub(e.Seen) > maxAge {
			delete(s.entries, repo)
		}
	}
	if err := s.st.Save(fileName, s.entries); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// SeedOnce is Seed for a repo's first watch of the session, and nil after.
// A later watch of the same repo, after picking repos again, would otherwise
// compare against runs this session saved and mark them as changed since
// the last session.
func (s *Set) SeedOnce(repo string) *watch.Seed {
	s.mu.Lock()
	seen := s.seeded[repo]
	if s.seeded == nil {
		s.seeded = map[string]bool{}
	}
	s.seeded[repo] = true
	s.mu.Unlock()
	if seen {
		return nil
	}
	return s.Seed(repo)
}
