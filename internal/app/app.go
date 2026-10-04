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

	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/runsview"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

// Deps are the side effects the app needs, passed in so tests can fake them.
type Deps struct {
	// Watch starts watching repos until ctx is done and returns the events.
	Watch         func(ctx context.Context, repos []string) <-chan watch.Event
	SaveSelection func(repos []string) error
	Now           func() time.Time
}

type screen int

const (
	screenPicker screen = iota
	screenTabs
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

type Model struct {
	deps   Deps
	screen screen
	picker picker.Model
	runs   runsview.Model
	tab    int

	gen    int
	events <-chan watch.Event
	cancel context.CancelFunc

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
	m.screen, m.tab = screenTabs, 0
}

func (m *Model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
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
		return m.updatePicker(msg)

	case eventMsg:
		if msg.gen != m.gen || !msg.ok {
			return m, nil
		}
		m.runs, _ = m.runs.Update(msg.ev)
		return m, m.wait()

	case picker.ConfirmMsg:
		m.err = m.deps.SaveSelection(msg.Repos)
		m.start(msg.Repos)
		return m, m.wait()

	case runsview.OpenRunMsg:
		// The run screen arrives in the next piece.
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.stop()
			return m, tea.Quit
		}
		if m.screen == screenPicker {
			return m.updatePicker(msg)
		}
		return m.key(msg)
	}
	// Picker messages such as status and repo refreshes keep arriving while
	// the tabs are shown, so the picker is current when reopened.
	return m.updatePicker(msg)
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
		m.stop()
		return m, tea.Quit
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
	if m.screen == screenPicker {
		return m.picker.View()
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
	help := "tab pane  enter open  1-5 tabs  p repos  q quit"
	if m.tab != 0 {
		t := tabs[m.tab]
		body = dimStyle.Render(fmt.Sprintf("%s is not built yet, it lands in milestone %d.", t.name, t.milestone))
		help = "1-5 tabs  p repos  q quit"
	}
	lines := strings.Split(body, "\n")
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}

	v := tea.NewView(top + "\n\n" + strings.Join(lines, "\n") + "\n" + dimStyle.Render(help))
	v.AltScreen = true
	return v
}
