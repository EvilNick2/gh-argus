package runview

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func at(sec int) time.Time { return now.Add(time.Duration(sec-100) * time.Second) }

var run = runs.Run{ID: 16, RunNumber: 16, Name: "Manifest check", HeadBranch: "main", Status: "in_progress"}

var jobs = []runs.Job{
	{ID: 1, Name: "check", Status: "in_progress", StartedAt: at(0), Steps: []runs.Step{
		{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: at(0), CompletedAt: at(2)},
		{Number: 2, Name: "Run actions/checkout@v4", Status: "completed", Conclusion: "success", StartedAt: at(2), CompletedAt: at(3)},
		{Number: 3, Name: "Validate manifest", Status: "in_progress", StartedAt: at(96)},
	}},
	{ID: 2, Name: "lint", Status: "queued", Steps: []runs.Step{
		{Number: 1, Name: "Lint step", Status: "queued"},
	}},
}

func newModel(t *testing.T) Model {
	t.Helper()
	m := New("EvilNick2/dotfiles", run, func() time.Time { return now }).SetSize(100, 20)
	m, _ = m.Update(watch.RunEvent{Jobs: jobs})
	return m
}

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func view(m Model) string { return ansi.Strip(m.View()) }

func line(t *testing.T, v, substr string) string {
	t.Helper()
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substr, v)
	return ""
}

func TestHeaderShowsRun(t *testing.T) {
	v := view(newModel(t))

	l := strings.Join(strings.Split(v, "\n")[:2], " ")
	for _, want := range []string{"EvilNick2/dotfiles", "#16 Manifest check", "* running", "main"} {
		if !strings.Contains(l, want) {
			t.Errorf("header %q missing %q", l, want)
		}
	}
}

func TestCursorJobShowsStepsWithDurations(t *testing.T) {
	v := view(newModel(t))

	if l := line(t, v, "Set up job"); !strings.Contains(l, "+") || !strings.Contains(l, "2s") {
		t.Errorf("step line %q", l)
	}
	if l := line(t, v, "Validate manifest"); !strings.Contains(l, "*") || !strings.Contains(l, "4s") {
		t.Errorf("running step line %q, want run badge and 4s so far", l)
	}
	if strings.Contains(v, "Lint step") {
		t.Errorf("steps of a job not under the cursor shown:\n%s", v)
	}
	if l := line(t, v, "lint"); !strings.Contains(l, "o lint") {
		t.Errorf("queued job line %q", l)
	}
}

func TestJMovesToNextJobAndExpandsIt(t *testing.T) {
	m := newModel(t)

	m, _ = m.Update(key("j"))
	v := view(m)
	if !strings.Contains(v, "Lint step") || strings.Contains(v, "Set up job") {
		t.Errorf("after j:\n%s", v)
	}
}

func TestEnterOpensLogOfCursorJob(t *testing.T) {
	m := newModel(t)

	m, _ = m.Update(key("j"))
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("no command")
	}
	msg, ok := cmd().(OpenLogMsg)
	if !ok || msg.Repo != "EvilNick2/dotfiles" || msg.Job.ID != 2 {
		t.Errorf("got %#v, want OpenLogMsg for job 2", msg)
	}
}

func TestEscGoesBack(t *testing.T) {
	_, cmd := newModel(t).Update(key("esc"))
	if cmd == nil {
		t.Fatal("no command")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Error("esc did not send BackMsg")
	}
}

func TestBeforeJobsArriveShowsLoading(t *testing.T) {
	m := New("o/r", run, func() time.Time { return now }).SetSize(100, 20)

	if v := view(m); !strings.Contains(v, "loading jobs") {
		t.Errorf("view:\n%s", v)
	}
	if _, cmd := m.Update(key("enter")); cmd != nil {
		t.Error("enter with no jobs returned a command")
	}
}

func TestErrorShownAndJobsKept(t *testing.T) {
	m := newModel(t)

	m, _ = m.Update(watch.RunEvent{Err: errors.New("GET jobs: 502")})
	v := view(m)
	if !strings.Contains(v, "502") || !strings.Contains(v, "check") {
		t.Errorf("view:\n%s", v)
	}
}

func TestSetRunUpdatesHeader(t *testing.T) {
	m := newModel(t)
	done := run
	done.Status, done.Conclusion = "completed", "failure"

	m = m.SetRun(done)
	if l := strings.Split(view(m), "\n")[1]; !strings.Contains(l, "x failed") {
		t.Errorf("header %q, want fail", l)
	}
}

func TestCursorKeptOnJobByIDWhenJobsReorder(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(key("j")) // on lint

	reordered := []runs.Job{jobs[1], jobs[0]}
	m, _ = m.Update(watch.RunEvent{Jobs: reordered})
	_, cmd := m.Update(key("enter"))
	if msg := cmd().(OpenLogMsg); msg.Job.ID != 2 {
		t.Errorf("cursor moved to job %d, want 2", msg.Job.ID)
	}
}

func TestScrollsToKeepCursorJobAndStepsVisible(t *testing.T) {
	var many []runs.Job
	for i := range 20 {
		many = append(many, runs.Job{ID: int64(i), Name: "job" + string(rune('a'+i)), Status: "completed", Conclusion: "success",
			Steps: []runs.Step{{Number: 1, Name: "only step", Status: "completed", Conclusion: "success"}}})
	}
	m := New("o/r", run, func() time.Time { return now }).SetSize(100, 8)
	m, _ = m.Update(watch.RunEvent{Jobs: many})
	for range 15 {
		m, _ = m.Update(key("j"))
	}
	v := view(m)
	if !strings.Contains(v, "jobp") || !strings.Contains(v, "only step") || strings.Contains(v, "joba") {
		t.Errorf("cursor job p and its step not scrolled into view:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n > 8 {
		t.Errorf("view is %d lines, taller than height 8", n)
	}
}

func TestRunScreenIsOnePaneOfFullSize(t *testing.T) {
	lines := strings.Split(view(newModel(t)), "\n")

	if len(lines) != 20 {
		t.Fatalf("%d lines, want 20", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 100 {
			t.Errorf("line %d is %d wide, want 100: %q", i, w, l)
		}
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[19], "╰") {
		t.Errorf("not framed:\n%s", strings.Join(lines, "\n"))
	}
}

// Inside the pane the state line is y 1 and a blank y 2, so jobs start at
// y 3: check at 3, its three steps at 4 to 6, lint at 7.

func TestClickJobSelectsIt(t *testing.T) {
	m := newModel(t)

	m, cmd := m.Mouse(mouse.Event{X: 10, Y: 7, Kind: mouse.Click})
	if cmd != nil {
		t.Error("single click opened something")
	}
	if v := view(m); !strings.Contains(v, "Lint step") {
		t.Errorf("lint not selected:\n%s", v)
	}
}

func TestDoubleClickJobOrStepOpensLog(t *testing.T) {
	for _, y := range []int{3, 5} {
		_, cmd := newModel(t).Mouse(mouse.Event{X: 10, Y: y, Kind: mouse.DoubleClick})
		if cmd == nil {
			t.Fatalf("y %d: no command", y)
		}
		if msg, ok := cmd().(OpenLogMsg); !ok || msg.Job.ID != 1 {
			t.Errorf("y %d: got %#v, want OpenLogMsg for job 1", y, cmd())
		}
	}
}

func TestClickOutsideJobsDoesNothing(t *testing.T) {
	for _, y := range []int{0, 1, 2, 15} {
		m, cmd := newModel(t).Mouse(mouse.Event{X: 10, Y: y, Kind: mouse.DoubleClick})
		if cmd != nil {
			t.Errorf("y %d: returned a command", y)
		}
		if v := view(m); !strings.Contains(v, "Set up job") {
			t.Errorf("y %d: selection moved", y)
		}
	}
}

func TestWheelMovesBetweenJobs(t *testing.T) {
	m := newModel(t)

	m, _ = m.Mouse(mouse.Event{X: 10, Y: 10, Kind: mouse.WheelDown})
	if v := view(m); !strings.Contains(v, "Lint step") {
		t.Errorf("wheel down did not select lint:\n%s", v)
	}
	m, _ = m.Mouse(mouse.Event{X: 10, Y: 10, Kind: mouse.WheelUp})
	if v := view(m); !strings.Contains(v, "Set up job") {
		t.Errorf("wheel up did not go back:\n%s", v)
	}
}

func TestSetAttemptLabelsHeaderAndReloadsJobs(t *testing.T) {
	m := newModel(t).SetAttempt(1, 2)

	v := view(m)
	if !strings.Contains(strings.Split(v, "\n")[1], "attempt 1 of 2") {
		t.Errorf("state line %q, want attempt 1 of 2", strings.Split(v, "\n")[1])
	}
	if !strings.Contains(v, "loading jobs") || strings.Contains(v, "Set up job") {
		t.Errorf("jobs of the previous attempt kept:\n%s", v)
	}
}

func TestLatestAttemptOfSeveralIsLabelled(t *testing.T) {
	m := newModel(t).SetAttempt(0, 3)

	if l := strings.Split(view(m), "\n")[1]; !strings.Contains(l, "attempt 3 of 3") {
		t.Errorf("state line %q", l)
	}
}

func TestEmptyEarlierAttemptSaysSo(t *testing.T) {
	m := newModel(t).SetAttempt(1, 2)
	m, _ = m.Update(watch.RunEvent{Jobs: []runs.Job{}})

	if v := view(m); !strings.Contains(v, "no jobs in this attempt") {
		t.Errorf("view:\n%s", v)
	}
}
