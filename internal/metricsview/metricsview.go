// Package metricsview is the Metrics tab: CI health per watched repo and per
// workflow, derived from each repo's recent runs.
package metricsview

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EvilNick2/gh-argus/internal/metrics"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/timefmt"
)

// LoadedMsg delivers one repo's recent runs, or the error fetching them.
type LoadedMsg struct {
	Repo string
	Runs []runs.Run
	Err  error
}

type repoState struct {
	total  metrics.Stat
	per    []metrics.Stat
	err    error
	loaded bool
}

// item is a selectable row: a repo total (wf -1) or one of its workflows.
type item struct {
	repo string
	wf   int
}

type Model struct {
	repos  []string
	state  map[string]*repoState
	now    func() time.Time
	cursor int
	width  int
	height int
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
	return m
}

func (m Model) items() []item {
	var out []item
	for _, r := range m.repos {
		st := m.state[r]
		if !st.loaded || st.total.Runs == 0 {
			continue
		}
		out = append(out, item{r, -1})
		for i := range st.per {
			out = append(out, item{r, i})
		}
	}
	return out
}

func (m Model) stat(it item) metrics.Stat {
	st := m.state[it.repo]
	if it.wf < 0 {
		return st.total
	}
	return st.per[it.wf]
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		st, ok := m.state[msg.Repo]
		if !ok {
			break
		}
		st.err = msg.Err
		if msg.Err == nil {
			st.total, st.per = metrics.Compute(msg.Runs)
			st.loaded = true
		}
	case tea.KeyPressMsg:
		n := len(m.items())
		switch msg.String() {
		case "down", "j":
			m.cursor++
		case "up", "k":
			m.cursor--
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = n
		}
		m.cursor = max(0, min(m.cursor, n-1))
	}
	return m, nil
}

var (
	boldStyle   = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	passStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	cursorStyle = lipgloss.NewStyle().Reverse(true)
)

// glyphs renders recent results so they read without colour: + passed,
// x failed, . anything else.
func glyphs(history []string) string {
	var b strings.Builder
	for _, w := range history {
		switch w {
		case "pass":
			b.WriteString(passStyle.Render("+"))
		case "fail":
			b.WriteString(failStyle.Render("x"))
		default:
			b.WriteString(dimStyle.Render("."))
		}
	}
	return b.String()
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:max(0, n-2)]) + ".."
}

func columns(s metrics.Stat) string {
	rate := "-"
	if r, ok := s.SuccessRate(); ok {
		rate = fmt.Sprintf("%.0f%%", r*100)
	}
	trend := "-"
	if !math.IsNaN(s.Trend) {
		trend = fmt.Sprintf("%+.0f%%", s.Trend*100)
	}
	return fmt.Sprintf("%5d %8s %7s %7s %7d  ", s.Runs, rate, timefmt.Duration(s.Median), trend, s.Reruns)
}

func (m Model) View() string {
	nameW := 12
	for _, r := range m.repos {
		nameW = max(nameW, len(r))
		for _, s := range m.state[r].per {
			nameW = max(nameW, len([]rune(s.Name))+2)
		}
	}
	nameW = min(nameW, 40)

	header := dimStyle.Render(pad("", nameW) + fmt.Sprintf("%5s %8s %7s %7s %7s  %s", "runs", "success", "median", "trend", "reruns", "recent"))

	items := m.items()
	var rows []string
	cursorRow, idx := 0, 0
	for _, r := range m.repos {
		st := m.state[r]
		name := pad(truncate(r, nameW), nameW)
		switch {
		case st.err != nil:
			rows = append(rows, boldStyle.Render(name)+errStyle.Render(st.err.Error()))
			continue
		case !st.loaded:
			rows = append(rows, boldStyle.Render(name)+dimStyle.Render("loading"))
			continue
		case st.total.Runs == 0:
			rows = append(rows, boldStyle.Render(name)+dimStyle.Render("no completed runs"))
			continue
		}
		for i := -1; i < len(st.per); i++ {
			s, label := st.total, name
			if i >= 0 {
				s, label = st.per[i], pad(truncate("  "+st.per[i].Name, nameW), nameW)
			}
			switch {
			case idx == m.cursor:
				label, cursorRow = cursorStyle.Render(label), len(rows)
			case i < 0:
				label = boldStyle.Render(label)
			}
			rows = append(rows, label+columns(s)+glyphs(s.History))
			idx++
		}
	}

	body := max(1, m.height-2)
	offset := 0
	if cursorRow >= body {
		offset = cursorRow - body + 1
	}
	rows = rows[offset:min(len(rows), offset+body)]
	for len(rows) < body {
		rows = append(rows, "")
	}

	detail := ""
	if len(items) > 0 {
		it := items[min(m.cursor, len(items)-1)]
		s := m.stat(it)
		name := it.repo
		if it.wf >= 0 {
			name = s.Name
		}
		detail = dimStyle.Render(fmt.Sprintf("%s: last run %s ago, queue median %s, %d cancelled",
			name, timefmt.Age(m.now().Sub(s.Last)), timefmt.Duration(s.QueueMedian), s.Cancelled))
	}
	return strings.Join(append(append([]string{header}, rows...), detail), "\n")
}
