package app

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/actions"
	"github.com/EvilNick2/gh-argus/internal/caches"
	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/repos"
	"github.com/EvilNick2/gh-argus/internal/runners"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/runsview"
	"github.com/EvilNick2/gh-argus/internal/runview"
	"github.com/EvilNick2/gh-argus/internal/theme"
	"github.com/EvilNick2/gh-argus/internal/watch"
	"github.com/EvilNick2/gh-argus/internal/workflows"
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
	// logErrs are returned by successive log fetches, before logBody.
	logErrs []error

	acts   []string
	actErr error

	seeds     map[string]*watch.Seed
	remaining int
	clock     time.Time

	wfCalls []string
	wfs     map[string][]workflows.Workflow
	wfSets  []string

	specs      map[int64]workflows.DispatchSpec
	refSpecs   map[string]map[int64]workflows.DispatchSpec
	refErr     map[string]error
	specRefs   []string
	dispatches []string
	envCalls   []string

	recentCalls []string
	recent      map[string][]runs.Run

	deletedRuns []string
	deleteErr   map[int64]error
	runnerCalls []string
	runnerErr   map[string]error
	cacheCalls  []string
	caches      map[string][]caches.Cache
	deleted     []string
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
		WatchRun: func(ctx context.Context, repo string, id int64, attempt int) <-chan watch.RunEvent {
			ch := make(chan watch.RunEvent, 4)
			call := fmt.Sprintf("%s/%d", repo, id)
			if attempt > 0 {
				call += fmt.Sprintf("#%d", attempt)
			}
			f.runCalls = append(f.runCalls, call)
			f.runCtxs = append(f.runCtxs, ctx)
			f.runChans = append(f.runChans, ch)
			return ch
		},
		FetchLog: func(ctx context.Context, repo string, id int64) ([]joblog.Line, error) {
			f.logCalls = append(f.logCalls, fmt.Sprintf("%s/%d", repo, id))
			if len(f.logErrs) > 0 {
				err := f.logErrs[0]
				f.logErrs = f.logErrs[1:]
				return nil, err
			}
			return f.logBody, nil
		},
		Act: func(ctx context.Context, repo string, id int64, k actions.Kind) error {
			f.acts = append(f.acts, fmt.Sprintf("%s/%d %v", repo, id, k))
			return f.actErr
		},
		Seed:      func(repo string) *watch.Seed { return f.seeds[repo] },
		Remaining: func() int { return f.remaining },
		RunAttempt: func(ctx context.Context, repo string, id int64, n int) (runs.Run, error) {
			return runs.Run{ID: id, RunNumber: 7, Name: "build", Status: "completed", Conclusion: "failure", RunAttempt: n}, nil
		},
		ListWorkflows: func(ctx context.Context, repo string) ([]workflows.Workflow, error) {
			f.wfCalls = append(f.wfCalls, repo)
			return f.wfs[repo], nil
		},
		DispatchSpec: func(ctx context.Context, repo string, wf workflows.Workflow, ref string) (string, workflows.DispatchSpec, error) {
			f.specRefs = append(f.specRefs, ref)
			if ref == "" {
				return "main", f.specs[wf.ID], nil
			}
			if err := f.refErr[ref]; err != nil {
				return ref, workflows.DispatchSpec{}, err
			}
			if specs, ok := f.refSpecs[ref]; ok {
				return ref, specs[wf.ID], nil
			}
			return ref, f.specs[wf.ID], nil
		},
		RecentRuns: func(ctx context.Context, repo string) ([]runs.Run, error) {
			f.recentCalls = append(f.recentCalls, repo)
			return f.recent[repo], nil
		},
		ListRunners: func(ctx context.Context, repo string) ([]runners.Runner, error) {
			f.runnerCalls = append(f.runnerCalls, repo)
			if err := f.runnerErr[repo]; err != nil {
				return nil, err
			}
			return []runners.Runner{{ID: 21, Name: "dockhand-relay", OS: "Linux", Status: "online"}}, nil
		},
		ListCaches: func(ctx context.Context, repo string) ([]caches.Cache, error) {
			f.cacheCalls = append(f.cacheCalls, repo)
			return f.caches[repo], nil
		},
		DeleteRun: func(ctx context.Context, repo string, id int64) error {
			if err := f.deleteErr[id]; err != nil {
				return err
			}
			f.deletedRuns = append(f.deletedRuns, fmt.Sprintf("%s/%d", repo, id))
			return nil
		},
		DeleteCache: func(ctx context.Context, repo string, id int64) error {
			f.deleted = append(f.deleted, fmt.Sprintf("%s/%d", repo, id))
			return nil
		},
		Environments: func(ctx context.Context, repo string) ([]string, error) {
			f.envCalls = append(f.envCalls, repo)
			return []string{"github-pages", "production"}, nil
		},
		Branches: func(ctx context.Context, repo string) ([]string, error) {
			return []string{"dev", "main"}, nil
		},
		Dispatch: func(ctx context.Context, repo string, id int64, ref string, inputs map[string]string) error {
			f.dispatches = append(f.dispatches, fmt.Sprintf("%s/%d@%s %v", repo, id, ref, inputs))
			return nil
		},
		SetWorkflow: func(ctx context.Context, repo string, id int64, enabled bool) error {
			f.wfSets = append(f.wfSets, fmt.Sprintf("%s/%d %v", repo, id, enabled))
			return nil
		},
		Now: func() time.Time {
			if f.clock.IsZero() {
				return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
			}
			return f.clock
		},
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
	var msg tea.Msg
	for _, cmd := range m.Init()().(tea.BatchMsg) {
		if got, ok := cmd().(eventMsg); ok {
			msg = got
		}
	}
	m, next := step(m, msg)
	if v := view(m); !strings.Contains(v, "+ #7") || !strings.Contains(v, "build") {
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
	if !strings.Contains(v, "o/r  #7 build") || !strings.Contains(v, "loading jobs") {
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
	if v := view(m); !strings.Contains(v, "[1] Runs") || strings.Contains(v, "loading jobs") {
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
	if l := strings.Split(view(m), "\n")[2]; !strings.Contains(l, "x failed") {
		t.Errorf("run state line %q, want x failed", l)
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
	if v := view(m); !strings.Contains(v, "o/r  #7 build") || strings.Contains(v, "loading log") {
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

var failedRun = runs.Run{ID: 16, RunNumber: 16, Name: "Manifest check", Status: "completed", Conclusion: "failure"}

// onRunsTab returns a model on the Runs tab with failedRun under the cursor.
func onRunsTab(t *testing.T, f *fakeWatch) Model {
	t.Helper()
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	f.chans[0] <- watch.Event{Repo: "o/r", Initial: true, Runs: []runs.Run{failedRun}}
	m, _ = step(m, m.wait()())
	return m
}

func TestRerunFailedAsksThenActsOnY(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("r"))
	if v := view(m); !strings.Contains(v, "rerun failed jobs of #16 Manifest check? y/n") {
		t.Errorf("no prompt:\n%s", v)
	}
	if len(f.acts) != 0 {
		t.Fatal("acted before confirming")
	}
	m, cmd := step(m, keyMsg("y"))
	if cmd == nil {
		t.Fatal("y returned no command")
	}
	m, _ = step(m, cmd())
	if len(f.acts) != 1 || f.acts[0] != "o/r/16 rerun failed jobs" {
		t.Errorf("acts %v", f.acts)
	}
	if v := view(m); !strings.Contains(v, "rerun failed jobs requested for #16") {
		t.Errorf("no confirmation message:\n%s", v)
	}
}

func TestOtherKeyCancelsPrompt(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("R"))
	m, cmd := step(m, keyMsg("q"))
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q at the prompt quit instead of declining")
		}
	}
	if len(f.acts) != 0 {
		t.Errorf("acted after declining: %v", f.acts)
	}
	if v := view(m); strings.Contains(v, "y/n") {
		t.Errorf("prompt still shown:\n%s", v)
	}
}

func TestDisallowedActionExplainsAndDoesNotPrompt(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("c"))
	v := view(m)
	if strings.Contains(v, "y/n") || !strings.Contains(v, "cannot cancel #16, it has already completed") {
		t.Errorf("view:\n%s", v)
	}
}

func TestCancelFromRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	m, _ = step(m, keyMsg("c"))
	if v := view(m); !strings.Contains(v, "cancel #7 build? y/n") {
		t.Errorf("cancel prompt:\n%s", v)
	}
	m, cmd := step(m, keyMsg("y"))
	if cmd == nil {
		t.Fatal("y returned no command")
	}
	step(m, cmd())
	if len(f.acts) != 1 || f.acts[0] != "o/r/7 cancel" {
		t.Errorf("acts %v", f.acts)
	}
}

func TestActionErrorShown(t *testing.T) {
	f := &fakeWatch{actErr: &actions.StatusError{StatusCode: 403, Message: "Resource not accessible by integration"}}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("R"))
	m, cmd := step(m, keyMsg("y"))
	m, _ = step(m, cmd())
	if v := view(m); !strings.Contains(v, "rerun all jobs failed: 403 Resource not accessible") {
		t.Errorf("view:\n%s", v)
	}
}

func TestMessageClearsOnNextKey(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("c"))
	m, _ = step(m, keyMsg("j"))
	if v := view(m); strings.Contains(v, "cannot cancel") {
		t.Errorf("message not cleared:\n%s", v)
	}
}

func TestRefusalPhrasing(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	passed := runs.Run{ID: 72, RunNumber: 72, Name: "pages", Status: "completed", Conclusion: "success"}
	f.chans[0] <- watch.Event{Repo: "o/r", Initial: true, Runs: []runs.Run{passed}}
	m, _ = step(m, m.wait()())

	m, _ = step(m, keyMsg("r"))
	if v := view(m); !strings.Contains(v, "cannot rerun failed jobs of #72, it succeeded") {
		t.Errorf("view:\n%s", v)
	}
}

func TestSavedRunsShowBeforeFirstPoll(t *testing.T) {
	f := &fakeWatch{seeds: map[string]*watch.Seed{
		"o/r": {ETag: `"e"`, Runs: []runs.Run{{ID: 1, RunNumber: 41, Name: "release", Status: "completed", Conclusion: "success"}}},
	}}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))

	v := view(m)
	if !strings.Contains(v, "+ #41") || !strings.Contains(v, "cached, refreshing") {
		t.Errorf("saved runs not shown at start:\n%s", v)
	}
}

// runAll runs cmd and any commands it batches, feeding each message to m.
func runAll(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = runAll(m, c)
		}
	case nil:
	default:
		var next tea.Cmd
		m, next = step(m, msg)
		m = runAll(m, next)
	}
	return m
}

func onWorkflowsTab(t *testing.T, f *fakeWatch) Model {
	t.Helper()
	m := sized(New(f.deps(), newPicker(), []string{"o/a", "o/b"}))
	m, cmd := step(m, keyMsg("2"))
	return runAll(m, cmd)
}

var buildWF = workflows.Workflow{ID: 1, Name: "Build and publish", Path: ".github/workflows/build.yml", State: "active"}

func TestWorkflowsTabLoadsEachRepo(t *testing.T) {
	f := &fakeWatch{wfs: map[string][]workflows.Workflow{"o/a": {buildWF}}}
	m := onWorkflowsTab(t, f)

	if !slices.Equal(f.wfCalls, []string{"o/a", "o/b"}) {
		t.Errorf("workflow loads %v", f.wfCalls)
	}
	v := view(m)
	if !strings.Contains(v, "Build and publish") || !strings.Contains(v, "no workflows") {
		t.Errorf("view:\n%s", v)
	}
}

func TestDisableWorkflowConfirmsThenReloads(t *testing.T) {
	f := &fakeWatch{wfs: map[string][]workflows.Workflow{"o/a": {buildWF}}}
	m := onWorkflowsTab(t, f)

	m, _ = step(m, keyMsg("d"))
	if v := view(m); !strings.Contains(v, "disable workflow Build and publish? y/n") {
		t.Errorf("no prompt:\n%s", v)
	}
	m, cmd := step(m, keyMsg("y"))
	loadsBefore := len(f.wfCalls)
	m = runAll(m, cmd)
	if len(f.wfSets) != 1 || f.wfSets[0] != "o/a/1 false" {
		t.Errorf("workflow sets %v", f.wfSets)
	}
	if len(f.wfCalls) != loadsBefore+1 || f.wfCalls[len(f.wfCalls)-1] != "o/a" {
		t.Errorf("o/a not reloaded after disabling: %v", f.wfCalls)
	}
	if v := view(m); !strings.Contains(v, "disabled Build and publish") {
		t.Errorf("no confirmation:\n%s", v)
	}
}

func TestEnableAlreadyEnabledExplains(t *testing.T) {
	f := &fakeWatch{wfs: map[string][]workflows.Workflow{"o/a": {buildWF}}}
	m := onWorkflowsTab(t, f)

	m, _ = step(m, keyMsg("e"))
	v := view(m)
	if strings.Contains(v, "y/n") || !strings.Contains(v, "Build and publish is already enabled") {
		t.Errorf("view:\n%s", v)
	}
	if len(f.wfSets) != 0 {
		t.Errorf("sent %v", f.wfSets)
	}
}

func TestRunKeysDoNothingOnWorkflowsTab(t *testing.T) {
	f := &fakeWatch{wfs: map[string][]workflows.Workflow{"o/a": {buildWF}}}
	m := onWorkflowsTab(t, f)

	m, _ = step(m, keyMsg("c"))
	if v := view(m); strings.Contains(v, "y/n") || strings.Contains(v, "cannot") {
		t.Errorf("run key acted on workflows tab:\n%s", v)
	}
}

func TestEnterDispatchesAfterCheckingAndConfirming(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {buildWF}},
		specs: map[int64]workflows.DispatchSpec{1: {Dispatchable: true}},
	}
	m := onWorkflowsTab(t, f)

	m, check := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if v := view(m); !strings.Contains(v, "checking Build and publish") {
		t.Errorf("no checking message:\n%s", v)
	}
	m = runAll(m, check)
	if v := view(m); !strings.Contains(v, "run Build and publish on main? y/n") {
		t.Fatalf("no prompt:\n%s", v)
	}
	m, send := step(m, keyMsg("y"))
	m = runAll(m, send)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/1@main map[]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
	if v := view(m); !strings.Contains(v, "dispatched Build and publish on main") {
		t.Errorf("no confirmation:\n%s", v)
	}
}

func TestEnterRefusesWithReason(t *testing.T) {
	dynamic := workflows.Workflow{ID: 2, Name: "pages-build-deployment", Path: "dynamic/pages/pages-build-deployment", State: "active"}
	off := workflows.Workflow{ID: 3, Name: "Nightly", Path: ".github/workflows/n.yml", State: "disabled_manually"}
	push := workflows.Workflow{ID: 4, Name: "Push only", Path: ".github/workflows/p.yml", State: "active"}
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {dynamic, off, push}},
		specs: map[int64]workflows.DispatchSpec{4: {Dispatchable: false}},
	}
	m := onWorkflowsTab(t, f)

	want := []string{
		"pages-build-deployment is dynamic and cannot be dispatched",
		"Nightly is disabled, enable it first",
		"Push only has no workflow_dispatch trigger",
	}
	for i, w := range want {
		if i > 0 {
			m, _ = step(m, keyMsg("j"))
		}
		var cmd tea.Cmd
		m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		m = runAll(m, cmd)
		if v := view(m); !strings.Contains(v, w) || strings.Contains(v, "y/n") {
			t.Errorf("row %d: want %q without a prompt in:\n%s", i, w, v)
		}
	}
	if len(f.dispatches) != 0 {
		t.Errorf("dispatched %v", f.dispatches)
	}
}

var needyWF = workflows.Workflow{ID: 5, Name: "Release", Path: ".github/workflows/release.yml", State: "active"}

// openForm presses key on the only workflow and runs the dispatch check.
func openForm(t *testing.T, f *fakeWatch, k tea.Msg) Model {
	t.Helper()
	m := onWorkflowsTab(t, f)
	m, cmd := step(m, k)
	return runAll(m, cmd)
}

func TestRequiredInputOpensFormAndSubmitDispatches(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "version", Type: "string", Required: true}}}},
	}
	m := openForm(t, f, tea.KeyPressMsg{Code: tea.KeyEnter})

	if v := view(m); !strings.Contains(v, "run Release on main") || !strings.Contains(v, "version*") {
		t.Fatalf("form not shown:\n%s", v)
	}
	for _, r := range "1.2.q" {
		m, _ = step(m, keyMsg(string(r)))
	}
	if len(f.dispatches) != 0 {
		t.Fatal("typing q in the form did something other than type")
	}
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runAll(m, cmd)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/5@main map[version:1.2.q]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
	v := view(m)
	if !strings.Contains(v, "dispatched Release on main") || !strings.Contains(v, "[2] Workflows") {
		t.Errorf("not back on the tab with a confirmation:\n%s", v)
	}
}

func TestIOpensFormForDefaultedInputs(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "force", Type: "boolean", Default: "false"}}}},
	}
	m := openForm(t, f, keyMsg("i"))

	m, _ = step(m, keyMsg("space"))
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runAll(m, cmd)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/5@main map[force:true]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
}

func TestEnterWithOnlyDefaultedInputsStillPrompts(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "force", Type: "boolean", Default: "false"}}}},
	}
	m := openForm(t, f, tea.KeyPressMsg{Code: tea.KeyEnter})

	if v := view(m); !strings.Contains(v, "run Release on main? y/n") {
		t.Errorf("view:\n%s", v)
	}
}

func TestEscClosesFormWithoutDispatching(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "version", Type: "string", Required: true}}}},
	}
	m := openForm(t, f, tea.KeyPressMsg{Code: tea.KeyEnter})

	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runAll(m, cmd)
	if v := view(m); !strings.Contains(v, "[2] Workflows") || strings.Contains(v, "version*") {
		t.Errorf("form not closed:\n%s", v)
	}
	if len(f.dispatches) != 0 {
		t.Errorf("dispatched %v", f.dispatches)
	}
}

func TestIPicksBranchForWorkflowWithoutInputs(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {buildWF}},
		specs: map[int64]workflows.DispatchSpec{1: {Dispatchable: true}},
	}
	m := openForm(t, f, keyMsg("i"))

	if v := view(m); !strings.Contains(v, "run Build and publish on main") || !strings.Contains(v, "2 branches") {
		t.Fatalf("form not shown with branches:\n%s", v)
	}
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runAll(m, cmd)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/1@dev map[]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
	if v := view(m); !strings.Contains(v, "dispatched Build and publish on dev") {
		t.Errorf("view:\n%s", v)
	}
}

func TestMetricsTabLoadsRecentRunsPerRepo(t *testing.T) {
	r := runs.Run{ID: 1, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "success", RunAttempt: 1,
		CreatedAt: time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC), RunStartedAt: time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 4, 11, 0, 9, 0, time.UTC)}
	f := &fakeWatch{recent: map[string][]runs.Run{"o/a": {r}}}
	m := sized(New(f.deps(), newPicker(), []string{"o/a", "o/b"}))

	m, cmd := step(m, keyMsg("3"))
	m = runAll(m, cmd)
	if !slices.Equal(f.recentCalls, []string{"o/a", "o/b"}) {
		t.Errorf("recent run loads %v", f.recentCalls)
	}
	v := view(m)
	if !strings.Contains(v, "success") || !strings.Contains(v, "100%") || !strings.Contains(v, "9s") {
		t.Errorf("metrics not shown:\n%s", v)
	}
	if strings.Contains(v, "not built yet") {
		t.Errorf("placeholder shown:\n%s", v)
	}
}

func TestRunnersTabShowsRunnersAndNotPermitted(t *testing.T) {
	f := &fakeWatch{runnerErr: map[string]error{"o/b": &fetch.StatusError{StatusCode: 404, Path: "/x"}}}
	m := sized(New(f.deps(), newPicker(), []string{"o/a", "o/b"}))

	m, cmd := step(m, keyMsg("5"))
	m = runAll(m, cmd)
	if !slices.Equal(f.runnerCalls, []string{"o/a", "o/b"}) {
		t.Errorf("runner loads %v", f.runnerCalls)
	}
	v := view(m)
	if !strings.Contains(v, "dockhand-relay") || !strings.Contains(v, "not permitted") || strings.Contains(v, "not built yet") {
		t.Errorf("view:\n%s", v)
	}
}

func TestDeleteCacheConfirmsThenReloads(t *testing.T) {
	f := &fakeWatch{caches: map[string][]caches.Cache{
		"o/a": {{ID: 505, Key: "Linux-node-208b2f", Ref: "refs/heads/main", SizeInBytes: 2048}},
	}}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	m, cmd := step(m, keyMsg("4"))
	m = runAll(m, cmd)

	m, _ = step(m, keyMsg("d"))
	if v := view(m); !strings.Contains(v, "delete cache Linux-node-208b2f from o/a? y/n") {
		t.Fatalf("no prompt:\n%s", v)
	}
	loads := len(f.cacheCalls)
	m, cmd = step(m, keyMsg("y"))
	m = runAll(m, cmd)
	if len(f.deleted) != 1 || f.deleted[0] != "o/a/505" {
		t.Errorf("deleted %v", f.deleted)
	}
	if len(f.cacheCalls) != loads+1 {
		t.Errorf("caches not reloaded after delete: %v", f.cacheCalls)
	}
	if v := view(m); !strings.Contains(v, "deleted cache Linux-node-208b2f") {
		t.Errorf("view:\n%s", v)
	}
}

func TestDOnEmptyCacheTabDoesNothing(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	m, cmd := step(m, keyMsg("4"))
	m = runAll(m, cmd)

	m, _ = step(m, keyMsg("d"))
	if v := view(m); strings.Contains(v, "y/n") || !strings.Contains(v, "no caches") {
		t.Errorf("view:\n%s", v)
	}
}

func TestChromeHeaderTabsAndStatusBar(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a", "o/b"}))

	lines := strings.Split(view(m), "\n")
	if len(lines) != 20 {
		t.Fatalf("view is %d lines, want the full height of 20", len(lines))
	}
	if !strings.Contains(lines[0], "argus") || !strings.Contains(lines[0], "watching 2 repos") {
		t.Errorf("header %q", lines[0])
	}
	if !strings.Contains(lines[1], "[1] Runs") || !strings.Contains(lines[1], "[5] Runners") {
		t.Errorf("tab line %q", lines[1])
	}
	if last := lines[19]; !strings.Contains(last, "q quit") {
		t.Errorf("status bar %q, want hints", last)
	}
}

func TestPromptShowsInStatusBar(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("r"))
	lines := strings.Split(view(m), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "rerun failed jobs of #16 Manifest check? y/n") {
		t.Errorf("status bar %q", last)
	}
}

func TestLightBackgroundSwitchesPalette(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	defer theme.SetDark(true)

	step(m, tea.BackgroundColorMsg{Color: color.White})
	if theme.Current().Text == theme.Current().Bar || theme.Current().Plume != lipgloss.Color("#12798A") {
		t.Errorf("palette not switched to light: %+v", theme.Current())
	}
}

func TestHeaderShowsRequestsLeft(t *testing.T) {
	f := &fakeWatch{remaining: 4987}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	if l := strings.Split(view(m), "\n")[0]; !strings.Contains(l, "4987 requests left") {
		t.Errorf("header %q", l)
	}
	f.remaining = -1
	if l := strings.Split(view(m), "\n")[0]; strings.Contains(l, "requests left") {
		t.Errorf("header %q shows a count before any request", l)
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	m, _ = step(m, keyMsg("?"))
	v := view(m)
	if !strings.Contains(v, "keys") || !strings.Contains(v, "rerun failed jobs") || !strings.Contains(v, "Workflows") {
		t.Fatalf("help not shown:\n%s", v)
	}
	m, _ = step(m, keyMsg("G"))
	if v := view(m); !strings.Contains(v, "double-click") || !strings.Contains(v, "shift+drag") {
		t.Errorf("G did not scroll help to the mouse keys:\n%s", v)
	}
	m, _ = step(m, keyMsg("g"))
	m, _ = step(m, keyMsg("?"))
	if v := view(m); strings.Contains(v, "rerun failed jobs") {
		t.Errorf("? did not close help:\n%s", v)
	}
	m, _ = step(m, keyMsg("?"))
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if v := view(m); strings.Contains(v, "rerun failed jobs") {
		t.Errorf("esc did not close help:\n%s", v)
	}
}

func TestHelpSwallowsKeysButQuitStillWorks(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("?"))
	m, _ = step(m, keyMsg("r"))
	if v := view(m); strings.Contains(v, "y/n") {
		t.Errorf("r acted behind the help screen:\n%s", v)
	}
	_, cmd := step(m, keyMsg("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit from the help screen")
	}
}

func TestQuestionMarkInLogSearchIsText(t *testing.T) {
	f := &fakeWatch{logBody: []joblog.Line{{Text: "why?"}}}
	m, fetch := onLogScreen(t, f)
	m, _ = step(m, fetch())

	m, _ = step(m, keyMsg("/"))
	m, _ = step(m, keyMsg("?"))
	if v := view(m); strings.Contains(v, "rerun failed jobs") {
		t.Errorf("? opened help while typing a search:\n%s", v)
	}
}

func TestStatusBarDropsLegendBeforeHints(t *testing.T) {
	f := &fakeWatch{}
	m := New(f.deps(), newPicker(), []string{"o/a"})

	wide, _ := step(m, tea.WindowSizeMsg{Width: 160, Height: 20})
	lines := strings.Split(view(wide), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "+ pass") || !strings.Contains(last, "q quit") {
		t.Errorf("wide status bar %q, want legend and hints", last)
	}
	narrow, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	lines = strings.Split(view(narrow), "\n")
	if last := lines[len(lines)-1]; strings.Contains(last, "+ pass") || !strings.Contains(last, "enter open") {
		t.Errorf("narrow status bar %q, want hints without the legend", last)
	}
}

func leftClick(x, y int) tea.Msg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestMouseIsEnabled(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	if mode := m.View().MouseMode; mode != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, want cell motion", mode)
	}
}

func TestClickTabSwitchesTab(t *testing.T) {
	f := &fakeWatch{wfs: map[string][]workflows.Workflow{"o/a": {buildWF}}}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	// The tab line is " [1] Runs   [2] Workflows ...", so x 16 is on [2].
	m, cmd := step(m, leftClick(16, 1))
	m = runAll(m, cmd)
	if v := view(m); !strings.Contains(v, "Build and publish") {
		t.Errorf("click on [2] did not open Workflows:\n%s", v)
	}
}

func TestDoubleClickRunOpensRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)
	f.clock = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	// The runs pane starts two lines down, its first run two rows into it.
	m, _ = step(m, leftClick(40, 4))
	f.clock = f.clock.Add(200 * time.Millisecond)
	m, cmd := step(m, leftClick(40, 4))
	if cmd == nil {
		t.Fatal("double click returned no command")
	}
	m, _ = step(m, cmd())
	if len(f.runCalls) != 1 || f.runCalls[0] != "o/r/16" {
		t.Errorf("run watch calls %v, want run 16 opened", f.runCalls)
	}
}

func TestSlowSecondClickIsNotADoubleClick(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)
	f.clock = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	m, _ = step(m, leftClick(40, 4))
	f.clock = f.clock.Add(time.Second)
	_, cmd := step(m, leftClick(40, 4))
	if cmd != nil {
		t.Errorf("two clicks a second apart opened something: %#v", cmd())
	}
}

func TestWheelOnRunsTab(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a", "o/b"}))

	m, _ = step(m, tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if v := view(m); !strings.Contains(v, "─ o/b ") {
		t.Errorf("wheel over repos did not select o/b:\n%s", v)
	}
}

func TestMouseBackButtonLeavesRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	m, cmd := step(m, tea.MouseClickMsg{X: 10, Y: 10, Button: tea.MouseBackward})
	m = runAll(m, cmd)
	if v := view(m); !strings.Contains(v, "[1] Runs") {
		t.Errorf("back button did not return to tabs:\n%s", v)
	}
}

func TestMouseIgnoredWhilePromptIsUp(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)
	f.clock = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	m, _ = step(m, keyMsg("r"))
	m, _ = step(m, leftClick(16, 1))
	if v := view(m); !strings.Contains(v, "y/n") || strings.Contains(v, "loading") {
		t.Errorf("click acted while the prompt was up:\n%s", v)
	}
}

func TestClickClosesHelp(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	m, _ = step(m, keyMsg("?"))
	m, _ = step(m, leftClick(30, 10))
	if v := view(m); strings.Contains(v, "rerun failed jobs") {
		t.Errorf("click did not close help:\n%s", v)
	}
}

func TestDoubleClickWorkflowStartsDispatch(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {buildWF}},
		specs: map[int64]workflows.DispatchSpec{1: {Dispatchable: true}},
		clock: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}
	m := onWorkflowsTab(t, f)

	// Below the tab line: the pane border, the o/a header, then the workflow.
	m, _ = step(m, leftClick(10, 4))
	f.clock = f.clock.Add(100 * time.Millisecond)
	m, cmd := step(m, leftClick(10, 4))
	m = runAll(m, cmd)
	if v := view(m); !strings.Contains(v, "run Build and publish on main? y/n") {
		t.Errorf("double click did not offer to dispatch:\n%s", v)
	}
}

func TestClickSelectsCacheThenDeleteUsesIt(t *testing.T) {
	f := &fakeWatch{caches: map[string][]caches.Cache{"o/a": {
		{ID: 1, Key: "first", Ref: "refs/heads/main"},
		{ID: 2, Key: "second", Ref: "refs/heads/main"},
	}}}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	m, cmd := step(m, keyMsg("4"))
	m = runAll(m, cmd)

	m, _ = step(m, leftClick(10, 5))
	m, _ = step(m, keyMsg("d"))
	if v := view(m); !strings.Contains(v, "delete cache second from o/a? y/n") {
		t.Errorf("view:\n%s", v)
	}
}

func TestWheelOnMetricsAndRunners(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	for _, tab := range []string{"3", "5"} {
		var cmd tea.Cmd
		m, cmd = step(m, keyMsg(tab))
		m = runAll(m, cmd)
		// Must not panic or act with nothing to select.
		m, _ = step(m, tea.MouseWheelMsg{X: 10, Y: 6, Button: tea.MouseWheelDown})
		m, _ = step(m, leftClick(10, 6))
	}
}

func TestMouseInPickerTicksRepos(t *testing.T) {
	f := &fakeWatch{clock: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	m := sized(New(f.deps(), newPicker(), nil))

	// orpheus is the second row, at screen y 4, its box at x 2 to 4.
	m, _ = step(m, leftClick(3, 4))
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = step(m, cmd())
	if len(f.calls) != 1 || !slices.Equal(f.calls[0], []string{"EvilNick2/orpheus"}) {
		t.Errorf("watch calls %v, want orpheus ticked by the click", f.calls)
	}
}

func TestDoubleClickBooleanInForm(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "force", Type: "boolean", Default: "false"}}}},
		clock: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}
	m := openForm(t, f, keyMsg("i"))

	// Below the header bar: border, repo, blank, branch, then force at y 5.
	m, _ = step(m, leftClick(10, 5))
	f.clock = f.clock.Add(100 * time.Millisecond)
	m, _ = step(m, leftClick(10, 5))
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runAll(m, cmd)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/5@main map[force:true]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
}

func TestWheelScrollsHelpAndClickCloses(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))

	m, _ = step(m, keyMsg("?"))
	for range 20 {
		m, _ = step(m, tea.MouseWheelMsg{X: 30, Y: 10, Button: tea.MouseWheelDown})
	}
	if v := view(m); !strings.Contains(v, "shift+drag") {
		t.Errorf("wheel did not scroll help:\n%s", v)
	}
	m, _ = step(m, leftClick(30, 10))
	if v := view(m); strings.Contains(v, "shift+drag") {
		t.Error("click did not close help")
	}
}

func TestRRefreshesTheOtherTabs(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/a"}))
	counts := map[string]func() int{
		"2": func() int { return len(f.wfCalls) },
		"3": func() int { return len(f.recentCalls) },
		"4": func() int { return len(f.cacheCalls) },
		"5": func() int { return len(f.runnerCalls) },
	}
	for _, tab := range []string{"2", "3", "4", "5"} {
		var cmd tea.Cmd
		m, cmd = step(m, keyMsg(tab))
		m = runAll(m, cmd)
		before := counts[tab]()
		m, cmd = step(m, keyMsg("r"))
		if v := view(m); !strings.Contains(v, "refreshing") {
			t.Errorf("tab %s: no refreshing message:\n%s", tab, v)
		}
		m = runAll(m, cmd)
		if counts[tab]() != before+1 {
			t.Errorf("tab %s: r loaded %d times, want once", tab, counts[tab]()-before)
		}
	}
}

func TestRStillRerunsOnRunsTab(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("r"))
	if v := view(m); !strings.Contains(v, "rerun failed jobs of #16") {
		t.Errorf("view:\n%s", v)
	}
}

func TestForceCancelFromRunScreen(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	m, _ = step(m, keyMsg("C"))
	if v := view(m); !strings.Contains(v, "force cancel #7 build? y/n") {
		t.Fatalf("no prompt:\n%s", v)
	}
	m, cmd := step(m, keyMsg("y"))
	runAll(m, cmd)
	if len(f.acts) != 1 || f.acts[0] != "o/r/7 force cancel" {
		t.Errorf("acts %v", f.acts)
	}
}

func TestEnvironmentInputGetsRepoEnvironments(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: []workflows.Input{{Name: "target", Type: "environment", Required: true}}}},
	}
	m := openForm(t, f, tea.KeyPressMsg{Code: tea.KeyEnter})

	if v := view(m); !strings.Contains(v, "2 environments") {
		t.Errorf("form without environments:\n%s", v)
	}
}

func TestEnvironmentsNotFetchedWithoutEnvironmentInputs(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {buildWF}},
		specs: map[int64]workflows.DispatchSpec{1: {Dispatchable: true}},
	}
	openForm(t, f, keyMsg("i"))

	if len(f.envCalls) != 0 {
		t.Errorf("environments fetched for a workflow without environment inputs: %v", f.envCalls)
	}
}

var tagInput = []workflows.Input{{Name: "tag", Type: "string"}}

// submitOnBranch opens the form on needyWF, switches the branch to ref with
// left (the fake branches are dev and main) and submits.
func submitOnBranch(t *testing.T, f *fakeWatch) Model {
	t.Helper()
	m := openForm(t, f, keyMsg("i"))
	for _, r := range "x" {
		m, _ = step(m, keyMsg(string(r)))
	}
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	return runAll(m, cmd)
}

func TestSubmitOnDefaultBranchDoesNotRecheck(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: tagInput}},
	}
	m := openForm(t, f, keyMsg("i"))
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runAll(m, cmd)

	if !slices.Equal(f.specRefs, []string{""}) {
		t.Errorf("workflow file read for %q, want the default branch only", f.specRefs)
	}
	if len(f.dispatches) != 1 {
		t.Errorf("dispatches %v", f.dispatches)
	}
}

func TestSubmitOnOtherBranchWithSameInputsDispatches(t *testing.T) {
	f := &fakeWatch{
		wfs:   map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs: map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: tagInput}},
	}
	submitOnBranch(t, f)

	if !slices.Equal(f.specRefs, []string{"", "dev"}) {
		t.Errorf("workflow file read for %q, want default then dev", f.specRefs)
	}
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/5@dev map[tag:x]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
}

func TestSubmitOnBranchWithDifferentInputsReopensForm(t *testing.T) {
	devInputs := []workflows.Input{{Name: "tag", Type: "string"}, {Name: "channel", Type: "string", Required: true}}
	f := &fakeWatch{
		wfs:      map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs:    map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: tagInput}},
		refSpecs: map[string]map[int64]workflows.DispatchSpec{"dev": {5: {Dispatchable: true, Inputs: devInputs}}},
	}
	m := submitOnBranch(t, f)

	if len(f.dispatches) != 0 {
		t.Fatalf("dispatched despite different inputs: %v", f.dispatches)
	}
	v := view(m)
	if !strings.Contains(v, "run Release on dev") || !strings.Contains(v, "channel*") || !strings.Contains(v, "inputs differ on dev") {
		t.Fatalf("form not reopened for dev:\n%s", v)
	}
	// The tag typed before carries over. Fill channel and run.
	for _, r := range "beta" {
		m, _ = step(m, keyMsg(string(r)))
	}
	m, cmd := step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runAll(m, cmd)
	if len(f.dispatches) != 1 || f.dispatches[0] != "o/a/5@dev map[channel:beta tag:x]" {
		t.Errorf("dispatches %v", f.dispatches)
	}
}

func TestSubmitOnBranchWithoutTriggerStaysOnForm(t *testing.T) {
	f := &fakeWatch{
		wfs:      map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs:    map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: tagInput}},
		refSpecs: map[string]map[int64]workflows.DispatchSpec{"dev": {5: {Dispatchable: false}}},
	}
	m := submitOnBranch(t, f)

	if len(f.dispatches) != 0 {
		t.Errorf("dispatched %v", f.dispatches)
	}
	if v := view(m); !strings.Contains(v, "Release has no workflow_dispatch trigger on dev") {
		t.Errorf("view:\n%s", v)
	}
}

func TestSubmitOnBranchWithoutFileSaysSo(t *testing.T) {
	f := &fakeWatch{
		wfs:    map[string][]workflows.Workflow{"o/a": {needyWF}},
		specs:  map[int64]workflows.DispatchSpec{5: {Dispatchable: true, Inputs: tagInput}},
		refErr: map[string]error{"dev": &fetch.StatusError{StatusCode: 404, Path: "/x"}},
	}
	m := submitOnBranch(t, f)

	if v := view(m); !strings.Contains(v, ".github/workflows/release.yml does not exist on dev") {
		t.Errorf("view:\n%s", v)
	}
}

func TestTypingARunsFilterTakesEveryKey(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTab(t, f)

	m, _ = step(m, keyMsg("/"))
	for _, k := range []string{"r", "c", "q", "p", "2", "?"} {
		var cmd tea.Cmd
		m, cmd = step(m, keyMsg(k))
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatalf("%s quit while typing a filter", k)
			}
		}
	}
	v := view(m)
	if !strings.Contains(v, "/rcqp2?") || strings.Contains(v, "y/n") || !strings.Contains(v, "[1] Runs") {
		t.Errorf("keys acted while typing a filter:\n%s", v)
	}
}

// runOnce runs cmd, and each command it batches, feeding the messages to m
// without following the commands that come back. A watch's wait returns
// another wait, which runAll would follow forever.
func runOnce(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m, _ = step(m, c())
		}
		return m
	}
	m, _ = step(m, msg)
	return m
}

// feedJobs sends the newest job watch an empty job list, as the watcher's
// first poll would, so commands waiting on it return.
func feedJobs(f *fakeWatch) {
	f.runChans[len(f.runChans)-1] <- watch.RunEvent{Jobs: []runs.Job{}}
}

func TestACyclesThroughAttempts(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	rerun := openRun
	rerun.RunAttempt = 2
	m, _ = step(m, runsview.OpenRunMsg{Repo: "o/r", Run: rerun})

	m, cmd := step(m, keyMsg("a"))
	feedJobs(f)
	m = runOnce(m, cmd)
	if f.runCalls[len(f.runCalls)-1] != "o/r/7#1" {
		t.Errorf("run watch calls %v, want attempt 1 watched", f.runCalls)
	}
	if l := strings.Split(view(m), "\n")[2]; !strings.Contains(l, "x failed") || !strings.Contains(l, "attempt 1 of 2") {
		t.Errorf("state line %q, want attempt 1's own state", l)
	}
	if f.runCtxs[0].Err() == nil {
		t.Error("latest attempt's job watch not stopped")
	}

	m, cmd = step(m, keyMsg("a"))
	feedJobs(f)
	m = runOnce(m, cmd)
	if f.runCalls[len(f.runCalls)-1] != "o/r/7" {
		t.Errorf("run watch calls %v, want back to the latest", f.runCalls)
	}
	if l := strings.Split(view(m), "\n")[2]; !strings.Contains(l, "* running") || !strings.Contains(l, "attempt 2 of 2") {
		t.Errorf("state line %q, want the latest attempt again", l)
	}
}

func TestAOnSingleAttemptRunExplains(t *testing.T) {
	f := &fakeWatch{}
	m, _ := onRunScreen(t, f)

	m, _ = step(m, keyMsg("a"))
	if v := view(m); !strings.Contains(v, "#7 has only one attempt") {
		t.Errorf("view:\n%s", v)
	}
	if len(f.runCalls) != 1 {
		t.Errorf("run watch calls %v", f.runCalls)
	}
}

func TestRepoUpdatesLeaveAnEarlierAttemptsHeader(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	rerun := openRun
	rerun.RunAttempt = 2
	m, _ = step(m, runsview.OpenRunMsg{Repo: "o/r", Run: rerun})
	m, cmd := step(m, keyMsg("a"))
	feedJobs(f)
	m = runOnce(m, cmd)

	done := rerun
	done.Status, done.Conclusion = "completed", "success"
	f.chans[0] <- watch.Event{Repo: "o/r", Runs: []runs.Run{done}, Changes: []runs.Change{{Prev: &rerun, Run: done}}}
	m, _ = step(m, m.wait()())
	if l := strings.Split(view(m), "\n")[2]; !strings.Contains(l, "x failed") {
		t.Errorf("state line %q, want attempt 1's state kept", l)
	}
}

var (
	passedRun  = runs.Run{ID: 15, RunNumber: 15, Name: "Manifest check", Status: "completed", Conclusion: "success"}
	runningRun = runs.Run{ID: 17, RunNumber: 17, Name: "Build", Status: "in_progress"}
)

// onRunsTabWith returns a model on the Runs tab of o/r showing rs, with the
// runs pane focused.
func onRunsTabWith(t *testing.T, f *fakeWatch, rs ...runs.Run) Model {
	t.Helper()
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	f.chans[0] <- watch.Event{Repo: "o/r", Initial: true, Runs: rs}
	m, _ = step(m, m.wait()())
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyTab})
	return m
}

func spaceKey() tea.Msg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }

func TestDeleteRunUnderCursor(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTabWith(t, f, failedRun, passedRun)

	m, _ = step(m, keyMsg("d"))
	if v := view(m); !strings.Contains(v, "delete #16 Manifest check? y/n") {
		t.Fatalf("no prompt:\n%s", v)
	}
	m, cmd := step(m, keyMsg("y"))
	m = runAll(m, cmd)
	if !slices.Equal(f.deletedRuns, []string{"o/r/16"}) {
		t.Errorf("deleted %v", f.deletedRuns)
	}
	v := view(m)
	if strings.Contains(v, "x #16") || !strings.Contains(v, "deleted #16") {
		t.Errorf("run not removed or no confirmation:\n%s", v)
	}
}

func TestDeleteMarkedRunsSkippingUnfinished(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTabWith(t, f, runningRun, failedRun, passedRun)

	for _, k := range []tea.Msg{spaceKey(), keyMsg("j"), spaceKey(), keyMsg("j"), spaceKey()} {
		m, _ = step(m, k)
	}
	m, _ = step(m, keyMsg("d"))
	if v := view(m); !strings.Contains(v, "delete 2 runs from o/r, skipping 1 unfinished? y/n") {
		t.Fatalf("no prompt:\n%s", v)
	}
	m, cmd := step(m, keyMsg("y"))
	m = runAll(m, cmd)
	if !slices.Equal(f.deletedRuns, []string{"o/r/16", "o/r/15"}) {
		t.Errorf("deleted %v", f.deletedRuns)
	}
	if v := view(m); strings.Contains(v, "marked") || !strings.Contains(v, "deleted 2 runs") || !strings.Contains(v, "#17") {
		t.Errorf("view:\n%s", v)
	}
}

func TestDeleteOnlyUnfinishedExplains(t *testing.T) {
	f := &fakeWatch{}
	m := onRunsTabWith(t, f, runningRun)

	m, _ = step(m, keyMsg("d"))
	if v := view(m); strings.Contains(v, "y/n") || !strings.Contains(v, "cannot delete #17, it has not finished") {
		t.Errorf("view:\n%s", v)
	}
}

func TestPartialDeleteReportsFailuresAndRemovesTheRest(t *testing.T) {
	f := &fakeWatch{deleteErr: map[int64]error{15: &actions.StatusError{StatusCode: 403, Message: "Must have admin rights"}}}
	m := onRunsTabWith(t, f, failedRun, passedRun)

	m, _ = step(m, spaceKey())
	m, _ = step(m, keyMsg("j"))
	m, _ = step(m, spaceKey())
	m, _ = step(m, keyMsg("d"))
	m, cmd := step(m, keyMsg("y"))
	m = runAll(m, cmd)
	v := view(m)
	if !strings.Contains(v, "deleted 1 of 2 runs, #15: 403 Must have admin rights") {
		t.Errorf("no partial failure message:\n%s", v)
	}
	if strings.Contains(v, "#16 ") || !strings.Contains(v, "#15") {
		t.Errorf("want #16 removed and #15 kept:\n%s", v)
	}
}

func TestRunningJobLogWaitsForTheJobToFinish(t *testing.T) {
	f := &fakeWatch{logBody: []joblog.Line{{Text: "hello from the runner"}}}
	m, wait := onRunScreen(t, f)
	running := logJob
	running.Status, running.Conclusion = "in_progress", ""

	m, _ = step(m, runview.OpenLogMsg{Repo: "o/r", Job: running})
	if len(f.logCalls) != 0 {
		t.Errorf("fetched the log of a running job: %v", f.logCalls)
	}
	if v := view(m); !strings.Contains(v, "log appears when the job finishes") {
		t.Errorf("log screen:\n%s", v)
	}

	f.runChans[0] <- watch.RunEvent{Jobs: []runs.Job{logJob}}
	m, cmd := step(m, wait())
	// A second poll with the job unchanged lets the batched wait return, and
	// must not fetch again.
	f.runChans[0] <- watch.RunEvent{Jobs: []runs.Job{logJob}}
	m = runOnce(m, cmd)
	if len(f.logCalls) != 1 {
		t.Errorf("log fetched %d times after the job finished, want 1", len(f.logCalls))
	}
	if v := view(m); !strings.Contains(v, "hello from the runner") || !strings.Contains(v, "x failed") {
		t.Errorf("log screen after the job finished:\n%s", v)
	}
}

func TestLogNotUploadedYetIsRetried(t *testing.T) {
	notFound := &joblog.StatusError{StatusCode: 404}
	f := &fakeWatch{logBody: []joblog.Line{{Text: "hello from the runner"}}, logErrs: []error{notFound}}
	m, fetch := onLogScreen(t, f)

	m, retry := step(m, fetch())
	if v := view(m); !strings.Contains(v, "waiting for the log to upload") {
		t.Errorf("log screen after a 404:\n%s", v)
	}
	if retry == nil {
		t.Fatal("no retry after a 404")
	}
	m, refetch := step(m, logRetryMsg{gen: m.logGen})
	m, _ = step(m, refetch())
	if v := view(m); len(f.logCalls) != 2 || !strings.Contains(v, "hello from the runner") {
		t.Errorf("fetches %v, log screen:\n%s", f.logCalls, v)
	}
}

func TestLogRetriesGiveUp(t *testing.T) {
	notFound := &joblog.StatusError{StatusCode: 404}
	f := &fakeWatch{logErrs: []error{notFound, notFound, notFound, notFound, notFound, notFound, notFound}}
	m, fetch := onLogScreen(t, f)

	m, retry := step(m, fetch())
	for retry != nil && len(f.logCalls) < 10 {
		var refetch tea.Cmd
		m, refetch = step(m, logRetryMsg{gen: m.logGen})
		m, retry = step(m, refetch())
	}
	if len(f.logCalls) > 6 {
		t.Errorf("fetched %d times, want at most 6", len(f.logCalls))
	}
	if v := view(m); !strings.Contains(v, "404 Not Found") {
		t.Errorf("log screen after giving up:\n%s", v)
	}
}

func TestLogScreenTicksWhileTheJobRuns(t *testing.T) {
	f := &fakeWatch{}
	m, wait := onRunScreen(t, f)
	running := logJob
	running.Status, running.Conclusion = "in_progress", ""

	m, tick := step(m, runview.OpenLogMsg{Repo: "o/r", Job: running})
	if tick == nil {
		t.Fatal("no tick for a running job's log")
	}
	m, tick = step(m, logTickMsg{gen: m.logGen})
	if tick == nil {
		t.Error("tick stopped while the job still runs")
	}

	f.runChans[0] <- watch.RunEvent{Jobs: []runs.Job{logJob}}
	m, _ = step(m, wait())
	if _, tick = step(m, logTickMsg{gen: m.logGen}); tick != nil {
		t.Error("tick kept going after the job finished")
	}
}

// eventCmds steps a repo watch event and returns every message its command
// produces, feeding the watch another event so the batched wait returns.
func eventCmds(t *testing.T, f *fakeWatch, m Model, ev watch.Event) []tea.Msg {
	t.Helper()
	m, cmd := step(m, eventMsg{gen: m.gen, ev: ev, ok: true})
	f.chans[0] <- watch.Event{Repo: "o/r"}
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c != nil {
				out = append(out, c())
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func rang(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if raw, ok := msg.(tea.RawMsg); ok && raw.Msg == "\a" {
			return true
		}
	}
	return false
}

func TestBellRingsWhenAWatchedRunFinishes(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	done := openRun
	done.Status, done.Conclusion = "completed", "success"

	msgs := eventCmds(t, f, m, watch.Event{Repo: "o/r", Runs: []runs.Run{done}, Changes: []runs.Change{{Prev: &openRun, Run: done}}})
	if !rang(msgs) {
		t.Errorf("no bell, got %v", msgs)
	}
}

func TestNoBellForRunsThatFinishedBeforeLaunch(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))
	done := openRun
	done.Status, done.Conclusion = "completed", "success"

	msgs := eventCmds(t, f, m, watch.Event{Repo: "o/r", Initial: true, Runs: []runs.Run{done}, Changes: []runs.Change{{Prev: &openRun, Run: done}}})
	if rang(msgs) {
		t.Error("bell on the first poll")
	}
}

func TestNoBellWhenARunStarts(t *testing.T) {
	f := &fakeWatch{}
	m := sized(New(f.deps(), newPicker(), []string{"o/r"}))

	msgs := eventCmds(t, f, m, watch.Event{Repo: "o/r", Runs: []runs.Run{openRun}, Changes: []runs.Change{{Run: openRun}}})
	if rang(msgs) {
		t.Error("bell for a run that only started")
	}
}
