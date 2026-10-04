package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/repos"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/runsview"
	"github.com/EvilNick2/gh-argus/internal/runview"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

// fakeWatch records each Watch call and hands back a channel the test feeds.
type fakeWatch struct {
	calls [][]string
	ctxs  []context.Context
	chans []chan watch.Event
	saved [][]string

	runCalls []string
	runCtxs  []context.Context
	runChans []chan watch.RunEvent

	logCalls []string
	logBody  []joblog.Line
}

func (f *fakeWatch) deps() Deps {
	return Deps{
		Watch: func(ctx context.Context, rs []string) <-chan watch.Event {
			ch := make(chan watch.Event, 4)
			f.calls = append(f.calls, rs)
			f.ctxs = append(f.ctxs, ctx)
			f.chans = append(f.chans, ch)
			return ch
		},
		SaveSelection: func(rs []string) error {
			f.saved = append(f.saved, rs)
			return nil
		},
		WatchRun: func(ctx context.Context, repo string, id int64) <-chan watch.RunEvent {
			ch := make(chan watch.RunEvent, 4)
			f.runCalls = append(f.runCalls, fmt.Sprintf("%s/%d", repo, id))
			f.runCtxs = append(f.runCtxs, ctx)
			f.runChans = append(f.runChans, ch)
			return ch
		},
		FetchLog: func(ctx context.Context, repo string, id int64) ([]joblog.Line, error) {
			f.logCalls = append(f.logCalls, fmt.Sprintf("%s/%d", repo, id))
			return f.logBody, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
	}
}

var sample = []repos.Repo{
	{FullName: "EvilNick2/dotfiles", Owner: "EvilNick2", Name: "dotfiles"},
	{FullName: "EvilNick2/orpheus", Owner: "EvilNick2", Name: "orpheus"},
}

func newPicker() picker.Model {
	return picker.New(sample, nil, nil)
}

// step applies msg and then runs any returned command, feeding its message
// back in, except for commands that block on the watch channel.
func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func keyMsg(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func sized(m Model) Model {
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	return m
}

// confirm picks the first repo in the picker and feeds the ConfirmMsg back.
func confirm(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, cmd := step(m, keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter in picker returned no command")
	}
	return step(m, cmd())
}

func TestStartsInPickerWithoutRepos(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), nil))

	if v := view(m); !strings.Contains(v, "pick repos to watch") {
		t.Errorf("view:\n%s", v)
	}
	if len(f.calls) != 0 {
		t.Errorf("watch started before picking: %v", f.calls)
	}
}

func TestConfirmSavesSelectionStartsWatchAndShowsRuns(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), nil))

	m, _ = confirm(t, m)
	want := []string{"EvilNick2/dotfiles"}
	if len(f.calls) != 1 || !slices.Equal(f.calls[0], want) {
		t.Errorf("watch calls %v, want one with %v", f.calls, want)
	}
	if len(f.saved) != 1 || !slices.Equal(f.saved[0], want) {
		t.Errorf("saved %v, want %v", f.saved, want)
	}
	if v := view(m); !strings.Contains(v, "Runs") || !strings.Contains(v, "waiting for first poll") {
		t.Errorf("view after confirm:\n%s", v)
	}
}

func TestReposGivenSkipsPickerAndWatches(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))

	if len(f.calls) != 1 || !slices.Equal(f.calls[0], []string{"o/r"}) {
		t.Errorf("watch calls %v", f.calls)
	}
	if len(f.saved) != 0 {
		t.Errorf("-R repos saved as selection: %v", f.saved)
	}
	if v := view(m); strings.Contains(v, "pick repos") {
		t.Errorf("picker shown despite repos given:\n%s", v)
	}
}

func TestWatchEventsReachRunsTab(t *testing.T) {
	f := &fakeWatch{}
	m := New(f.deps(), newPicker(), []string{"o/r"})
	m = sized(m)

	f.chans[0] <- watch.Event{Repo: "o/r", Initial: true, Runs: []runs.Run{
		{ID: 1, RunNumber: 7, Name: "build", Status: "completed", Conclusion: "success"},
	}}
	msg := m.Init()()
	m, next := step(m, msg)
	if v := view(m); !strings.Contains(v, "#7 build") {
		t.Errorf("event not shown:\n%s", v)
	}
	if next == nil {
		t.Error("no command to wait for the next event")
	}
}

func TestNumberKeysSwitchTabs(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))

	m, _ = step(m, keyMsg("3"))
	if v := view(m); !strings.Contains(v, "Metrics") || strings.Contains(v, "waiting for first poll") {
		t.Errorf("tab 3 view:\n%s", v)
	}
	m, _ = step(m, keyMsg("1"))
	if v := view(m); !strings.Contains(v, "waiting for first poll") {
		t.Errorf("back on tab 1 view:\n%s", v)
	}
}

// repickOrpheus swaps the picker selection from dotfiles to orpheus.
func repickOrpheus(m Model) Model {
	for _, k := range []string{"space", "j", "space"} {
		m, _ = step(m, keyMsg(k))
	}
	return m
}

func TestPReopensPickerAndStopsOldWatch(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), nil))
	m, _ = confirm(t, m)

	m, _ = step(m, keyMsg("p"))
	if v := view(m); !strings.Contains(v, "pick repos to watch") {
		t.Errorf("p did not reopen picker:\n%s", v)
	}
	if f.ctxs[0].Err() == nil {
		t.Error("old watch context not cancelled")
	}
	m = repickOrpheus(m)
	m, _ = confirm(t, m)
	if len(f.calls) != 2 || !slices.Equal(f.calls[1], []string{"EvilNick2/orpheus"}) {
		t.Errorf("second watch calls %v", f.calls)
	}
}

func TestEventsFromOldWatchAreIgnored(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), nil))
	m, waitOld := confirm(t, m)
	m, _ = step(m, keyMsg("p"))
	m = repickOrpheus(m)
	m, _ = confirm(t, m)

	f.chans[0] <- watch.Event{Repo: "EvilNick2/orpheus", Initial: true, Runs: []runs.Run{{ID: 1, RunNumber: 99, Name: "stale"}}}
	m, next := step(m, waitOld())
	if v := view(m); strings.Contains(v, "#99 stale") {
		t.Errorf("stale event shown:\n%s", v)
	}
	if next != nil {
		t.Error("kept waiting on the old watch channel")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		f := &fakeWatch{}
		m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
		_, cmd := step(m, keyMsg(k))
		if cmd == nil {
			t.Errorf("%s: no command", k)
			continue
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: command did not quit", k)
		}
		if f.ctxs[0].Err() == nil {
			t.Errorf("%s: watch context not cancelled on quit", k)
		}
	}
}

func TestInitRefreshesPickerEvenWhenReposGiven(t *testing.T) {
	f := &fakeWatch{}
	refreshed := false
	p := newPicker().WithInit(func() tea.Msg { refreshed = true; return nil })
	m := New(f.deps(), p, []string{"o/r"})
	close(f.chans[0])

	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init did not batch the watch wait with the picker refresh")
	}
	for _, cmd := range batch {
		cmd()
	}
	if !refreshed {
		t.Error("picker refresh did not run")
	}
}

var openRun = runs.Run{ID: 7, RunNumber: 7, Name: "build", HeadBranch: "main", Status: "in_progress"}

// onRunScreen returns a model watching o/r with run 7 open.
func onRunScreen(t *testing.T, f *fakeWatch) (Model, tea.Cmd) {
	t.Helper()
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	return step(m, runsview.OpenRunMsg{Repo: "o/r", Run: openRun})
}

func TestOpenRunWatchesItsJobsAndShowsRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, cmd := onRunScreen(t, f)

	if len(f.runCalls) != 1 || f.runCalls[0] != "o/r/7" {
		t.Errorf("run watch calls %v", f.runCalls)
	}
	v := view(m)
	if !strings.Contains(v, "o/r #7 build") || !strings.Contains(v, "loading jobs") {
		t.Errorf("run screen:\n%s", v)
	}
	if cmd == nil {
		t.Error("no command waiting for job events")
	}
}

func TestRunEventsReachRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, wait := onRunScreen(t, f)

	f.runChans[0] <- watch.RunEvent{Jobs: []runs.Job{{ID: 70, Name: "compile", Status: "in_progress"}}}
	m, next := step(m, wait())
	if v := view(m); !strings.Contains(v, "compile") {
		t.Errorf("job not shown:\n%s", v)
	}
	if next == nil {
		t.Error("no command to wait for the next job event")
	}
}

func TestEscLeavesRunScreenAndStopsJobWatch(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	m, cmd := step(m, keyMsg("esc"))
	m, _ = step(m, cmd())
	if v := view(m); !strings.Contains(v, "1 Runs") || strings.Contains(v, "loading jobs") {
		t.Errorf("not back on tabs:\n%s", v)
	}
	if f.runCtxs[0].Err() == nil {
		t.Error("job watch not cancelled")
	}
	if f.ctxs[0].Err() != nil {
		t.Error("repo watch cancelled when leaving the run screen")
	}
}

func TestRepoWatchUpdatesOpenRunHeader(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)
	done := openRun
	done.Status, done.Conclusion = "completed", "failure"

	f.chans[0] <- watch.Event{Repo: "o/r", Runs: []runs.Run{done}, Changes: []runs.Change{{Prev: &openRun, Run: done}}}
	m, _ = step(m, m.wait()())
	if l := strings.Split(view(m), "\n")[0]; !strings.Contains(l, "fail") {
		t.Errorf("run header %q, want fail", l)
	}
}

func TestStaleJobEventsIgnoredAfterReopen(t *testing.T) {
	f := &fakeWatch{}
	m, waitOld := onRunScreen(t, f)
	m, cmd := step(m, keyMsg("esc"))
	m, _ = step(m, cmd())
	m, _ = step(m, runsview.OpenRunMsg{Repo: "o/r", Run: openRun})

	f.runChans[0] <- watch.RunEvent{Jobs: []runs.Job{{ID: 1, Name: "stale-job"}}}
	m, next := step(m, waitOld())
	if v := view(m); strings.Contains(v, "stale-job") {
		t.Errorf("stale job shown:\n%s", v)
	}
	if next != nil {
		t.Error("kept waiting on the old job channel")
	}
}

func TestQuitFromRunScreenStopsBothWatches(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	_, cmd := step(m, keyMsg("q"))
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit")
	}
	if f.ctxs[0].Err() == nil || f.runCtxs[0].Err() == nil {
		t.Error("watches not cancelled on quit")
	}
}

var logJob = runs.Job{ID: 70, Name: "compile", Status: "completed", Conclusion: "failure"}

// onLogScreen opens the log of job 70 from the run screen and returns the
// command that fetches it.
func onLogScreen(t *testing.T, f *fakeWatch) (Model, tea.Cmd) {
	t.Helper()
	m, _ := onRunScreen(t, f)
	m, fetch := step(m, runview.OpenLogMsg{Repo: "o/r", Job: logJob})
	if fetch == nil {
		t.Fatal("opening a log returned no fetch command")
	}
	return m, fetch
}

func TestOpenLogFetchesAndShowsIt(t *testing.T) {
	f := &fakeWatch{logBody: []joblog.Line{{Text: "hello from the runner"}}}
	m, fetch := onLogScreen(t, f)

	if v := view(m); !strings.Contains(v, "compile") || !strings.Contains(v, "loading log") {
		t.Errorf("log screen before fetch:\n%s", v)
	}
	m, _ = step(m, fetch())
	if len(f.logCalls) != 1 || f.logCalls[0] != "o/r/70" {
		t.Errorf("log fetches %v", f.logCalls)
	}
	if v := view(m); !strings.Contains(v, "hello from the runner") {
		t.Errorf("log not shown:\n%s", v)
	}
}

func TestEscFromLogReturnsToRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, fetch := onLogScreen(t, f)
	m, _ = step(m, fetch())

	m, cmd := step(m, keyMsg("esc"))
	m, _ = step(m, cmd())
	if v := view(m); !strings.Contains(v, "o/r #7 build") || strings.Contains(v, "loading log") {
		t.Errorf("not back on run screen:\n%s", v)
	}
	if f.runCtxs[0].Err() != nil {
		t.Error("job watch cancelled by leaving the log")
	}
}

func TestReloadFetchesAgain(t *testing.T) {
	f := &fakeWatch{}
	m, fetch := onLogScreen(t, f)
	m, _ = step(m, fetch())

	m, cmd := step(m, keyMsg("r"))
	m, refetch := step(m, cmd())
	if refetch == nil {
		t.Fatal("reload returned no fetch command")
	}
	step(m, refetch())
	if len(f.logCalls) != 2 {
		t.Errorf("log fetched %d times, want 2", len(f.logCalls))
	}
}

func TestStaleLogResultIgnored(t *testing.T) {
	f := &fakeWatch{logBody: []joblog.Line{{Text: "stale log"}}}
	m, staleFetch := onLogScreen(t, f)
	m, cmd := step(m, keyMsg("esc"))
	m, _ = step(m, cmd())
	other := logJob
	other.ID, other.Name = 71, "other"
	m, _ = step(m, runview.OpenLogMsg{Repo: "o/r", Job: other})

	m, _ = step(m, staleFetch())
	if v := view(m); strings.Contains(v, "stale log") {
		t.Errorf("stale log shown:\n%s", v)
	}
}

func TestQWhileSearchingLogIsText(t *testing.T) {
	f := &fakeWatch{logBody: []joblog.Line{{Text: "a quiet line"}}}
	m, fetch := onLogScreen(t, f)
	m, _ = step(m, fetch())

	m, _ = step(m, keyMsg("/"))
	m, cmd := step(m, keyMsg("q"))
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q in log search quit the app")
		}
	}
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if v := view(m); !strings.Contains(v, "/q 1/1") {
		t.Errorf("search for q not applied:\n%s", v)
	}
}
