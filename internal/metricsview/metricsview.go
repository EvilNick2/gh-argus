// Package metricsview is the Metrics tab: CI health per watched repo and per
// workflow, derived from each repo's recent runs.
package metricsview

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/metrics"
	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/theme"
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

// rowItems gives, for each row below the column header, the index of the
// selectable item on it, or -1 for a status row such as loading. View draws
// rows in the same order.
func (m Model) rowItems() []int {
	var out []int
	idx := 0
	for _, r := range m.repos {
		st := m.state[r]
		if st.err != nil || !st.loaded || st.total.Runs == 0 {
			out = append(out, -1)
			continue
		}
		for range len(st.per) + 1 {
			out = append(out, idx)
			idx++
		}
	}
	return out
}

// offset is the first row shown, keeping the cursor in the rows that fit
// between the column header and the detail line.
func (m Model) offset() int {
	body := max(1, m.height-4)
	for i, it := range m.rowItems() {
		if it == m.cursor && i >= body {
			return i - body + 1
		}
	}
	return 0
}

// Mouse selects the row under a click and moves with the wheel.
func (m Model) Mouse(ev mouse.Event) (Model, tea.Cmd) {
	n := len(m.items())
	if d := ev.Wheel(); d != 0 {
		m.cursor = max(0, min(n-1, m.cursor+d))
		return m, nil
	}
	// Rows start below the border and the column header.
	rows := m.rowItems()
	i := m.offset() + ev.Y - 2
	if ev.Clicked() && ev.Y >= 2 && ev.Y < m.height-2 && i < len(rows) && rows[i] >= 0 {
		m.cursor = rows[i]
	}
	return m, nil
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

// glyphs renders recent results so they read without colour: + passed,
// x failed, . anything else.
func glyphs(history []string) string {
	var b strings.Builder
	for _, w := range history {
		switch w {
		case "pass":
			b.WriteString(theme.Pass().Render("+"))
		case "fail":
			b.WriteString(theme.Fail().Render("x"))
		default:
			b.WriteString(theme.Muted().Render("."))
		}
	}
	return b.String()
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
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
	inner := max(10, m.width-2)
	nameW := 12
	for _, r := range m.repos {
		nameW = max(nameW, len(r))
		for _, s := range m.state[r].per {
			nameW = max(nameW, len([]rune(s.Name))+2)
		}
	}
	nameW = min(nameW, 40)

	header := theme.Muted().Render(" " + pad("", nameW) + fmt.Sprintf("%5s %8s %7s %7s %7s  %s", "runs", "success", "median", "trend", "reruns", "recent"))

	items := m.items()
	var rows []string
	idx := 0
	for _, r := range m.repos {
		st := m.state[r]
		name := pad(truncate(r, nameW), nameW)
		switch {
		case st.err != nil:
			rows = append(rows, " "+theme.Bold().Render(name)+theme.Fail().Render(st.err.Error()))
			continue
		case !st.loaded:
			rows = append(rows, " "+theme.Bold().Render(name)+theme.Muted().Render("loading"))
			continue
		case st.total.Runs == 0:
			rows = append(rows, " "+theme.Bold().Render(name)+theme.Muted().Render("no completed runs"))
			continue
		}
		for i := -1; i < len(st.per); i++ {
			s, label := st.total, theme.Bold().Render(name)
			if i >= 0 {
				s, label = st.per[i], theme.Text().Render(pad(truncate("  "+st.per[i].Name, nameW), nameW))
			}
			row := " " + label + theme.Text().Render(columns(s)) + glyphs(s.History)
			if idx == m.cursor {
				row = theme.Selected(row, inner)
			}
			rows = append(rows, row)
			idx++
		}
	}

	// The pane holds the column header, the rows and a detail line.
	body := max(1, m.height-4)
	offset := m.offset()
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
		detail = theme.Muted().Render(fmt.Sprintf(" %s: last run %s ago, queue median %s, %d cancelled",
			name, timefmt.Age(m.now().Sub(s.Last)), timefmt.Duration(s.QueueMedian), s.Cancelled))
	}
	content := append(append([]string{header}, rows...), detail)
	return theme.Pane("metrics, last 100 runs per repo", strings.Join(content, "\n"), m.width, m.height, true)
}
