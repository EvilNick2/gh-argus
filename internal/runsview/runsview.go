// Package runsview is the Runs tab: watched repos down the left with a badge
// each, and the runs of the highlighted repo on the right.
package runsview

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EvilNick2/gh-argus/internal/badge"
	"github.com/EvilNick2/gh-argus/internal/runs"
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

// Current returns the run under the runs cursor, which actions apply to.
func (m Model) Current() (string, runs.Run, bool) {
	st := m.current()
	if len(st.runs) == 0 {
		return "", runs.Run{}, false
	}
	return m.repos[m.repoIdx], st.runs[m.runIdx], true
}

// rows is how many run rows fit below the pane's header and error lines.
func (m Model) rows() int {
	return max(1, m.height-2)
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
	if ev.Runs != nil {
		st.runs, st.seen = ev.Runs, true
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
		}
	}
	m.clamp()
}

var (
	dimStyle    = lipgloss.NewStyle().Faint(true)
	boldStyle   = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Reverse(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func repoBadge(st *repoState) string {
	switch {
	case st.err != nil:
		return "err"
	case runs.Active(st.runs):
		return "run"
	case len(st.runs) > 0:
		return badge.Word(st.runs[0].Status, st.runs[0].Conclusion)
	}
	return ""
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	if n <= 2 {
		return string(r[:n])
	}
	return string(r[:n-2]) + ".."
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
}

func shortName(full string) string {
	_, name, _ := strings.Cut(full, "/")
	return name
}

func (m Model) View() string {
	nameW := 8
	for _, r := range m.repos {
		nameW = max(nameW, len([]rune(shortName(r))))
	}
	nameW = min(nameW, 24)
	sideW := 5 + nameW

	var side []string
	for i, r := range m.repos {
		name := pad(truncate(shortName(r), nameW), nameW)
		if i == m.repoIdx {
			if m.focusRuns {
				name = boldStyle.Render(name)
			} else {
				name = cursorStyle.Render(name)
			}
		}
		side = append(side, badge.Render(repoBadge(m.state[r]))+" "+name)
	}

	paneW := max(10, m.width-sideW-3)
	var pane []string
	if len(m.repos) > 0 {
		pane = append(pane, boldStyle.Render(m.repos[m.repoIdx]))
	}
	st := m.current()
	if st.err != nil {
		pane = append(pane, errStyle.Render(truncate(st.err.Error(), paneW)))
	} else {
		pane = append(pane, "")
	}
	switch {
	case !st.seen && st.err == nil:
		pane = append(pane, dimStyle.Render("waiting for first poll"))
	case st.seen && len(st.runs) == 0:
		pane = append(pane, dimStyle.Render("no runs"))
	}
	end := min(len(st.runs), m.runOffset+m.rows())
	for i := m.runOffset; i < end; i++ {
		r := st.runs[i]
		tail := "  " + r.HeadBranch + "  " + fmt.Sprintf("%3s", age(m.now().Sub(r.CreatedAt)))
		title := fmt.Sprintf("#%d %s", r.RunNumber, r.Name)
		title = pad(truncate(title, paneW-5-len([]rune(tail))), paneW-5-len([]rune(tail)))
		if m.focusRuns && i == m.runIdx {
			title = cursorStyle.Render(title)
		}
		pane = append(pane, badge.Render(badge.Word(r.Status, r.Conclusion))+" "+title+dimStyle.Render(tail))
	}

	var b strings.Builder
	for i := range max(m.height, len(side), len(pane)) {
		if i >= m.height && m.height > 0 {
			break
		}
		l, r := "", ""
		if i < len(side) {
			l = side[i]
		}
		if i < len(pane) {
			r = pane[i]
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(pad(l, sideW) + dimStyle.Render(" | ") + r)
	}
	return b.String()
}
