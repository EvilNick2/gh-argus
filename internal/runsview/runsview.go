// Package runsview is the Runs tab: watched repos down the left with a badge
// each, and the runs of the highlighted repo on the right.
package runsview

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/theme"
	"github.com/EvilNick2/gh-argus/internal/timefmt"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

// OpenRunMsg asks the app to open the run screen.
type OpenRunMsg struct {
	Repo string
	Run  runs.Run
}

type repoState struct {
	runs []runs.Run
	err  error
	seen bool
	// cached is set while runs come from a saved snapshot, before the first
	// poll of this session.
	cached bool
	// fresh holds runs that changed since the last session.
	fresh map[int64]bool
	// unseen marks a repo with fresh runs that has not been highlighted.
	unseen bool
}

type Model struct {
	repos []string
	state map[string]*repoState
	now   func() time.Time

	focusRuns bool
	repoIdx   int
	runIdx    int
	runOffset int

	width, height int
}

func New(repos []string, now func() time.Time) Model {
	m := Model{repos: repos, state: map[string]*repoState{}, now: now}
	for _, r := range repos {
		m.state[r] = &repoState{}
	}
	return m
}

func (m Model) SetSize(w, h int) Model {
	m.width, m.height = w, h
	m.clamp()
	return m
}

func (m Model) current() *repoState {
	if len(m.repos) == 0 {
		return &repoState{}
	}
	return m.state[m.repos[m.repoIdx]]
}

// Seed shows runs saved by an earlier session until the first poll.
func (m Model) Seed(repo string, rs []runs.Run) Model {
	if st, ok := m.state[repo]; ok {
		st.runs, st.seen, st.cached = rs, true, true
		m.clamp()
	}
	return m
}

// Current returns the run under the runs cursor, which actions apply to.
func (m Model) Current() (string, runs.Run, bool) {
	st := m.current()
	if len(st.runs) == 0 {
		return "", runs.Run{}, false
	}
	return m.repos[m.repoIdx], st.runs[m.runIdx], true
}

// rows is how many two-line runs fit in the runs pane, inside its border
// and below its status line.
func (m Model) rows() int {
	return max(1, (m.height-3)/2)
}

func (m *Model) clamp() {
	n := len(m.current().runs)
	m.runIdx = max(0, min(m.runIdx, n-1))
	if m.runIdx < m.runOffset {
		m.runOffset = m.runIdx
	}
	if m.runIdx >= m.runOffset+m.rows() {
		m.runOffset = m.runIdx - m.rows() + 1
	}
	m.runOffset = max(0, min(m.runOffset, n-m.rows()))
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watch.Event:
		m.apply(msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) apply(ev watch.Event) {
	st, ok := m.state[ev.Repo]
	if !ok {
		return
	}
	// Keep the cursor on the same run when new runs push it down.
	var keep int64 = -1
	if ev.Repo == m.repos[m.repoIdx] && m.runIdx < len(st.runs) {
		keep = st.runs[m.runIdx].ID
	}
	st.err = ev.Err
	if ev.Err == nil {
		st.cached = false
	}
	if ev.Runs != nil {
		st.runs, st.seen = ev.Runs, true
	}
	// Changes on the first poll happened while argus was closed.
	if ev.Initial && len(ev.Changes) > 0 {
		st.fresh = map[int64]bool{}
		for _, c := range ev.Changes {
			st.fresh[c.Run.ID] = true
		}
		st.unseen = ev.Repo != m.repos[m.repoIdx]
	}
	if keep >= 0 {
		for i, r := range st.runs {
			if r.ID == keep {
				m.runIdx = i
			}
		}
	}
	m.clamp()
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.focusRuns = !m.focusRuns
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "home", "g":
		m.move(-1 << 30)
	case "end", "G":
		m.move(1 << 30)
	case "enter":
		if !m.focusRuns {
			m.focusRuns = true
			break
		}
		st := m.current()
		if len(st.runs) == 0 {
			break
		}
		open := OpenRunMsg{Repo: m.repos[m.repoIdx], Run: st.runs[m.runIdx]}
		return m, func() tea.Msg { return open }
	}
	return m, nil
}

func (m *Model) move(d int) {
	if m.focusRuns {
		m.runIdx += d
	} else if len(m.repos) > 0 {
		next := max(0, min(m.repoIdx+d, len(m.repos)-1))
		if next != m.repoIdx {
			m.repoIdx, m.runIdx, m.runOffset = next, 0, 0
			m.state[m.repos[next]].unseen = false
		}
	}
	m.clamp()
}

// repoIcon is the sidebar icon: ? when the last poll failed, * while any
// run is active, otherwise the state of the latest run.
func repoIcon(st *repoState) string {
	switch {
	case st.err != nil:
		return theme.Fail().Render("?")
	case runs.Active(st.runs):
		return theme.Gold().Render("*")
	case len(st.runs) > 0:
		return theme.Icon(st.runs[0].Status, st.runs[0].Conclusion)
	}
	return " "
}

func truncate(s string, n int) string {
	return ansi.Truncate(s, max(0, n), "..")
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

func shortName(full string) string {
	_, name, _ := strings.Cut(full, "/")
	return name
}

// newTag marks something that changed since the last session.
func newTag() string {
	return theme.Gold().Render("new")
}

func (m Model) View() string {
	nameW := 8
	for _, r := range m.repos {
		nameW = max(nameW, ansi.StringWidth(shortName(r)))
	}
	nameW = min(nameW, 24)
	// " " icon " " name "  new "
	sideInner := nameW + 8
	sideW := sideInner + 2
	runsW := max(20, m.width-sideW)
	runsInner := runsW - 2

	var side []string
	for i, r := range m.repos {
		st := m.state[r]
		tag := "   "
		if st.unseen {
			tag = newTag()
		}
		row := " " + repoIcon(st) + " " + pad(truncate(shortName(r), nameW), nameW) + "  " + tag + " "
		if i == m.repoIdx {
			row = theme.Selected(row, sideInner)
		}
		side = append(side, row)
	}

	st := m.current()
	var body []string
	switch {
	case st.err != nil:
		body = append(body, " "+theme.Fail().Render(st.err.Error()))
	case st.cached:
		body = append(body, " "+theme.Muted().Render("cached, refreshing"))
	case !st.seen:
		body = append(body, " "+theme.Muted().Render("waiting for first poll"))
	case len(st.runs) == 0:
		body = append(body, " "+theme.Muted().Render("no runs"))
	default:
		body = append(body, " "+theme.Muted().Render(fmt.Sprintf("%d recent runs", len(st.runs))))
	}
	end := min(len(st.runs), m.runOffset+m.rows())
	for i := m.runOffset; i < end; i++ {
		r := st.runs[i]
		meta := "  " + r.Name
		if !r.CreatedAt.IsZero() {
			meta = "  " + timefmt.Age(m.now().Sub(r.CreatedAt)) + " ago" + meta
		}
		first := " " + theme.Icon(r.Status, r.Conclusion) + " " + theme.Bold().Render(fmt.Sprintf("#%d", r.RunNumber)) +
			" " + theme.Accent().UnsetBold().Render(r.HeadBranch) + theme.Muted().Render(meta)
		if r.RunAttempt > 1 {
			first += theme.Muted().Render(fmt.Sprintf("  attempt %d", r.RunAttempt))
		}
		if st.fresh[r.ID] {
			first += "  " + newTag()
		}
		title := r.DisplayTitle
		if title == "" {
			title = r.Name
		}
		second := "   " + theme.Text().Render(truncate(title, runsInner-4))
		if i == m.runIdx {
			first, second = theme.Selected(first, runsInner), theme.Selected(second, runsInner)
		}
		body = append(body, first, second)
	}

	title := ""
	if len(m.repos) > 0 {
		title = m.repos[m.repoIdx]
	}
	left := strings.Split(theme.Pane("repos", strings.Join(side, "\n"), sideW, m.height, !m.focusRuns), "\n")
	right := strings.Split(theme.Pane(title, strings.Join(body, "\n"), runsW, m.height, m.focusRuns), "\n")
	out := make([]string, len(left))
	for i := range left {
		out[i] = left[i] + right[i]
	}
	return strings.Join(out, "\n")
}
