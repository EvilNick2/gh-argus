// Package logview is the log screen for one job: scrolling, word wrap and
// search over the parsed log.
package logview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/badge"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/runs"
)

// LogMsg delivers a fetched log, or the error from fetching it.
type LogMsg struct {
	Lines []joblog.Line
	Err   error
}

// BackMsg asks the app to close the log view.
type BackMsg struct{}

// ReloadMsg asks the app to fetch the log again.
type ReloadMsg struct{}

// contextLines is how many lines are kept above an error or match jumped to.
const contextLines = 2

type row struct {
	line int
	text string
}

type Model struct {
	repo   string
	job    runs.Job
	lines  []joblog.Line
	loaded bool
	err    error

	rows     []row
	firstRow []int // first row index of each line
	top      int
	wrap     bool

	query     string
	searching bool
	matches   []int // line indices
	match     int

	width, height int
}

func New(repo string, job runs.Job) Model {
	return Model{repo: repo, job: job, wrap: true}
}

func (m Model) SetSize(w, h int) Model {
	line := m.topLine()
	m.width, m.height = w, h
	m.layout()
	m.showLine(line, 0)
	return m
}

// Searching reports that the search box has focus, so keys are text.
func (m Model) Searching() bool {
	return m.searching
}

func (m Model) bodyRows() int {
	return max(1, m.height-2)
}

func (m Model) topLine() int {
	if m.top < len(m.rows) {
		return m.rows[m.top].line
	}
	return 0
}

func (m *Model) clamp() {
	m.top = max(0, min(m.top, len(m.rows)-m.bodyRows()))
}

// showLine scrolls so line is visible with up to above lines of context.
func (m *Model) showLine(line, above int) {
	if line < len(m.firstRow) {
		m.top = m.firstRow[max(0, line-above)]
	}
	m.clamp()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LogMsg:
		m.err = msg.Err
		if msg.Err != nil {
			return m, nil
		}
		m.lines, m.loaded = msg.Lines, true
		m.findMatches()
		m.layout()
		m.top = len(m.rows)
		for i, l := range m.lines {
			if l.Kind == joblog.Error {
				m.showLine(i, contextLines)
				break
			}
		}
		m.clamp()
	case tea.KeyPressMsg:
		if m.searching {
			m.searchKey(msg)
			return m, nil
		}
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		return m, func() tea.Msg { return BackMsg{} }
	case "r":
		m.loaded, m.err = false, nil
		return m, func() tea.Msg { return ReloadMsg{} }
	case "down", "j":
		m.top++
	case "up", "k":
		m.top--
	case "pgdown", "space", "f":
		m.top += m.bodyRows()
	case "pgup", "b":
		m.top -= m.bodyRows()
	case "home", "g":
		m.top = 0
	case "end", "G":
		m.top = len(m.rows)
	case "w":
		line := m.topLine()
		m.wrap = !m.wrap
		m.layout()
		m.showLine(line, 0)
	case "/":
		m.searching, m.query = true, ""
	case "n":
		m.jump(1)
	case "N":
		m.jump(-1)
	}
	m.clamp()
	return m, nil
}

func (m *Model) searchKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.searching, m.query = false, ""
		m.findMatches()
		m.layout()
	case "enter":
		m.searching = false
		m.findMatches()
		m.layout()
		// Start from the first match at or below the top of the screen.
		top := m.topLine()
		m.match = 0
		for i, l := range m.matches {
			if l >= top {
				m.match = i
				break
			}
		}
		if len(m.matches) > 0 {
			m.showLine(m.matches[m.match], contextLines)
		}
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
	default:
		m.query += msg.Text
	}
}

func (m *Model) jump(d int) {
	if len(m.matches) == 0 {
		return
	}
	m.match = (m.match + d + len(m.matches)) % len(m.matches)
	m.showLine(m.matches[m.match], contextLines)
}

func (m *Model) findMatches() {
	m.matches, m.match = nil, 0
	if m.query == "" {
		return
	}
	q := strings.ToLower(m.query)
	for i, l := range m.lines {
		if strings.Contains(strings.ToLower(display(l)), q) {
			m.matches = append(m.matches, i)
		}
	}
}

var (
	boldStyle = lipgloss.NewStyle().Bold(true)
	dimStyle  = lipgloss.NewStyle().Faint(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	kindStyle = map[joblog.Kind]lipgloss.Style{
		joblog.Plain:   lipgloss.NewStyle(),
		joblog.Group:   lipgloss.NewStyle().Bold(true),
		joblog.Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		joblog.Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		joblog.Command: lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	}
)

// display is a line's text as shown, with the prefix GitHub's log UI uses.
func display(l joblog.Line) string {
	switch l.Kind {
	case joblog.Group:
		return "> " + l.Text
	case joblog.Error:
		return "Error: " + l.Text
	case joblog.Warning:
		return "Warning: " + l.Text
	}
	return l.Text
}

// styled renders a line in its kind's style with query matches reversed.
func styled(l joblog.Line, query string) string {
	s := display(l)
	st := kindStyle[l.Kind]
	lower := strings.ToLower(s)
	q := strings.ToLower(query)
	// Byte offsets from the lowered copy only hold when lowering kept lengths.
	if q == "" || len(lower) != len(s) {
		return st.Render(s)
	}
	var b strings.Builder
	for {
		i := strings.Index(lower, q)
		if i < 0 {
			b.WriteString(st.Render(s))
			return b.String()
		}
		b.WriteString(st.Render(s[:i]))
		b.WriteString(st.Reverse(true).Render(s[i : i+len(q)]))
		s, lower = s[i+len(q):], lower[i+len(q):]
	}
}

// layout turns lines into screen rows, wrapped or truncated to the width.
func (m *Model) layout() {
	m.rows = nil
	m.firstRow = make([]int, len(m.lines))
	w := max(1, m.width)
	for i, l := range m.lines {
		m.firstRow[i] = len(m.rows)
		s := styled(l, m.query)
		if !m.wrap {
			m.rows = append(m.rows, row{i, ansi.Truncate(s, w, "..")})
			continue
		}
		for _, part := range strings.Split(ansi.Hardwrap(s, w, true), "\n") {
			m.rows = append(m.rows, row{i, part})
		}
	}
}

func (m Model) View() string {
	j := m.job
	header := boldStyle.Render(m.repo) + "  " + boldStyle.Render(j.Name) + "  " +
		badge.Render(badge.Word(j.Status, j.Conclusion))

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render(m.err.Error())
	case !m.loaded:
		status = dimStyle.Render("loading log")
	default:
		end := min(len(m.rows), m.top+m.bodyRows())
		first, last := 0, 0
		if len(m.rows) > 0 {
			first, last = m.rows[m.top].line+1, m.rows[end-1].line+1
		}
		status = fmt.Sprintf("lines %d-%d of %d", first, last, len(m.lines))
		if !m.wrap {
			status += "  wrap off"
		}
		switch {
		case m.searching:
			status += "  /" + m.query
		case m.query != "" && len(m.matches) == 0:
			status += "  /" + m.query + " no matches"
		case m.query != "":
			status += fmt.Sprintf("  /%s %d/%d", m.query, m.match+1, len(m.matches))
		}
		status = dimStyle.Render(status)
	}

	out := []string{ansi.Truncate(header, m.width, ".."), ansi.Truncate(status, m.width, "..")}
	if m.loaded {
		for _, r := range m.rows[m.top:min(len(m.rows), m.top+m.bodyRows())] {
			out = append(out, r.text)
		}
	}
	return strings.Join(out, "\n")
}
