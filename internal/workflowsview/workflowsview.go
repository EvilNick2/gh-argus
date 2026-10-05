// Package workflowsview is the Workflows tab: each watched repo's workflows
// in one list grouped by repo.
package workflowsview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/grouped"
	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/theme"
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

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

func row(w workflows.Workflow, nameW int) string {
	state := theme.Pass().Render("on ")
	switch {
	case w.State == "deleted":
		state = theme.Muted().Render("del")
	case !w.Enabled():
		state = theme.Gold().Render("off")
	}
	name := theme.Text().Render(pad(w.Name, nameW))
	if w.Dynamic() {
		name = theme.Muted().Render(pad(w.Name, nameW))
	}
	tail := w.Path
	if w.Dynamic() {
		tail = "dynamic, cannot dispatch"
	}
	if !w.Enabled() && w.State != "deleted" {
		tail += "  " + strings.ReplaceAll(w.State, "_", " ")
	}
	return "   " + state + " " + name + "  " + theme.Muted().Render(tail)
}

func (m Model) View() string {
	nameW := 10
	for _, w := range m.list.All() {
		nameW = max(nameW, ansi.StringWidth(w.Name))
	}
	nameW = min(nameW, 40)
	return m.list.Render(grouped.View[workflows.Workflow]{
		Title: "workflows",
		Row:   func(w workflows.Workflow, _ bool) string { return row(w, nameW) },
		Empty: "no workflows",
	})
}

// Mouse selects the workflow under a click and moves with the wheel. It
// reports a double click on a workflow as activated, which dispatches it.
func (m Model) Mouse(ev mouse.Event) (Model, bool) {
	var activated bool
	m.list, activated = m.list.Mouse(ev)
	return m, activated
}
