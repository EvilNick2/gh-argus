// Package picker is the repo picker screen: a multi-select list of repos with
// fuzzy filtering, owner filtering and CI badges fetched for rows on screen.
package picker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/EvilNick2/gh-argus/internal/repos"
)

// StatusFunc returns a command that fetches statuses for names and answers
// with a StatusMsg.
type StatusFunc func(names []string) tea.Cmd

// StatusMsg delivers fetched CI statuses. Err is shown but not fatal.
type StatusMsg struct {
	Statuses map[string]repos.Status
	Err      error
}

// ReposMsg replaces the repo list, typically after a background refresh.
// A nil Repos keeps the current list. Err is shown either way.
type ReposMsg struct {
	Repos []repos.Repo
	Err   error
}

const statusBatch = 20

// Lines drawn around the rows: title and info above, help below.
const chromeLines = 3

type Model struct {
	repos    []repos.Repo
	owners   []string
	statuses map[string]repos.Status
	asked    map[string]bool
	selected map[string]bool
	fetch    StatusFunc
	init     tea.Cmd

	visible   []int
	cursor    int
	offset    int
	filter    string
	filtering bool
	owner     int // 0 is all owners, otherwise owners[owner-1]

	width, height int
	err           error
	loaded        bool // a ReposMsg has arrived
	done          bool
	cancelled     bool
}

func New(rs []repos.Repo, selected []string, fetch StatusFunc) Model {
	m := Model{
		statuses: map[string]repos.Status{},
		asked:    map[string]bool{},
		selected: map[string]bool{},
		fetch:    fetch,
	}
	for _, s := range selected {
		m.selected[s] = true
	}
	m.setRepos(rs)
	return m
}

func (m *Model) setRepos(rs []repos.Repo) {
	m.repos = rs
	m.owners = nil
	seen := map[string]bool{}
	for _, r := range rs {
		if !seen[r.Owner] {
			seen[r.Owner] = true
			m.owners = append(m.owners, r.Owner)
		}
	}
	if m.owner > len(m.owners) {
		m.owner = 0
	}
	m.refilter()
}

// refilter recomputes the visible rows from the owner and text filters.
func (m *Model) refilter() {
	var idx []int
	for i, r := range m.repos {
		if m.owner == 0 || r.Owner == m.owners[m.owner-1] {
			idx = append(idx, i)
		}
	}
	if m.filter != "" {
		targets := make([]string, len(idx))
		for i, ri := range idx {
			targets[i] = m.repos[ri].FullName
		}
		var matched []int
		for _, match := range fuzzy.Find(m.filter, targets) {
			matched = append(matched, idx[match.Index])
		}
		idx = matched
	}
	m.visible = idx
	m.cursor = max(0, min(m.cursor, len(idx)-1))
	m.scroll()
}

func (m Model) rows() int {
	return max(1, m.height-chromeLines)
}

func (m *Model) scroll() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows() {
		m.offset = m.cursor - m.rows() + 1
	}
	m.offset = max(0, min(m.offset, len(m.visible)-m.rows()))
}

// enrich asks for statuses of on-screen repos that have not been asked for.
func (m *Model) enrich() tea.Cmd {
	if m.fetch == nil || m.height == 0 {
		return nil
	}
	var names []string
	for _, ri := range m.visible[m.offset:min(len(m.visible), m.offset+m.rows())] {
		name := m.repos[ri].FullName
		if !m.asked[name] {
			m.asked[name] = true
			names = append(names, name)
		}
	}
	var cmds []tea.Cmd
	for len(names) > 0 {
		n := min(statusBatch, len(names))
		cmds = append(cmds, m.fetch(names[:n]))
		names = names[n:]
	}
	return tea.Batch(cmds...)
}

// WithInit sets a command to run when the program starts, such as a
// background refresh of the repo list.
func (m Model) WithInit(cmd tea.Cmd) Model {
	m.init = cmd
	return m
}

func (m Model) Init() tea.Cmd {
	return m.init
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
	case StatusMsg:
		for k, v := range msg.Statuses {
			m.statuses[k] = v
		}
		m.err = msg.Err
	case ReposMsg:
		m.loaded = true
		if msg.Repos != nil {
			m.setRepos(msg.Repos)
		}
		m.err = msg.Err
	case tea.KeyPressMsg:
		if cmd := m.key(msg); cmd != nil {
			return m, cmd
		}
	}
	return m, m.enrich()
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		m.cancelled = true
		return tea.Quit
	}
	if m.filtering {
		switch k {
		case "enter":
			m.filtering = false
		case "esc":
			m.filtering, m.filter = false, ""
			m.refilter()
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
				m.refilter()
			}
		default:
			if msg.Text != "" {
				m.filter += msg.Text
				m.refilter()
			}
		}
		return nil
	}

	switch k {
	case "q", "esc":
		m.cancelled = true
		return tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(m.visible)-1, m.cursor+1)
	case "pgup":
		m.cursor = max(0, m.cursor-m.rows())
	case "pgdown":
		m.cursor = min(len(m.visible)-1, m.cursor+m.rows())
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.visible) - 1
	case "space":
		if name, ok := m.current(); ok {
			m.selected[name] = !m.selected[name]
		}
	case "/":
		m.filtering = true
	case "o":
		m.owner = (m.owner + 1) % (len(m.owners) + 1)
		m.refilter()
	case "enter":
		if len(m.Selected()) == 0 {
			name, ok := m.current()
			if !ok {
				return nil
			}
			m.selected[name] = true
		}
		m.done = true
		return tea.Quit
	}
	m.cursor = max(0, m.cursor)
	m.scroll()
	return nil
}

func (m Model) current() (string, bool) {
	if len(m.visible) == 0 {
		return "", false
	}
	return m.repos[m.visible[m.cursor]].FullName, true
}

// Done reports that the user confirmed a selection.
func (m Model) Done() bool {
	return m.done
}

// Cancelled reports that the user quit without confirming.
func (m Model) Cancelled() bool {
	return m.cancelled
}

// Selected returns the selected repos in list order. Previously selected repos
// missing from the list are left out.
func (m Model) Selected() []string {
	var out []string
	for _, r := range m.repos {
		if m.selected[r.FullName] {
			out = append(out, r.FullName)
		}
	}
	return out
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	passStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	runStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	privateLabel = dimStyle.Render("private")
)

func (m Model) badge(name string) string {
	s, ok := m.statuses[name]
	switch {
	case !ok && m.asked[name]:
		return dimStyle.Render("... ")
	case !ok:
		return "    "
	}
	switch s {
	case repos.StatusPassing:
		return passStyle.Render("pass")
	case repos.StatusFailing:
		return failStyle.Render("fail")
	case repos.StatusRunning:
		return runStyle.Render("run ")
	}
	return "    "
}

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render("gh argus") + dimStyle.Render("  pick repos to watch") + "\n")

	owner := "all"
	if m.owner > 0 {
		owner = m.owners[m.owner-1]
	}
	info := fmt.Sprintf("owner: %s  %d/%d repos  %d selected", owner, len(m.visible), len(m.repos), len(m.Selected()))
	if m.filtering || m.filter != "" {
		info = "/" + m.filter + "  " + info
	}
	if len(m.repos) == 0 && !m.loaded {
		info += "  loading repos"
	}
	if m.err != nil {
		info += "  " + errorStyle.Render(m.err.Error())
	}
	b.WriteString(dimStyle.Render(info) + "\n")

	end := min(len(m.visible), m.offset+m.rows())
	for i := m.offset; i < end; i++ {
		r := m.repos[m.visible[i]]
		box := "[ ]"
		if m.selected[r.FullName] {
			box = "[x]"
		}
		name := r.FullName
		if i == m.cursor {
			name = cursorStyle.Render(name)
		}
		line := box + " " + m.badge(r.FullName) + " " + name
		if r.Private {
			line += "  " + privateLabel
		}
		b.WriteString(line + "\n")
	}
	for i := end - m.offset; i < m.rows(); i++ {
		b.WriteString("\n")
	}

	help := "space select  enter watch  / filter  o owner  q quit"
	if m.filtering {
		help = "type to filter  enter done  esc clear"
	}
	b.WriteString(dimStyle.Render(help))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}
