// Package runnersview is the Runners tab: each watched repo's self-hosted
// runners. Listing them needs admin on the repo, so a 403 or 404 is the
// normal case for a collaborator and is shown as "not permitted", not as an
// error.
package runnersview

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/grouped"
	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/runners"
	"github.com/EvilNick2/gh-argus/internal/theme"
)

// LoadedMsg delivers one repo's runners, or the error fetching them.
type LoadedMsg struct {
	Repo    string
	Runners []runners.Runner
	Err     error
}

type Model struct {
	list grouped.Model[runners.Runner]
}

func New(repos []string) Model {
	return Model{list: grouped.New(repos, func(r runners.Runner) int64 { return r.ID })}
}

func (m Model) SetSize(w, h int) Model {
	m.list = m.list.SetSize(w, h)
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		m.list = m.list.Load(msg.Repo, msg.Runners, msg.Err)
	case tea.KeyPressMsg:
		m.list = m.list.Key(msg.String())
	}
	return m, nil
}

func notPermitted(err error) string {
	var se *fetch.StatusError
	if errors.As(err, &se) && (se.StatusCode == 403 || se.StatusCode == 404) {
		return theme.Muted().Render("not permitted, listing runners needs admin on this repo")
	}
	return theme.Fail().Render(err.Error())
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

func row(r runners.Runner, nameW int) string {
	state := theme.Pass().Render("online ")
	switch {
	case r.Status != "online":
		state = theme.Muted().Render(pad(r.Status, 7))
	case r.Busy:
		state = theme.Gold().Render("busy   ")
	}
	return "   " + state + " " + theme.Text().Render(pad(r.Name, nameW)) + "  " + pad(r.OS, 8) +
		theme.Muted().Render(strings.Join(r.LabelNames(), ", "))
}

func (m Model) View() string {
	nameW := 10
	for _, r := range m.list.All() {
		nameW = max(nameW, ansi.StringWidth(r.Name))
	}
	nameW = min(nameW, 40)
	return m.list.Render(grouped.View[runners.Runner]{
		Title: "self-hosted runners",
		Row:   func(r runners.Runner, _ bool) string { return row(r, nameW) },
		Empty: "no self-hosted runners",
		Error: notPermitted,
	})
}

// Mouse selects the row under a click and moves with the wheel.
func (m Model) Mouse(ev mouse.Event) Model {
	m.list, _ = m.list.Mouse(ev)
	return m
}
