// Package app is the top-level Bubble Tea model: the repo picker, then the
// tabbed console fed by the watcher.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/actions"
	"github.com/EvilNick2/gh-argus/internal/caches"
	"github.com/EvilNick2/gh-argus/internal/cachesview"
	"github.com/EvilNick2/gh-argus/internal/dispatchform"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/logview"
	"github.com/EvilNick2/gh-argus/internal/metricsview"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/runners"
	"github.com/EvilNick2/gh-argus/internal/runnersview"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/runsview"
	"github.com/EvilNick2/gh-argus/internal/runview"
	"github.com/EvilNick2/gh-argus/internal/theme"
	"github.com/EvilNick2/gh-argus/internal/watch"
	"github.com/EvilNick2/gh-argus/internal/workflows"
	"github.com/EvilNick2/gh-argus/internal/workflowsview"
)

// Deps are the side effects the app needs, passed in so tests can fake them.
type Deps struct {
	// Watch starts watching repos until ctx is done and returns the events.
	Watch func(ctx context.Context, repos []string) <-chan watch.Event
	// WatchRun polls the jobs of one run until ctx is done.
	WatchRun func(ctx context.Context, repo string, id int64) <-chan watch.RunEvent
	// FetchLog downloads and parses the log of a job.
	FetchLog func(ctx context.Context, repo string, id int64) ([]joblog.Line, error)
	// Act sends a run action such as a rerun or cancel.
	Act           func(ctx context.Context, repo string, id int64, k actions.Kind) error
	SaveSelection func(repos []string) error
	// Seed returns runs saved by an earlier session, or nil.
	Seed func(repo string) *watch.Seed
	// Remaining is the REST requests left this hour, or -1 before the first.
	Remaining func() int
	// ListWorkflows fetches the workflows of a repo.
	ListWorkflows func(ctx context.Context, repo string) ([]workflows.Workflow, error)
	// DispatchSpec returns the ref to dispatch on, normally the default
	// branch, and what the workflow file says about dispatch.
	DispatchSpec func(ctx context.Context, repo string, wf workflows.Workflow) (string, workflows.DispatchSpec, error)
	// RecentRuns fetches a repo's recent runs for the Metrics tab.
	RecentRuns func(ctx context.Context, repo string) ([]runs.Run, error)
	// ListRunners fetches a repo's self-hosted runners.
	ListRunners func(ctx context.Context, repo string) ([]runners.Runner, error)
	// ListCaches fetches a repo's Actions caches.
	ListCaches func(ctx context.Context, repo string) ([]caches.Cache, error)
	// DeleteCache deletes one cache.
	DeleteCache func(ctx context.Context, repo string, id int64) error
	// Branches lists a repo's branches for the dispatch form.
	Branches func(ctx context.Context, repo string) ([]string, error)
	// Dispatch triggers a workflow on ref with inputs.
	Dispatch func(ctx context.Context, repo string, id int64, ref string, inputs map[string]string) error
	// SetWorkflow enables or disables a workflow.
	SetWorkflow func(ctx context.Context, repo string, id int64, enabled bool) error
	Now         func() time.Time
}

type screen int

const (
	screenPicker screen = iota
	screenTabs
	screenRun
	screenLog
	screenForm
)

var tabs = []string{"Runs", "Workflows", "Metrics", "Cache", "Runners"}

// eventMsg carries one watcher event. gen identifies the watch it came from,
// so events from a watch that has since been replaced are dropped.
type eventMsg struct {
	gen int
	ev  watch.Event
	ok  bool
}

// runEventMsg carries one job event for the run screen, tagged like eventMsg.
type runEventMsg struct {
	gen int
	ev  watch.RunEvent
	ok  bool
}

// logMsg carries a fetched log, tagged so a slow fetch for a log that has
// since been closed is dropped.
type logMsg struct {
	gen   int
	lines []joblog.Line
	err   error
}

// wfLoadedMsg carries one repo's workflows, tagged with the watch generation
// so a load for a replaced selection is dropped.
type wfLoadedMsg struct {
	gen int
	workflowsview.LoadedMsg
}

// rnLoadedMsg and caLoadedMsg carry one repo's runners or caches, tagged
// like wfLoadedMsg.
type rnLoadedMsg struct {
	gen int
	runnersview.LoadedMsg
}

type caLoadedMsg struct {
	gen int
	cachesview.LoadedMsg
}

// metLoadedMsg carries one repo's recent runs for the Metrics tab, tagged
// like wfLoadedMsg.
type metLoadedMsg struct {
	gen int
	metricsview.LoadedMsg
}

// specMsg carries what a workflow file says about dispatch.
type specMsg struct {
	gen      int
	edit     bool // the user asked to edit inputs
	repo     string
	wf       workflows.Workflow
	ref      string
	branches []string
	spec     workflows.DispatchSpec
	err      error
}

// pending is a change to GitHub waiting for y/n.
type pending struct {
	prompt  string // asked as "<prompt>? y/n"
	success string // shown when it succeeds
	failure string // shown as "<failure> failed: <err>"
	do      func(context.Context) error
	after   tea.Cmd // runs once it succeeds, such as reloading the list
}

// actionDoneMsg reports the result of a pending change.
type actionDoneMsg struct {
	pending
	err error
}

var actionKeys = map[string]actions.Kind{
	"r": actions.RerunFailed,
	"R": actions.RerunAll,
	"c": actions.Cancel,
}

type Model struct {
	deps   Deps
	screen screen
	picker picker.Model
	runs   runsview.Model
	wfs    workflowsview.Model
	mets   metricsview.Model
	cchs   cachesview.Model
	rnrs   runnersview.Model
	repos  []string
	tab    int

	gen    int
	events <-chan watch.Event
	cancel context.CancelFunc

	run       runview.Model
	runGen    int
	runEvents <-chan watch.RunEvent
	runCancel context.CancelFunc

	log logview.Model

	form     dispatchform.Model
	formRepo string
	formWF   workflows.Workflow
	logGen   int
	logJob   runs.Job
	logRep   string

	confirm  *pending
	help     bool
	helpTop  int
	last     lastClick
	flash    string
	flashErr bool

	width, height int
	err           error
}

// New starts on the picker, or straight on the tabs watching repos when any
// are given.
func New(d Deps, p picker.Model, repos []string) Model {
	m := Model{deps: d, picker: p}
	if len(repos) > 0 {
		m.start(repos)
	}
	return m
}

func (m *Model) start(repos []string) {
	m.stop()
	ctx, cancel := context.WithCancel(context.Background())
	m.gen++
	m.cancel = cancel
	m.events = m.deps.Watch(ctx, repos)
	m.runs = runsview.New(repos, m.deps.Now).SetSize(m.width, m.bodyHeight())
	if m.deps.Seed != nil {
		for _, r := range repos {
			if seed := m.deps.Seed(r); seed != nil {
				m.runs = m.runs.Seed(r, seed.Runs)
			}
		}
	}
	m.wfs = workflowsview.New(repos).SetSize(m.width, m.bodyHeight())
	m.mets = metricsview.New(repos, m.deps.Now).SetSize(m.width, m.bodyHeight())
	m.cchs = cachesview.New(repos, m.deps.Now).SetSize(m.width, m.bodyHeight())
	m.rnrs = runnersview.New(repos).SetSize(m.width, m.bodyHeight())
	m.repos = repos
	m.screen, m.tab = screenTabs, 0
}

// loadRunners fetches each watched repo's self-hosted runners.
func (m Model) loadRunners() tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.repos {
		gen, list := m.gen, m.deps.ListRunners
		cmds = append(cmds, func() tea.Msg {
			rs, err := list(context.Background(), r)
			return rnLoadedMsg{gen: gen, LoadedMsg: runnersview.LoadedMsg{Repo: r, Runners: rs, Err: err}}
		})
	}
	return tea.Batch(cmds...)
}

// loadCaches fetches the caches of repos.
func (m Model) loadCaches(repos ...string) tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range repos {
		gen, list := m.gen, m.deps.ListCaches
		cmds = append(cmds, func() tea.Msg {
			cs, err := list(context.Background(), r)
			return caLoadedMsg{gen: gen, LoadedMsg: cachesview.LoadedMsg{Repo: r, Caches: cs, Err: err}}
		})
	}
	return tea.Batch(cmds...)
}

// loadMetrics fetches each watched repo's recent runs. The fetcher's ETags
// make reopening the tab free while nothing has changed.
func (m Model) loadMetrics() tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.repos {
		gen, recent := m.gen, m.deps.RecentRuns
		cmds = append(cmds, func() tea.Msg {
			rs, err := recent(context.Background(), r)
			return metLoadedMsg{gen: gen, LoadedMsg: metricsview.LoadedMsg{Repo: r, Runs: rs, Err: err}}
		})
	}
	return tea.Batch(cmds...)
}

// loadWorkflows fetches the workflows of repos. The fetcher's ETags make a
// reload of unchanged workflows free.
func (m Model) loadWorkflows(repos ...string) tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range repos {
		gen, list := m.gen, m.deps.ListWorkflows
		cmds = append(cmds, func() tea.Msg {
			wfs, err := list(context.Background(), r)
			return wfLoadedMsg{gen: gen, LoadedMsg: workflowsview.LoadedMsg{Repo: r, Workflows: wfs, Err: err}}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m *Model) openRun(repo string, r runs.Run) {
	m.stopRun()
	ctx, cancel := context.WithCancel(context.Background())
	m.runGen++
	m.runCancel = cancel
	m.runEvents = m.deps.WatchRun(ctx, repo, r.ID)
	m.run = runview.New(repo, r, m.deps.Now).SetSize(m.width, m.runHeight())
	m.screen = screenRun
}

func (m *Model) openLog(repo string, job runs.Job) tea.Cmd {
	m.logRep, m.logJob = repo, job
	m.log = logview.New(repo, job).SetSize(m.width, m.runHeight())
	m.screen = screenLog
	return m.fetchLog()
}

func (m *Model) fetchLog() tea.Cmd {
	m.logGen++
	gen, repo, id, fetch := m.logGen, m.logRep, m.logJob.ID, m.deps.FetchLog
	return func() tea.Msg {
		lines, err := fetch(context.Background(), repo, id)
		return logMsg{gen: gen, lines: lines, err: err}
	}
}

func (m *Model) stopRun() {
	if m.runCancel != nil {
		m.runCancel()
		m.runCancel = nil
	}
}

func (m *Model) quit() tea.Cmd {
	m.stopRun()
	m.stop()
	return tea.Quit
}

func (m Model) waitRun() tea.Cmd {
	gen, ch := m.runGen, m.runEvents
	return func() tea.Msg {
		ev, ok := <-ch
		return runEventMsg{gen: gen, ev: ev, ok: ok}
	}
}

func (m Model) wait() tea.Cmd {
	gen, ch := m.gen, m.events
	return func() tea.Msg {
		ev, ok := <-ch
		return eventMsg{gen: gen, ev: ev, ok: ok}
	}
}

// Lines around the tab body: header bar and tab line above, status bar below.
const chromeLines = 3

func (m Model) bodyHeight() int {
	return max(1, m.height-chromeLines)
}

// runHeight leaves the run, log and form screens the header and status bars.
func (m Model) runHeight() int {
	return max(1, m.height-2)
}

// Init always runs the picker's init, its repo list refresh, so the list is
// current if the picker is opened later with p.
func (m Model) Init() tea.Cmd {
	if m.screen == screenTabs {
		return tea.Batch(m.wait(), m.picker.Init(), tea.RequestBackgroundColor)
	}
	return tea.Batch(m.picker.Init(), tea.RequestBackgroundColor)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg, tea.MouseWheelMsg:
		return m.mouseMsg(msg)

	case tea.BackgroundColorMsg:
		theme.SetDark(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.runs = m.runs.SetSize(m.width, m.bodyHeight())
		m.wfs = m.wfs.SetSize(m.width, m.bodyHeight())
		m.mets = m.mets.SetSize(m.width, m.bodyHeight())
		m.cchs = m.cchs.SetSize(m.width, m.bodyHeight())
		m.rnrs = m.rnrs.SetSize(m.width, m.bodyHeight())
		m.run = m.run.SetSize(m.width, m.runHeight())
		m.log = m.log.SetSize(m.width, m.runHeight())
		m.form = m.form.SetSize(m.width, m.runHeight())
		return m.updatePicker(msg)

	case eventMsg:
		if msg.gen != m.gen || !msg.ok {
			return m, nil
		}
		m.runs, _ = m.runs.Update(msg.ev)
		if m.screen == screenRun || m.screen == screenLog {
			if repo, open := m.run.Run(); repo == msg.ev.Repo {
				for _, r := range msg.ev.Runs {
					if r.ID == open.ID {
						m.run = m.run.SetRun(r)
					}
				}
			}
		}
		return m, m.wait()

	case runEventMsg:
		if msg.gen != m.runGen || !msg.ok {
			return m, nil
		}
		m.run, _ = m.run.Update(msg.ev)
		return m, m.waitRun()

	case picker.ConfirmMsg:
		m.err = m.deps.SaveSelection(msg.Repos)
		m.start(msg.Repos)
		return m, m.wait()

	case runsview.OpenRunMsg:
		m.openRun(msg.Repo, msg.Run)
		return m, m.waitRun()

	case runview.BackMsg:
		m.stopRun()
		m.screen = screenTabs
		return m, nil

	case runview.OpenLogMsg:
		return m, m.openLog(msg.Repo, msg.Job)

	case logMsg:
		if msg.gen != m.logGen {
			return m, nil
		}
		m.log, _ = m.log.Update(logview.LogMsg{Lines: msg.lines, Err: msg.err})
		return m, nil

	case logview.ReloadMsg:
		return m, m.fetchLog()

	case logview.BackMsg:
		m.screen = screenRun
		return m, nil

	case actionDoneMsg:
		if msg.err != nil {
			m.flash, m.flashErr = fmt.Sprintf("%s failed: %v", msg.failure, msg.err), true
		} else {
			m.flash = msg.success
		}
		if msg.err == nil && msg.after != nil {
			return m, msg.after
		}
		return m, nil

	case dispatchform.SubmitMsg:
		m.screen = screenTabs
		dispatch, repo, wf, ref, inputs := m.deps.Dispatch, m.formRepo, m.formWF, msg.Ref, msg.Inputs
		p := pending{
			success: fmt.Sprintf("dispatched %s on %s", wf.Name, ref),
			failure: fmt.Sprintf("dispatch %s", wf.Name),
		}
		return m, func() tea.Msg {
			return actionDoneMsg{pending: p, err: dispatch(context.Background(), repo, wf.ID, ref, inputs)}
		}

	case dispatchform.CancelMsg:
		m.screen = screenTabs
		return m, nil

	case specMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		return m.offerDispatch(msg), nil

	case rnLoadedMsg:
		if msg.gen == m.gen {
			m.rnrs, _ = m.rnrs.Update(msg.LoadedMsg)
		}
		return m, nil

	case caLoadedMsg:
		if msg.gen == m.gen {
			m.cchs, _ = m.cchs.Update(msg.LoadedMsg)
		}
		return m, nil

	case metLoadedMsg:
		if msg.gen == m.gen {
			m.mets, _ = m.mets.Update(msg.LoadedMsg)
		}
		return m, nil

	case wfLoadedMsg:
		if msg.gen == m.gen {
			m.wfs, _ = m.wfs.Update(msg.LoadedMsg)
		}
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, m.quit()
		}
		m.flash, m.flashErr = "", false
		if m.confirm != nil {
			return m.answer(msg)
		}
		if m.help {
			switch msg.String() {
			case "q":
				return m, m.quit()
			case "?", "esc":
				m.help = false
			case "down", "j":
				m.scrollHelp(1)
			case "up", "k":
				m.scrollHelp(-1)
			case "home", "g":
				m.helpTop = 0
			case "end", "G":
				m.scrollHelp(1 << 30)
			}
			return m, nil
		}
		if msg.String() == "?" && m.helpAvailable() {
			m.help, m.helpTop = true, 0
			return m, nil
		}
		if kind, ok := actionKeys[msg.String()]; ok && (m.screen == screenRun || m.screen == screenTabs && m.tab == 0) {
			return m.ask(kind), nil
		}
		if msg.String() == "d" && m.screen == screenTabs && m.tab == 3 {
			return m.askDeleteCache(), nil
		}
		if k := msg.String(); (k == "e" || k == "d") && m.screen == screenTabs && m.tab == 1 {
			return m.askWorkflow(k == "e"), nil
		}
		if k := msg.String(); (k == "enter" || k == "i") && m.screen == screenTabs && m.tab == 1 {
			return m.checkDispatch(k == "i")
		}
		switch m.screen {
		case screenPicker:
			return m.updatePicker(msg)
		case screenRun:
			if msg.String() == "q" {
				return m, m.quit()
			}
			var cmd tea.Cmd
			m.run, cmd = m.run.Update(msg)
			return m, cmd
		case screenForm:
			var cmd tea.Cmd
			m.form, cmd = m.form.Update(msg)
			return m, cmd
		case screenLog:
			if msg.String() == "q" && !m.log.Searching() {
				return m, m.quit()
			}
			var cmd tea.Cmd
			m.log, cmd = m.log.Update(msg)
			return m, cmd
		}
		return m.key(msg)
	}
	// Picker messages such as status and repo refreshes keep arriving while
	// the tabs are shown, so the picker is current when reopened.
	return m.updatePicker(msg)
}

// ask starts the y/n prompt for kind on the run in focus, or explains why it
// does not apply.
func (m Model) ask(kind actions.Kind) Model {
	var (
		repo string
		r    runs.Run
		ok   = true
	)
	if m.screen == screenRun {
		repo, r = m.run.Run()
	} else {
		repo, r, ok = m.runs.Current()
	}
	switch {
	case !ok:
	case !kind.Allowed(r):
		reason := "it has not finished"
		switch {
		case kind == actions.Cancel:
			reason = "it has already completed"
		case r.Status == "completed":
			reason = "it succeeded"
		}
		m.flash = fmt.Sprintf("cannot %s, %s", phrase(kind, r.RunNumber), reason)
	default:
		act := m.deps.Act
		m.confirm = &pending{
			prompt:  phrase(kind, r.RunNumber) + " " + r.Name,
			success: fmt.Sprintf("%v requested for #%d", kind, r.RunNumber),
			failure: kind.String(),
			do:      func(ctx context.Context) error { return act(ctx, repo, r.ID, kind) },
		}
	}
	return m
}

// askWorkflow starts the y/n prompt to enable or disable the workflow under
// the cursor, or explains why it does not apply.
func (m Model) askWorkflow(enable bool) Model {
	repo, wf, ok := m.wfs.Current()
	verb, done := "disable", "disabled"
	if enable {
		verb, done = "enable", "enabled"
	}
	switch {
	case !ok:
	case wf.State == "deleted":
		m.flash = fmt.Sprintf("cannot %s %s, it was deleted", verb, wf.Name)
	case wf.Enabled() == enable:
		m.flash = fmt.Sprintf("%s is already %s", wf.Name, done)
	default:
		set := m.deps.SetWorkflow
		m.confirm = &pending{
			prompt:  fmt.Sprintf("%s workflow %s", verb, wf.Name),
			success: fmt.Sprintf("%s %s", done, wf.Name),
			failure: fmt.Sprintf("%s %s", verb, wf.Name),
			do:      func(ctx context.Context) error { return set(ctx, repo, wf.ID, enable) },
			after:   m.loadWorkflows(repo),
		}
	}
	return m
}

// askDeleteCache starts the y/n prompt to delete the cache under the cursor.
func (m Model) askDeleteCache() Model {
	repo, c, ok := m.cchs.Current()
	if !ok {
		return m
	}
	del := m.deps.DeleteCache
	m.confirm = &pending{
		prompt:  fmt.Sprintf("delete cache %s from %s", c.Key, repo),
		success: "deleted cache " + c.Key,
		failure: "delete cache " + c.Key,
		do:      func(ctx context.Context) error { return del(ctx, repo, c.ID) },
		after:   m.loadCaches(repo),
	}
	return m
}

// checkDispatch refuses dynamic and disabled workflows, otherwise fetches
// what the workflow file says about dispatch and the repo's branches. With
// edit set, the form opens even when every input has a default, so the
// branch can be chosen.
func (m Model) checkDispatch(edit bool) (tea.Model, tea.Cmd) {
	repo, wf, ok := m.wfs.Current()
	switch {
	case !ok:
		return m, nil
	case wf.Dynamic():
		m.flash = fmt.Sprintf("%s is dynamic and cannot be dispatched", wf.Name)
		return m, nil
	case !wf.Enabled():
		m.flash = fmt.Sprintf("%s is disabled, enable it first", wf.Name)
		return m, nil
	}
	m.flash = fmt.Sprintf("checking %s", wf.Name)
	gen, get, list := m.gen, m.deps.DispatchSpec, m.deps.Branches
	return m, func() tea.Msg {
		ref, spec, err := get(context.Background(), repo, wf)
		// Without the branch list the field still takes any typed ref.
		branches, _ := list(context.Background(), repo)
		return specMsg{gen: gen, edit: edit, repo: repo, wf: wf, ref: ref, branches: branches, spec: spec, err: err}
	}
}

// offerDispatch asks to dispatch once the workflow file has been read.
// Inputs with defaults are left out so GitHub applies the defaults.
func (m Model) offerDispatch(msg specMsg) Model {
	name := msg.wf.Name
	switch {
	case msg.err != nil:
		m.flash, m.flashErr = fmt.Sprintf("checking %s failed: %v", name, msg.err), true
	case !msg.spec.Dispatchable:
		m.flash = fmt.Sprintf("%s has no workflow_dispatch trigger", name)
	case msg.spec.NeedsInput() || msg.edit:
		m.flash = ""
		m.formRepo, m.formWF = msg.repo, msg.wf
		m.form = dispatchform.New(msg.repo, msg.wf, msg.ref, msg.branches, msg.spec.Inputs).SetSize(m.width, m.runHeight())
		m.screen = screenForm
	default:
		dispatch, repo, id, ref := m.deps.Dispatch, msg.repo, msg.wf.ID, msg.ref
		m.flash = ""
		m.confirm = &pending{
			prompt:  fmt.Sprintf("run %s on %s", name, ref),
			success: fmt.Sprintf("dispatched %s on %s", name, ref),
			failure: fmt.Sprintf("dispatch %s", name),
			do:      func(ctx context.Context) error { return dispatch(ctx, repo, id, ref, nil) },
		}
	}
	return m
}

// phrase is the action applied to run number n, such as "cancel #7" or
// "rerun failed jobs of #16".
func phrase(kind actions.Kind, n int) string {
	if kind == actions.Cancel {
		return fmt.Sprintf("%v #%d", kind, n)
	}
	return fmt.Sprintf("%v of #%d", kind, n)
}

// answer resolves the prompt. Only y sends the action, any other key declines.
func (m Model) answer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := *m.confirm
	m.confirm = nil
	if msg.String() != "y" {
		return m, nil
	}
	return m, func() tea.Msg {
		return actionDoneMsg{pending: p, err: p.do(context.Background())}
	}
}

func (m Model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.picker.Update(msg)
	m.picker = next.(picker.Model)
	return m, cmd
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "q":
		return m, m.quit()
	case "p":
		m.stop()
		m.screen = screenPicker
		return m, nil
	case "1", "2", "3", "4", "5":
		m.tab = int(k[0] - '1')
		switch m.tab {
		case 1:
			return m, m.loadWorkflows(m.repos...)
		case 2:
			return m, m.loadMetrics()
		case 3:
			return m, m.loadCaches(m.repos...)
		case 4:
			return m, m.loadRunners()
		}
		return m, nil
	}
	var cmd tea.Cmd
	switch m.tab {
	case 0:
		m.runs, cmd = m.runs.Update(msg)
	case 1:
		m.wfs, cmd = m.wfs.Update(msg)
	case 2:
		m.mets, cmd = m.mets.Update(msg)
	case 3:
		m.cchs, cmd = m.cchs.Update(msg)
	case 4:
		m.rnrs, cmd = m.rnrs.Update(msg)
	}
	return m, cmd
}

func (m Model) View() tea.View {
	switch m.screen {
	case screenPicker:
		return m.picker.View()
	case screenRun:
		return m.frame(m.run.View(), false, m.runHeight(), "j/k job  enter log  r/R rerun  c cancel  esc back  ? keys  q quit")
	case screenForm:
		return m.frame(m.form.View(), false, m.runHeight(), "tab next  left/right pick  space toggle  enter run  esc cancel")
	case screenLog:
		return m.frame(m.log.View(), false, m.runHeight(), "/ search  n/N match  w wrap  r reload  esc back  ? keys  q quit")
	}

	body := m.runs.View()
	help := "tab pane  enter open  r/R rerun  c cancel  p repos  ? keys  q quit"
	switch m.tab {
	case 1:
		body = m.wfs.View()
		help = "enter run  i branch/inputs  e/d enable/disable  p repos  ? keys  q quit"
	case 2:
		body = m.mets.View()
		help = "j/k move  p repos  ? keys  q quit"
	case 3:
		body = m.cchs.View()
		help = "d delete  p repos  ? keys  q quit"
	case 4:
		body = m.rnrs.View()
		help = "j/k move  p repos  ? keys  q quit"
	}
	return m.frame(body, true, m.bodyHeight(), help)
}

// scrollHelp moves the key reference by d rows within its length.
func (m *Model) scrollHelp(d int) {
	m.helpTop = max(0, min(m.helpTop+d, helpMaxOffset(m.width, m.helpHeight())))
}

// helpHeight is the pane height the key reference is drawn in.
func (m Model) helpHeight() int {
	if m.screen == screenTabs {
		return m.bodyHeight()
	}
	return m.runHeight()
}

// helpAvailable reports a screen where ? opens the key reference rather than
// being typed.
func (m Model) helpAvailable() bool {
	switch m.screen {
	case screenTabs, screenRun:
		return true
	case screenLog:
		return !m.log.Searching()
	}
	return false
}

// frame draws a screen: the header bar, the tab line on tab screens, body
// padded to height, and the status bar.
func (m Model) frame(body string, withTabs bool, height int, help string) tea.View {
	n := len(m.repos)
	watching := fmt.Sprintf("watching %d repos", n)
	if n == 1 {
		watching = "watching 1 repo"
	}
	lines := []string{theme.Bar(theme.Accent().Render(" argus")+theme.Muted().Render("  "+watching), m.requestsLeft(), m.width)}
	if withTabs {
		var labels []string
		for i := range tabs {
			label := tabLabel(i)
			if i == m.tab {
				label = theme.Accent().Render(label)
			} else {
				label = theme.Muted().Render(label)
			}
			labels = append(labels, label)
		}
		line := " " + strings.Join(labels, "   ")
		if m.err != nil {
			line += "   " + theme.Fail().Render(m.err.Error())
		}
		lines = append(lines, line)
	}
	if m.help {
		body = helpView(m.width, height, m.helpTop)
		help = "j/k scroll  ? or esc close  q quit"
	}
	content := strings.Split(body, "\n")
	for len(content) < height {
		content = append(content, "")
	}
	lines = append(lines, content[:height]...)
	lines = append(lines, m.statusBar(help, withTabs && m.tab == 0))

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// requestsLeft shows the REST requests left this hour, gold under 500 and
// red under 100, or nothing before the first response.
func (m Model) requestsLeft() string {
	if m.deps.Remaining == nil {
		return ""
	}
	n := m.deps.Remaining()
	if n < 0 {
		return ""
	}
	style := theme.Muted()
	switch {
	case n < 100:
		style = theme.Fail()
	case n < 500:
		style = theme.Gold()
	}
	return style.Render(fmt.Sprintf("%d requests left ", n))
}

// statusBar shows a pending prompt, else a message, on the left, and the key
// hints on the right, with the icon legend on the Runs tab.
func (m Model) statusBar(help string, legend bool) string {
	var left string
	switch {
	case m.confirm != nil:
		left = theme.Gold().Bold(true).Render(" " + m.confirm.prompt + "? y/n")
		help = "y confirm  any other key cancels"
		legend = false
	case m.flashErr:
		left = theme.Fail().Render(" " + m.flash)
	case m.flash != "":
		left = theme.Text().Render(" " + m.flash)
	}
	// The legend goes first when space runs out, then the hints are cut.
	right := theme.Muted().Render(help + " ")
	withLegend := theme.Legend() + theme.Muted().Render("   ") + right
	if legend && ansi.StringWidth(left)+ansi.StringWidth(withLegend) < m.width {
		right = withLegend
	}
	if room := m.width - ansi.StringWidth(left) - 1; ansi.StringWidth(right) > room {
		right = ansi.Truncate(right, max(0, room), "")
	}
	return theme.Bar(left, right, m.width)
}
