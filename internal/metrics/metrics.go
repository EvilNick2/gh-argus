// Package metrics derives CI health from a runs list: success rate, duration
// and its trend, reruns and recent results. Durations come from updated_at
// minus run_started_at, which matched run_duration_ms from the timing
// endpoint on every run sampled, so no per-run request is needed.
package metrics

import (
	"math"
	"slices"
	"time"

	"github.com/EvilNick2/gh-argus/internal/badge"
	"github.com/EvilNick2/gh-argus/internal/runs"
)

// historyLen is how many recent results Stat.History keeps.
const historyLen = 20

// Stat summarises the completed runs of a repo or of one workflow.
type Stat struct {
	Name       string // workflow name, empty for a repo total
	WorkflowID int64
	Runs       int // completed runs
	Succeeded  int
	Failed     int // failure, timed_out and startup_failure
	Cancelled  int
	Reruns     int // runs on attempt 2 or later
	Median     time.Duration
	// QueueMedian is run_started_at minus created_at. It measured 0 on every
	// run sampled, so it is a detail rather than a headline.
	QueueMedian time.Duration
	// Trend is the median duration of the newer half of runs relative to the
	// older half, so 0.5 is 50% slower. NaN with fewer than 4 durations.
	Trend   float64
	History []string // badge words, oldest to newest
	Last    time.Time
}

// SuccessRate is passed over passed plus failed. Cancelled and skipped runs
// are neither, so they are left out. It is undefined with no passes or fails.
func (s Stat) SuccessRate() (float64, bool) {
	n := s.Succeeded + s.Failed
	if n == 0 {
		return 0, false
	}
	return float64(s.Succeeded) / float64(n), true
}

// Compute summarises rs, which is ordered newest first as the API returns
// it, for the repo as a whole and per workflow. Workflows are ordered by
// their latest run, newest first.
func Compute(rs []runs.Run) (Stat, []Stat) {
	total := summarise(rs)

	byID := map[int64][]runs.Run{}
	var order []int64
	for _, r := range rs {
		if _, ok := byID[r.WorkflowID]; !ok {
			order = append(order, r.WorkflowID)
		}
		byID[r.WorkflowID] = append(byID[r.WorkflowID], r)
	}
	per := make([]Stat, 0, len(order))
	for _, id := range order {
		s := summarise(byID[id])
		s.WorkflowID, s.Name = id, byID[id][0].Name
		per = append(per, s)
	}
	slices.SortStableFunc(per, func(a, b Stat) int { return b.Last.Compare(a.Last) })
	return total, per
}

func summarise(rs []runs.Run) Stat {
	s := Stat{Trend: math.NaN()}
	var durs, queues []time.Duration
	for _, r := range rs {
		if r.CreatedAt.After(s.Last) {
			s.Last = r.CreatedAt
		}
		if r.Status != "completed" {
			continue
		}
		s.Runs++
		switch r.Conclusion {
		case "success":
			s.Succeeded++
		case "failure", "timed_out", "startup_failure":
			s.Failed++
		case "cancelled":
			s.Cancelled++
		}
		if r.RunAttempt > 1 {
			s.Reruns++
		}
		if !r.RunStartedAt.IsZero() && r.UpdatedAt.After(r.RunStartedAt) {
			durs = append(durs, r.UpdatedAt.Sub(r.RunStartedAt))
			queues = append(queues, max(0, r.RunStartedAt.Sub(r.CreatedAt)))
		}
		if len(s.History) < historyLen {
			s.History = append(s.History, badge.Word(r.Status, r.Conclusion))
		}
	}
	slices.Reverse(s.History)

	s.Median, s.QueueMedian = median(durs), median(queues)
	if n := len(durs); n >= 4 {
		newer, older := median(durs[:n/2]), median(durs[n-n/2:])
		if older > 0 {
			s.Trend = float64(newer)/float64(older) - 1
		}
	}
	return s
}

func median(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sorted := slices.Clone(ds)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
