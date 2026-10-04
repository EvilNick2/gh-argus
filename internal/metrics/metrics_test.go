package metrics

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/EvilNick2/gh-argus/internal/runs"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// run makes a completed run of workflow wf that started n hours after t0,
// queued for q seconds and ran for d seconds.
func run(wf int64, name string, n int, conclusion string, d, q int) runs.Run {
	created := t0.Add(time.Duration(n) * time.Hour)
	started := created.Add(time.Duration(q) * time.Second)
	return runs.Run{
		ID: int64(n), WorkflowID: wf, Name: name, Status: "completed", Conclusion: conclusion,
		RunAttempt: 1, CreatedAt: created, RunStartedAt: started, UpdatedAt: started.Add(time.Duration(d) * time.Second),
	}
}

// newestFirst orders runs the way the API returns them.
func newestFirst(rs ...runs.Run) []runs.Run {
	slices.SortFunc(rs, func(a, b runs.Run) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return rs
}

func TestSuccessRateIgnoresCancelledAndSkipped(t *testing.T) {
	total, _ := Compute(newestFirst(
		run(1, "ci", 1, "success", 10, 0),
		run(1, "ci", 2, "success", 10, 0),
		run(1, "ci", 3, "failure", 10, 0),
		run(1, "ci", 4, "timed_out", 10, 0),
		run(1, "ci", 5, "cancelled", 10, 0),
		run(1, "ci", 6, "skipped", 10, 0),
	))

	if total.Succeeded != 2 || total.Failed != 2 || total.Cancelled != 1 || total.Runs != 6 {
		t.Errorf("counts %+v", total)
	}
	if rate, ok := total.SuccessRate(); !ok || rate != 0.5 {
		t.Errorf("SuccessRate() = %v, %v, want 0.5", rate, ok)
	}
}

func TestSuccessRateUndefinedWithoutPassOrFail(t *testing.T) {
	total, _ := Compute([]runs.Run{run(1, "ci", 1, "cancelled", 10, 0)})

	if _, ok := total.SuccessRate(); ok {
		t.Error("rate defined with only cancelled runs")
	}
}

func TestUnfinishedRunsAreLeftOut(t *testing.T) {
	r := run(1, "ci", 1, "", 0, 0)
	r.Status = "in_progress"

	total, _ := Compute([]runs.Run{r, run(1, "ci", 0, "success", 30, 0)})
	if total.Runs != 1 || total.Median != 30*time.Second {
		t.Errorf("got %+v, want only the completed run", total)
	}
}

func TestMedianDurationAndQueue(t *testing.T) {
	total, _ := Compute(newestFirst(
		run(1, "ci", 1, "success", 10, 0),
		run(1, "ci", 2, "success", 30, 4),
		run(1, "ci", 3, "success", 20, 2),
	))

	if total.Median != 20*time.Second {
		t.Errorf("Median %v, want 20s", total.Median)
	}
	if total.QueueMedian != 2*time.Second {
		t.Errorf("QueueMedian %v, want 2s", total.QueueMedian)
	}
}

func TestTrendComparesNewerHalfToOlderHalf(t *testing.T) {
	// Older runs take 10s, newer runs 15s: 50% slower.
	total, _ := Compute(newestFirst(
		run(1, "ci", 1, "success", 10, 0),
		run(1, "ci", 2, "success", 10, 0),
		run(1, "ci", 3, "success", 15, 0),
		run(1, "ci", 4, "success", 15, 0),
	))

	if math.Abs(total.Trend-0.5) > 1e-9 {
		t.Errorf("Trend %v, want 0.5", total.Trend)
	}
}

func TestTrendNeedsFourRuns(t *testing.T) {
	total, _ := Compute(newestFirst(
		run(1, "ci", 1, "success", 10, 0),
		run(1, "ci", 2, "success", 20, 0),
		run(1, "ci", 3, "success", 30, 0),
	))

	if !math.IsNaN(total.Trend) {
		t.Errorf("Trend %v from 3 runs, want NaN", total.Trend)
	}
}

func TestRerunsCounted(t *testing.T) {
	r := run(1, "ci", 1, "success", 10, 0)
	r.RunAttempt = 3

	total, _ := Compute([]runs.Run{r, run(1, "ci", 2, "success", 10, 0)})
	if total.Reruns != 1 {
		t.Errorf("Reruns %d, want 1", total.Reruns)
	}
}

func TestPerWorkflowGroupedAndOrderedByLatestRun(t *testing.T) {
	_, per := Compute(newestFirst(
		run(1, "ci", 1, "success", 10, 0),
		run(2, "deploy", 2, "failure", 50, 0),
		run(1, "ci", 3, "failure", 10, 0),
		run(2, "deploy", 4, "success", 50, 0),
		run(1, "ci", 5, "success", 10, 0),
	))

	if len(per) != 2 || per[0].Name != "ci" || per[1].Name != "deploy" {
		t.Fatalf("got %+v, want ci (latest run at 5) then deploy", per)
	}
	if per[0].Runs != 3 || per[1].Runs != 2 || per[1].Median != 50*time.Second {
		t.Errorf("ci %+v deploy %+v", per[0], per[1])
	}
	if !slices.Equal(per[0].History, []string{"pass", "fail", "pass"}) {
		t.Errorf("ci history %v, want oldest to newest", per[0].History)
	}
	if !per[0].Last.Equal(t0.Add(5 * time.Hour)) {
		t.Errorf("ci Last %v", per[0].Last)
	}
}

func TestHistoryKeepsLast20(t *testing.T) {
	var rs []runs.Run
	for i := range 25 {
		c := "success"
		if i >= 20 {
			c = "failure"
		}
		rs = append(rs, run(1, "ci", i, c, 10, 0))
	}

	total, _ := Compute(newestFirst(rs...))
	if len(total.History) != 20 || total.History[19] != "fail" || total.History[14] != "pass" {
		t.Errorf("history %v", total.History)
	}
}
