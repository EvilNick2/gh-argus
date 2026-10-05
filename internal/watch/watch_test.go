package watch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/runs"
)

func TestNextInterval(t *testing.T) {
	iv := Intervals{Active: 3 * time.Second, IdleMin: 15 * time.Second, IdleMax: 60 * time.Second}
	cases := []struct {
		prev   time.Duration
		active bool
		want   time.Duration
	}{
		{0, true, 3 * time.Second},
		{60 * time.Second, true, 3 * time.Second},
		{0, false, 15 * time.Second},
		{3 * time.Second, false, 15 * time.Second},
		{15 * time.Second, false, 30 * time.Second},
		{30 * time.Second, false, 60 * time.Second},
		{60 * time.Second, false, 60 * time.Second},
	}
	for _, c := range cases {
		if got := iv.next(c.prev, c.active); got != c.want {
			t.Errorf("next(%v, active=%v) = %v, want %v", c.prev, c.active, got, c.want)
		}
	}
}

// runsServer serves a single run whose status can be changed between polls,
// with an ETag derived from that status so unchanged polls get a 304.
type runsServer struct {
	mu        sync.Mutex
	status    string
	jobStatus string
	failWith  int
	hits      int
	jobsHits  int
}

func (s *runsServer) set(status, jobStatus string) {
	s.mu.Lock()
	s.status, s.jobStatus = status, jobStatus
	s.mu.Unlock()
}

func (s *runsServer) jobsRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobsHits
}

func (s *runsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits++
	var etag, body string
	switch r.URL.Path {
	case "/repos/o/r/actions/runs":
		etag = `"run-` + s.status + `"`
		body = fmt.Sprintf(`{"workflow_runs":[{"id":7,"name":"ci","status":%q}]}`, s.status)
	case "/repos/o/r/actions/runs/7/jobs":
		s.jobsHits++
		etag = `"job-` + s.jobStatus + `"`
		body = fmt.Sprintf(`{"jobs":[{"id":70,"run_id":7,"name":"build","status":%q}]}`, s.jobStatus)
	case "/repos/o/r/actions/runs/7/attempts/1/jobs":
		etag = `"attempt-1"`
		body = `{"jobs":[{"id":60,"run_id":7,"run_attempt":1,"name":"build","status":"completed","conclusion":"failure"}]}`
	default:
		http.NotFound(w, r)
		return
	}
	if s.failWith != 0 {
		http.Error(w, "nope", s.failWith)
		return
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	fmt.Fprint(w, body)
}

func startWatch(t *testing.T, s *runsServer) <-chan Event {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	w := &Watcher{
		Fetcher:   fetch.New(srv.Client(), srv.URL),
		Intervals: Intervals{Active: 5 * time.Millisecond, IdleMin: 5 * time.Millisecond, IdleMax: 5 * time.Millisecond},
	}
	events := make(chan Event, 16)
	go w.Watch(ctx, "o/r", nil, events)
	return events
}

func receive(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func TestWatchEmitsInitialSnapshot(t *testing.T) {
	events := startWatch(t, &runsServer{status: "in_progress", jobStatus: "in_progress"})

	ev := receive(t, events)
	if !ev.Initial || ev.Repo != "o/r" || len(ev.Runs) != 1 || ev.Runs[0].Status != "in_progress" {
		t.Fatalf("got %+v, want initial snapshot with one in_progress run", ev)
	}
}

func TestWatchEmitsChangeAndStaysQuietOn304(t *testing.T) {
	s := &runsServer{status: "in_progress", jobStatus: "in_progress"}
	events := startWatch(t, s)
	receive(t, events)

	// Several unchanged polls must produce no events.
	time.Sleep(30 * time.Millisecond)
	select {
	case ev := <-events:
		t.Fatalf("unexpected event while unchanged: %+v", ev)
	default:
	}

	s.set("completed", "completed")
	ev := receive(t, events)
	if ev.Initial || len(ev.Changes) != 1 || ev.Changes[0].Prev.Status != "in_progress" ||
		ev.Changes[0].Run.Status != "completed" {
		t.Fatalf("got %+v, want one in_progress -> completed change", ev)
	}
}

func TestWatchReportsErrorsAndKeepsPolling(t *testing.T) {
	s := &runsServer{status: "queued", failWith: http.StatusBadGateway}
	events := startWatch(t, s)

	ev := receive(t, events)
	if ev.Err == nil {
		t.Fatalf("got %+v, want an error event", ev)
	}

	s.mu.Lock()
	s.failWith = 0
	s.mu.Unlock()
	for {
		ev = receive(t, events)
		if ev.Err == nil {
			break
		}
	}
	if !ev.Initial || len(ev.Runs) != 1 {
		t.Fatalf("got %+v, want initial snapshot after recovery", ev)
	}
}

func TestWatchReportsJobsOfActiveRun(t *testing.T) {
	events := startWatch(t, &runsServer{status: "in_progress", jobStatus: "in_progress"})

	ev := receive(t, events)
	if len(ev.Jobs) != 1 || ev.Jobs[0].Prev != nil || ev.Jobs[0].Job.Name != "build" || ev.Jobs[0].Job.RunID != 7 {
		t.Fatalf("got jobs %+v, want new job build of run 7", ev.Jobs)
	}
}

func TestWatchPollsJobsOnceMoreWhenRunCompletes(t *testing.T) {
	s := &runsServer{status: "in_progress", jobStatus: "in_progress"}
	events := startWatch(t, s)
	receive(t, events)

	s.set("completed", "completed")
	ev := receive(t, events)
	if len(ev.Jobs) != 1 || ev.Jobs[0].Prev == nil || ev.Jobs[0].Job.Status != "completed" {
		t.Fatalf("got jobs %+v, want build in_progress -> completed", ev.Jobs)
	}

	hits := s.jobsRequests()
	time.Sleep(30 * time.Millisecond)
	if got := s.jobsRequests(); got != hits {
		t.Errorf("jobs polled %d more times after run completed", got-hits)
	}
}

func TestWatchDoesNotPollJobsOfCompletedRuns(t *testing.T) {
	s := &runsServer{status: "completed", jobStatus: "completed"}
	events := startWatch(t, s)
	ev := receive(t, events)

	time.Sleep(30 * time.Millisecond)
	if len(ev.Jobs) != 0 || s.jobsRequests() != 0 {
		t.Errorf("jobs fetched for a completed run: %d requests, event jobs %+v", s.jobsRequests(), ev.Jobs)
	}
}

func TestNewWatchOnSharedFetcherEmitsInitialFrom304(t *testing.T) {
	s := &runsServer{status: "completed", jobStatus: "completed"}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	w := &Watcher{
		Fetcher:   fetch.New(srv.Client(), srv.URL),
		Intervals: Intervals{Active: 5 * time.Millisecond, IdleMin: 5 * time.Millisecond, IdleMax: 5 * time.Millisecond},
	}

	for i := range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		events := make(chan Event, 16)
		go w.Watch(ctx, "o/r", nil, events)
		ev := receive(t, events)
		cancel()
		if !ev.Initial || len(ev.Runs) != 1 {
			t.Fatalf("watch %d: got %+v, want an initial snapshot", i+1, ev)
		}
	}
}

func startRunWatch(t *testing.T, s *runsServer) <-chan RunEvent {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := &Watcher{
		Fetcher:   fetch.New(srv.Client(), srv.URL),
		Intervals: Intervals{Active: 5 * time.Millisecond, IdleMin: 5 * time.Millisecond, IdleMax: 5 * time.Millisecond},
	}
	events := make(chan RunEvent, 16)
	go w.WatchRun(ctx, "o/r", 7, 0, events)
	return events
}

func receiveRun(t *testing.T, events <-chan RunEvent) RunEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run event")
		return RunEvent{}
	}
}

func TestWatchRunEmitsJobsThenChangesOnly(t *testing.T) {
	s := &runsServer{status: "in_progress", jobStatus: "in_progress"}
	events := startRunWatch(t, s)

	ev := receiveRun(t, events)
	if ev.Err != nil || len(ev.Jobs) != 1 || ev.Jobs[0].Status != "in_progress" {
		t.Fatalf("first event %+v", ev)
	}
	time.Sleep(30 * time.Millisecond)
	select {
	case ev := <-events:
		t.Fatalf("event while unchanged: %+v", ev)
	default:
	}

	s.set("completed", "completed")
	if ev := receiveRun(t, events); len(ev.Jobs) != 1 || ev.Jobs[0].Status != "completed" {
		t.Fatalf("change event %+v", ev)
	}
}

func TestWatchRunReportsErrors(t *testing.T) {
	events := startRunWatch(t, &runsServer{status: "queued", jobStatus: "queued", failWith: http.StatusNotFound})

	if ev := receiveRun(t, events); ev.Err == nil {
		t.Fatalf("got %+v, want error", ev)
	}
}

func startSeededWatch(t *testing.T, s *runsServer, seed *Seed) <-chan Event {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := &Watcher{
		Fetcher:   fetch.New(srv.Client(), srv.URL),
		Intervals: Intervals{Active: 5 * time.Millisecond, IdleMin: 5 * time.Millisecond, IdleMax: 5 * time.Millisecond},
	}
	events := make(chan Event, 16)
	go w.Watch(ctx, "o/r", seed, events)
	return events
}

func TestSeedConfirmedBy304IsInitialSnapshot(t *testing.T) {
	s := &runsServer{status: "completed", jobStatus: "completed"}
	seed := &Seed{ETag: `"run-completed"`, Runs: []runs.Run{{ID: 7, Name: "ci", Status: "completed"}}}
	events := startSeededWatch(t, s, seed)

	ev := receive(t, events)
	if ev.Err != nil || !ev.Initial || len(ev.Runs) != 1 || ev.Runs[0].Name != "ci" || len(ev.Changes) != 0 {
		t.Fatalf("got %+v, want the seeded runs as the initial snapshot with no changes", ev)
	}
	if ev.ETag != `"run-completed"` {
		t.Errorf("ETag %q", ev.ETag)
	}
}

func TestStaleSeedReportsChangesSinceLastSession(t *testing.T) {
	s := &runsServer{status: "completed", jobStatus: "completed"}
	seed := &Seed{ETag: `"run-in_progress"`, Runs: []runs.Run{{ID: 7, Name: "ci", Status: "in_progress"}}}
	events := startSeededWatch(t, s, seed)

	ev := receive(t, events)
	if !ev.Initial || len(ev.Runs) != 1 || ev.Runs[0].Status != "completed" {
		t.Fatalf("got %+v, want the fresh runs as the initial snapshot", ev)
	}
	if len(ev.Changes) != 1 || ev.Changes[0].Prev == nil || ev.Changes[0].Prev.Status != "in_progress" {
		t.Errorf("changes %+v, want in_progress -> completed since the last session", ev.Changes)
	}
	if ev.ETag != `"run-completed"` {
		t.Errorf("ETag %q, want the new one", ev.ETag)
	}
}

func TestChangeEventsCarryETag(t *testing.T) {
	s := &runsServer{status: "in_progress", jobStatus: "in_progress"}
	events := startWatch(t, s)
	receive(t, events)

	s.set("completed", "completed")
	if ev := receive(t, events); ev.ETag != `"run-completed"` {
		t.Errorf("change event ETag %q", ev.ETag)
	}
}

func TestWatchRunOfAnEarlierAttempt(t *testing.T) {
	srv := httptest.NewServer(&runsServer{status: "completed", jobStatus: "completed"})
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := &Watcher{
		Fetcher:   fetch.New(srv.Client(), srv.URL),
		Intervals: Intervals{Active: 5 * time.Millisecond, IdleMin: 5 * time.Millisecond, IdleMax: 5 * time.Millisecond},
	}
	events := make(chan RunEvent, 4)
	go w.WatchRun(ctx, "o/r", 7, 1, events)

	ev := receiveRun(t, events)
	if ev.Err != nil || len(ev.Jobs) != 1 || ev.Jobs[0].ID != 60 || ev.Jobs[0].RunAttempt != 1 {
		t.Errorf("got %+v, want attempt 1's job", ev)
	}
}
