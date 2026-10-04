// Package app is the top-level Bubble Tea model: the repo picker, then the
// tabbed console fed by the watcher.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EvilNick2/gh-argus/internal/actions"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/logview"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/runsview"
	"github.com/EvilNick2/gh-argus/internal/runview"
	"github.com/EvilNick2/gh-argus/internal/watch"
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
	Now  func() time.Time
}

type screen int

const (
	screenPicker screen = iota
	screenTabs
	screenRun
	screenLog
)

var tabs = []struct {
	name      string
	milestone int
}{
	{"Runs", 3},
	{"Workflows", 4},
	{"Metrics", 5},
	{"Cache", 6},
	{"Runners", 6},
}

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

// pendingAction is a run action waiting for y/n.
type pendingAction struct {
	repo string
	run  runs.Run
	kind actions.Kind
}

// actionDoneMsg reports the result of a run action.
type actionDoneMsg struct {
	pendingAction
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
	tab    int

	gen    int
	events <-chan watch.Event
	cancel context.CancelFunc

	run       runview.Model
	runGen    int
	runEvents <-chan watch.RunEvent
	runCancel context.CancelFunc

	log    logview.Model
	logGen int
	logJob runs.Job
	logRep string

	confirm  *pendingAction
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
	m.screen, m.tab = screenTabs, 0
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

// Lines around the tab body: tab bar and a blank line above, help below.
const chromeLines = 3

func (m Model) bodyHeight() int {
	return max(1, m.height-chromeLines)
}

// runHeight leaves the run screen one line for help.
func (m Model) runHeight() int {
	return max(1, m.height-1)
}

// Init always runs the picker's init, its repo list refresh, so the list is
// current if the picker is opened later with p.
func (m Model) Init() tea.Cmd {
	if m.screen == screenTabs {
		return tea.Batch(m.wait(), m.picker.Init())
	}
	return m.picker.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.runs = m.runs.SetSize(m.width, m.bodyHeight())
		m.run = m.run.SetSize(m.width, m.runHeight())
		m.log = m.log.SetSize(m.width, m.runHeight())
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
			m.flash, m.flashErr = fmt.Sprintf("%v failed: %v", msg.kind, msg.err), true
		} else {
			m.flash = fmt.Sprintf("%v requested for #%d", msg.kind, msg.run.RunNumber)
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
		if kind, ok := actionKeys[msg.String()]; ok && (m.screen == screenRun || m.screen == screenTabs && m.tab == 0) {
			return m.ask(kind), nil
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
		m.confirm = &pendingAction{repo: repo, run: r, kind: kind}
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
	act := m.deps.Act
	return m, func() tea.Msg {
		err := act(context.Background(), p.repo, p.run.ID, p.kind)
		return actionDoneMsg{pendingAction: p, err: err}
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
		return m, nil
	}
	if m.tab == 0 {
		var cmd tea.Cmd
		m.runs, cmd = m.runs.Update(msg)
		return m, cmd
	}
	return m, nil
}

var (
	activeTab = lipgloss.NewStyle().Bold(true).Reverse(true)
	dimStyle  = lipgloss.NewStyle().Faint(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func (m Model) View() tea.View {
	switch m.screen {
	case screenPicker:
		return m.picker.View()
	case screenRun:
		return screenView(m.run.View(), m.runHeight(), m.footer("j/k job  enter log  r/R rerun  c cancel  esc back  q quit"))
	case screenLog:
		return screenView(m.log.View(), m.runHeight(), m.footer("j/k scroll  / search  n/N match  w wrap  r reload  esc back  q quit"))
	}

	var bar []string
	for i, t := range tabs {
		label := fmt.Sprintf(" %d %s ", i+1, t.name)
		if i == m.tab {
			label = activeTab.Render(label)
		}
		bar = append(bar, label)
	}
	top := strings.Join(bar, " ")
	if m.err != nil {
		top += "  " + errStyle.Render(m.err.Error())
	}

	body := m.runs.View()
	help := "tab pane  enter open  r/R rerun  c cancel  1-5 tabs  p repos  q quit"
	if m.tab != 0 {
		t := tabs[m.tab]
		body = dimStyle.Render(fmt.Sprintf("%s is not built yet, it lands in milestone %d.", t.name, t.milestone))
		help = "1-5 tabs  p repos  q quit"
	}
	return screenView(top+"\n\n"+body, m.height-1, m.footer(help))
}

// footer is the bottom line: a pending prompt, else a message, else help.
func (m Model) footer(help string) string {
	switch {
	case m.confirm != nil:
		c := m.confirm
		return fmt.Sprintf("%s %s? y/n", phrase(c.kind, c.run.RunNumber), c.run.Name)
	case m.flashErr:
		return errStyle.Render(m.flash)
	case m.flash != "":
		return m.flash
	}
	return dimStyle.Render(help)
}

// screenView pads content to height lines and puts the footer below.
func screenView(content string, height int, footer string) tea.View {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	v := tea.NewView(strings.Join(lines, "\n") + "\n" + footer)
	v.AltScreen = true
	return v
}
