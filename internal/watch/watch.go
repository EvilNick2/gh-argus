// Package watch polls the workflow runs of a repository and reports changes.
package watch

import (
	"context"
	"fmt"
	"slices"
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

// Event is emitted when a poll finds something to report. The first
// successful poll is Initial. Later ones report Changes to runs and Jobs
// changes for runs in flight. Runs is always the current snapshot. Err is set
// when a request failed, alongside anything gathered before it.
type Event struct {
	Repo    string
	Initial bool
	Runs    []runs.Run
	Changes []runs.Change
	Jobs    []runs.JobChange
	Err     error
}

func (e Event) empty() bool {
	return !e.Initial && len(e.Changes) == 0 && len(e.Jobs) == 0 && e.Err == nil
}

type Watcher struct {
	Fetcher   *fetch.Fetcher
	Intervals Intervals
}

type state struct {
	runs []runs.Run
	have bool
	jobs map[int64][]runs.Job
}

// Watch polls repo ("owner/name") until ctx is done.
func (w *Watcher) Watch(ctx context.Context, repo string, out chan<- Event) {
	st := &state{jobs: map[int64][]runs.Job{}}
	var interval time.Duration
	for {
		ev := Event{Repo: repo}
		ev.Err = w.poll(ctx, repo, st, &ev)
		if ctx.Err() != nil {
			return
		}
		ev.Runs = st.runs
		if !ev.empty() {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}

		interval = w.Intervals.next(interval, ev.Err == nil && runs.Active(st.runs))
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) poll(ctx context.Context, repo string, st *state, ev *Event) error {
	res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/actions/runs?per_page=30")
	if err != nil {
		return err
	}
	prev := st.runs
	// The fetcher may be shared with an earlier watch, so even the first
	// response can be a 304. Its cached body still makes the first snapshot.
	if !res.NotModified || !st.have {
		cur, err := runs.Decode(res.Body)
		if err != nil {
			return err
		}
		if st.have {
			ev.Changes = runs.Diff(prev, cur)
		} else {
			ev.Initial = true
		}
		st.runs, st.have = cur, true
	}

	// Runs active in the previous snapshot are included so the final state
	// of their jobs is seen after they complete.
	var ids []int64
	for _, r := range slices.Concat(prev, st.runs) {
		if r.Status != "completed" && !slices.Contains(ids, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	for id := range st.jobs {
		if !slices.Contains(ids, id) {
			delete(st.jobs, id)
		}
	}
	for _, id := range ids {
		res, err := w.Fetcher.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?per_page=100", repo, id))
		if err != nil {
			return err
		}
		// A 304 still carries the cached body, which matters when a run
		// becomes active again after its jobs were dropped from st.jobs.
		jobs, err := runs.DecodeJobs(res.Body)
		if err != nil {
			return err
		}
		ev.Jobs = append(ev.Jobs, runs.DiffJobs(st.jobs[id], jobs)...)
		st.jobs[id] = jobs
	}
	return nil
}

// RunEvent carries the jobs of one run, or the error from polling them.
type RunEvent struct {
	Jobs []runs.Job
	Err  error
}

// WatchRun polls the jobs of run id until ctx is done, for a screen showing
// that run. It emits the jobs on the first successful poll and whenever they
// change. While any job is unfinished, or none exist yet, it polls at Active.
func (w *Watcher) WatchRun(ctx context.Context, repo string, id int64, out chan<- RunEvent) {
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?per_page=100", repo, id)
	var (
		last     []runs.Job
		have     bool
		interval time.Duration
	)
	for {
		var ev RunEvent
		send := false
		res, err := w.Fetcher.Get(ctx, path)
		if ctx.Err() != nil {
			return
		}
		switch {
		case err != nil:
			ev.Err, send = err, true
		case !res.NotModified || !have:
			jobs, err := runs.DecodeJobs(res.Body)
			if err != nil {
				ev.Err, send = err, true
				break
			}
			last, have = jobs, true
			ev.Jobs, send = jobs, true
		}
		if send {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}

		active := len(last) == 0
		for _, j := range last {
			if j.Status != "completed" {
				active = true
			}
		}
		interval = w.Intervals.next(interval, ev.Err == nil && active)
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		}
	}
}
