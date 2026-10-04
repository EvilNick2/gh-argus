// Package workflowsview is the Workflows tab: each watched repo's workflows
// in one list grouped by repo.
package workflowsview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EvilNick2/gh-argus/internal/workflows"
)

// LoadedMsg delivers one repo's workflows, or the error fetching them.
type LoadedMsg struct {
	Repo      string
	Workflows []workflows.Workflow
	Err       error
}

type repoState struct {
	wfs    []workflows.Workflow
	err    error
	loaded bool
}

// item is a selectable workflow row.
type item struct {
	repo string
	idx  int
}

type Model struct {
	repos  []string
	state  map[string]*repoState
	cursor int
	// moved is set once the user moves the cursor. Before that it stays on
	// the first workflow as repos load in whatever order they answer.
	moved  bool
	width  int
	height int
}

func New(repos []string) Model {
	m := Model{repos: repos, state: map[string]*repoState{}}
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
		for i := range m.state[r].wfs {
			out = append(out, item{r, i})
		}
	}
	return out
}

// Current returns the workflow under the cursor.
func (m Model) Current() (string, workflows.Workflow, bool) {
	items := m.items()
	if len(items) == 0 {
		return "", workflows.Workflow{}, false
	}
	it := items[min(m.cursor, len(items)-1)]
	return it.repo, m.state[it.repo].wfs[it.idx], true
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		st, ok := m.state[msg.Repo]
		if !ok {
			break
		}
		repo, cur, had := m.Current()
		st.err = msg.Err
		if msg.Err == nil {
			st.wfs, st.loaded = msg.Workflows, true
		}
		// Keep the cursor on the same workflow when lists load or change.
		if had && m.moved {
			for i, it := range m.items() {
				if it.repo == repo && m.state[it.repo].wfs[it.idx].ID == cur.ID {
					m.cursor = i
				}
			}
		}
	case tea.KeyPressMsg:
		m.moved = true
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
	onStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cursorStyle = lipgloss.NewStyle().Reverse(true)
)

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
}

func (m Model) row(w workflows.Workflow, nameW int, selected bool) string {
	state := onStyle.Render("on ")
	switch {
	case w.State == "deleted":
		state = dimStyle.Render("del")
	case !w.Enabled():
		state = offStyle.Render("off")
	}
	name := pad(w.Name, nameW)
	switch {
	case selected:
		name = cursorStyle.Render(name)
	case w.Dynamic():
		name = dimStyle.Render(name)
	}
	tail := w.Path
	if w.Dynamic() {
		tail = "dynamic, cannot dispatch"
	}
	if !w.Enabled() && w.State != "deleted" {
		tail += "  " + strings.ReplaceAll(w.State, "_", " ")
	}
	return "  " + state + " " + name + "  " + dimStyle.Render(tail)
}

func (m Model) View() string {
	nameW := 10
	for _, r := range m.repos {
		for _, w := range m.state[r].wfs {
			nameW = max(nameW, lipgloss.Width(w.Name))
		}
	}
	nameW = min(nameW, 40)

	var rows []string
	cursorRow, i := 0, 0
	for _, r := range m.repos {
		st := m.state[r]
		header := boldStyle.Render(r)
		if st.err != nil {
			header += "  " + errStyle.Render(st.err.Error())
		}
		rows = append(rows, header)
		switch {
		case !st.loaded && st.err == nil:
			rows = append(rows, dimStyle.Render("  loading"))
		case st.loaded && len(st.wfs) == 0:
			rows = append(rows, dimStyle.Render("  no workflows"))
		}
		for _, w := range st.wfs {
			if i == m.cursor {
				cursorRow = len(rows)
			}
			rows = append(rows, m.row(w, nameW, i == m.cursor))
			i++
		}
	}

	h := max(1, m.height)
	offset := 0
	if cursorRow >= h {
		offset = cursorRow - h + 1
	}
	// On the last workflow, show the end of the list, where repos without
	// workflows have nothing to select.
	if i > 0 && m.cursor == i-1 {
		offset = min(cursorRow, max(offset, len(rows)-h))
	}
	rows = rows[offset:min(len(rows), offset+h)]
	return strings.Join(rows, "\n")
}
