// Package runview is the run screen: one run's jobs, with the steps of the
// job under the cursor expanded.
package runview

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

// BackMsg asks the app to close the run screen.
type BackMsg struct{}

// OpenLogMsg asks the app to open the log of a job.
type OpenLogMsg struct {
	Repo string
	Job  runs.Job
}

type Model struct {
	repo   string
	run    runs.Run
	jobs   []runs.Job
	loaded bool
	err    error
	cursor int
	now    func() time.Time

	width, height int
}

func New(repo string, run runs.Run, now func() time.Time) Model {
	return Model{repo: repo, run: run, now: now}
}

func (m Model) SetSize(w, h int) Model {
	m.width, m.height = w, h
	return m
}

// SetRun replaces the run shown in the header, for when the watcher sees it
// change.
func (m Model) SetRun(r runs.Run) Model {
	m.run = r
	return m
}

// Run identifies the run on screen.
func (m Model) Run() (string, runs.Run) {
	return m.repo, m.run
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watch.RunEvent:
		m.err = msg.Err
		if msg.Err == nil {
			m.setJobs(msg.Jobs)
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "backspace":
			return m, func() tea.Msg { return BackMsg{} }
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = max(0, min(len(m.jobs)-1, m.cursor+1))
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = max(0, len(m.jobs)-1)
		case "enter":
			if len(m.jobs) == 0 {
				return m, nil
			}
			open := OpenLogMsg{Repo: m.repo, Job: m.jobs[m.cursor]}
			return m, func() tea.Msg { return open }
		}
	}
	return m, nil
}

// setJobs replaces the jobs, keeping the cursor on the same job.
func (m *Model) setJobs(jobs []runs.Job) {
	var keep int64 = -1
	if m.cursor < len(m.jobs) {
		keep = m.jobs[m.cursor].ID
	}
	m.jobs, m.loaded = jobs, true
	for i, j := range jobs {
		if j.ID == keep {
			m.cursor = i
		}
	}
	m.cursor = max(0, min(m.cursor, len(jobs)-1))
}

// elapsed is how long something ran, or has been running, or "" if it has
// not started.
func (m Model) elapsed(start, end time.Time) string {
	if start.IsZero() {
		return ""
	}
	if end.IsZero() {
		end = m.now()
	}
	return timefmt.Duration(end.Sub(start))
}

func (m Model) View() string {
	r := m.run
	inner := max(10, m.width-2)
	state := " " + theme.Icon(r.Status, r.Conclusion) + " " + theme.Text().Render(theme.StateWord(r.Status, r.Conclusion)) +
		"  " + theme.Accent().UnsetBold().Render(r.HeadBranch)
	if r.RunAttempt > 1 {
		state += theme.Muted().Render(fmt.Sprintf("  attempt %d", r.RunAttempt))
	}
	if m.err != nil {
		state += "  " + theme.Fail().Render(m.err.Error())
	}

	nameW := 10
	for _, j := range m.jobs {
		nameW = max(nameW, ansi.StringWidth(j.Name))
	}
	if len(m.jobs) > 0 {
		for _, s := range m.jobs[m.cursor].Steps {
			nameW = max(nameW, ansi.StringWidth(s.Name)+5)
		}
	}
	nameW = min(nameW, max(10, inner-16))

	var body []string
	cursorLine, steps := 0, 0
	if !m.loaded {
		body = append(body, theme.Muted().Render(" loading jobs"))
	} else if len(m.jobs) == 0 {
		body = append(body, theme.Muted().Render(" no jobs yet"))
	}
	for i, j := range m.jobs {
		row := " " + theme.Icon(j.Status, j.Conclusion) + " " + theme.Bold().Render(pad(j.Name, nameW)) + "  " +
			theme.Muted().Render(m.elapsed(j.StartedAt, j.CompletedAt))
		if i == m.cursor {
			row, cursorLine = theme.Selected(row, inner), len(body)
		}
		body = append(body, row)
		if i != m.cursor {
			continue
		}
		steps = len(j.Steps)
		for _, s := range j.Steps {
			label := pad(fmt.Sprintf("%2d %s", s.Number, s.Name), nameW-2)
			body = append(body, "     "+theme.Icon(s.Status, s.Conclusion)+" "+theme.Text().Render(label)+"  "+
				theme.Muted().Render(m.elapsed(s.StartedAt, s.CompletedAt)))
		}
	}

	// Scroll so the cursor job is visible, with as many of its steps as fit
	// below the state line and a blank line.
	rows := max(1, m.height-4)
	offset := 0
	if last := cursorLine + steps; last >= rows {
		offset = min(cursorLine, last-rows+1)
	}
	body = body[offset:min(len(body), offset+rows)]

	title := fmt.Sprintf("%s  #%d %s", m.repo, r.RunNumber, r.Name)
	return theme.Pane(title, strings.Join(append([]string{state, ""}, body...), "\n"), m.width, m.height, true)
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}
