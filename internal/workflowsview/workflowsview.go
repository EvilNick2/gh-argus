// Package workflowsview is the Workflows tab: each watched repo's workflows
// in one list grouped by repo.
package workflowsview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EvilNick2/gh-argus/internal/grouped"
	"github.com/EvilNick2/gh-argus/internal/workflows"
)

// LoadedMsg delivers one repo's workflows, or the error fetching them.
type LoadedMsg struct {
	Repo      string
	Workflows []workflows.Workflow
	Err       error
}

type Model struct {
	list grouped.Model[workflows.Workflow]
}

func New(repos []string) Model {
	return Model{list: grouped.New(repos, func(w workflows.Workflow) int64 { return w.ID })}
}

func (m Model) SetSize(w, h int) Model {
	m.list = m.list.SetSize(w, h)
	return m
}

// Current returns the workflow under the cursor.
func (m Model) Current() (string, workflows.Workflow, bool) {
	return m.list.Current()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		m.list = m.list.Load(msg.Repo, msg.Workflows, msg.Err)
	case tea.KeyPressMsg:
		m.list = m.list.Key(msg.String())
	}
	return m, nil
}

var (
	dimStyle    = lipgloss.NewStyle().Faint(true)
	onStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cursorStyle = lipgloss.NewStyle().Reverse(true)
)

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
}

func row(w workflows.Workflow, nameW int, selected bool) string {
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
	for _, w := range m.list.All() {
		nameW = max(nameW, lipgloss.Width(w.Name))
	}
	nameW = min(nameW, 40)
	return m.list.Render(grouped.View[workflows.Workflow]{
		Row:   func(w workflows.Workflow, selected bool) string { return row(w, nameW, selected) },
		Empty: "no workflows",
	})
}
