// Package watch polls the workflow runs of a repository and reports changes.
package watch

import (
	"context"
	"time"

	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/runs"
)

// Intervals sets the poll cadence. A repo with an unfinished run is polled
// at Active. Idle repos start at IdleMin and double up to IdleMax.
type Intervals struct {
	Active, IdleMin, IdleMax time.Duration
}

var DefaultIntervals = Intervals{Active: 3 * time.Second, IdleMin: 15 * time.Second, IdleMax: 60 * time.Second}

func (iv Intervals) next(prev time.Duration, active bool) time.Duration {
	switch {
	case active:
		return iv.Active
	case prev < iv.IdleMin:
		return iv.IdleMin
	default:
		return min(prev*2, iv.IdleMax)
	}
}

// Event is emitted on the first successful poll (Initial, with Runs), when
// runs change (Changes, with the new Runs), or when a poll fails (Err).
// Unchanged polls emit nothing.
type Event struct {
	Repo    string
	Initial bool
	Runs    []runs.Run
	Changes []runs.Change
	Err     error
}

type Watcher struct {
	Fetcher   *fetch.Fetcher
	Intervals Intervals
}

// Watch polls repo ("owner/name") until ctx is done.
func (w *Watcher) Watch(ctx context.Context, repo string, out chan<- Event) {
	path := "/repos/" + repo + "/actions/runs?per_page=30"
	var (
		prev     []runs.Run
		have     bool
		interval time.Duration
	)
	for {
		ev, active := Event{Repo: repo}, false
		res, err := w.Fetcher.Get(ctx, path)
		if err == nil && !res.NotModified {
			var cur []runs.Run
			if cur, err = runs.Decode(res.Body); err == nil {
				if !have {
					ev.Initial, ev.Runs = true, cur
				} else if ev.Changes = runs.Diff(prev, cur); len(ev.Changes) > 0 {
					ev.Runs = cur
				}
				prev, have = cur, true
			}
		}
		if ctx.Err() != nil {
			return
		}
		ev.Err = err
		if err == nil {
			active = runs.Active(prev)
		}
		if ev.Initial || len(ev.Changes) > 0 || ev.Err != nil {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}

		interval = w.Intervals.next(interval, active)
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
	}
}
