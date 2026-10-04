package runsview

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

var dotfilesRuns = []runs.Run{
	{ID: 16, RunNumber: 16, Name: "Manifest check", HeadBranch: "main", Status: "in_progress", CreatedAt: ago(12 * time.Second)},
	{ID: 15, RunNumber: 15, Name: "Manifest check", HeadBranch: "main", Status: "completed", Conclusion: "success", CreatedAt: ago(time.Hour)},
}

var orpheusRuns = []runs.Run{
	{ID: 41, RunNumber: 41, Name: "Release", HeadBranch: "main", Status: "completed", Conclusion: "failure", CreatedAt: ago(3 * time.Minute)},
}

func newModel(t *testing.T) Model {
	t.Helper()
	m := New([]string{"EvilNick2/dotfiles", "EvilNick2/orpheus"}, func() time.Time { return now })
	m = m.SetSize(100, 20)
	m, _ = m.Update(watch.Event{Repo: "EvilNick2/dotfiles", Initial: true, Runs: dotfilesRuns})
	m, _ = m.Update(watch.Event{Repo: "EvilNick2/orpheus", Initial: true, Runs: orpheusRuns})
	return m
}

func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func key(s string) tea.Msg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

// line returns the first line of the view, styling stripped, containing substr.
func line(t *testing.T, view, substr string) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substr, view)
	return ""
}

func TestSidebarBadgesFromRuns(t *testing.T) {
	v := newModel(t).View()

	// dotfiles has a run in progress, orpheus's latest run failed.
	if l := line(t, v, "orpheus"); !strings.Contains(l, "fail") {
		t.Errorf("orpheus sidebar line %q, want fail badge", l)
	}
	if l := line(t, v, "dotfiles "); !strings.Contains(l, "run") {
		t.Errorf("dotfiles sidebar line %q, want run badge", l)
	}
}

func TestRunsPaneShowsHighlightedRepo(t *testing.T) {
	m := newModel(t)

	v := m.View()
	if !strings.Contains(v, "#16 Manifest check") || strings.Contains(v, "#41 Release") {
		t.Errorf("first repo's runs not shown alone:\n%s", v)
	}
	m, _ = send(m, key("j"))
	v = m.View()
	if !strings.Contains(v, "#41 Release") || strings.Contains(v, "#16 Manifest check") {
		t.Errorf("j in sidebar did not switch repo:\n%s", v)
	}
}

func TestRunRowsShowStateBranchAndAge(t *testing.T) {
	v := newModel(t).View()

	l := line(t, v, "#15 Manifest check")
	for _, want := range []string{"pass", "main", "1h"} {
		if !strings.Contains(l, want) {
			t.Errorf("run row %q missing %q", l, want)
		}
	}
	if l := line(t, v, "#16 Manifest check"); !strings.Contains(l, "12s") || !strings.Contains(l, "run") {
		t.Errorf("in-progress run row %q", l)
	}
}

func TestTabThenEnterOpensRunUnderCursor(t *testing.T) {
	m := newModel(t)

	m, cmd := send(m, key("tab"), key("j"), key("enter"))
	if cmd == nil {
		t.Fatal("enter on a run returned no command")
	}
	msg, ok := cmd().(OpenRunMsg)
	if !ok || msg.Repo != "EvilNick2/dotfiles" || msg.Run.ID != 15 {
		t.Errorf("got %#v, want OpenRunMsg for dotfiles run 15", msg)
	}
}

func TestEnterInSidebarMovesFocusToRuns(t *testing.T) {
	m := newModel(t)

	m, cmd := send(m, key("enter"), key("j"), key("enter"))
	if cmd == nil {
		t.Fatal("no command")
	}
	if msg, ok := cmd().(OpenRunMsg); !ok || msg.Run.ID != 15 {
		t.Errorf("got %#v, want run 15 opened after enter focused the runs pane", msg)
	}
}

func TestChangeEventUpdatesRuns(t *testing.T) {
	m := newModel(t)
	done := []runs.Run{
		{ID: 16, RunNumber: 16, Name: "Manifest check", HeadBranch: "main", Status: "completed", Conclusion: "failure", CreatedAt: ago(12 * time.Second)},
		dotfilesRuns[1],
	}

	m, _ = send(m, watch.Event{Repo: "EvilNick2/dotfiles", Runs: done, Changes: []runs.Change{{Prev: &dotfilesRuns[0], Run: done[0]}}})
	v := m.View()
	if l := line(t, v, "#16 Manifest check"); !strings.Contains(l, "fail") {
		t.Errorf("updated run row %q, want fail", l)
	}
	if l := line(t, v, "dotfiles "); !strings.Contains(l, "fail") {
		t.Errorf("sidebar line %q, want fail after the active run failed", l)
	}
}

func TestErrorEventShowsOnRepo(t *testing.T) {
	m := newModel(t)

	m, _ = send(m, watch.Event{Repo: "EvilNick2/orpheus", Err: errors.New("GET /repos/EvilNick2/orpheus/actions/runs: 502")}, key("j"))
	v := m.View()
	if l := line(t, v, "orpheus "); !strings.Contains(l, "err") {
		t.Errorf("sidebar line %q, want err badge", l)
	}
	if !strings.Contains(v, "502") {
		t.Errorf("error not shown in runs pane:\n%s", v)
	}
}

func TestRepoWithNoEventYetShowsWaiting(t *testing.T) {
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(100, 20)

	if v := m.View(); !strings.Contains(v, "waiting for first poll") {
		t.Errorf("view:\n%s", v)
	}
}

func TestAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{59 * time.Second, "59s"},
		{90 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := age(c.d); got != c.want {
			t.Errorf("age(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCursorStaysOnRunWhenNewRunArrives(t *testing.T) {
	m := newModel(t)
	m, _ = send(m, key("tab"), key("j")) // on run 15

	newer := runs.Run{ID: 17, RunNumber: 17, Name: "Manifest check", Status: "queued", CreatedAt: now}
	m, _ = send(m, watch.Event{Repo: "EvilNick2/dotfiles", Runs: append([]runs.Run{newer}, dotfilesRuns...)})
	_, cmd := send(m, key("enter"))
	if msg, ok := cmd().(OpenRunMsg); !ok || msg.Run.ID != 15 {
		t.Errorf("opened %#v, want run 15 still under the cursor", msg)
	}
}

func TestRunsListScrollsToKeepCursorVisible(t *testing.T) {
	var many []runs.Run
	for i := range 30 {
		many = append(many, runs.Run{ID: int64(100 - i), RunNumber: 100 - i, Name: "ci", Status: "completed", Conclusion: "success", CreatedAt: now})
	}
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(100, 8)
	m, _ = send(m, watch.Event{Repo: "o/r", Initial: true, Runs: many}, key("tab"))
	for range 20 {
		m, _ = send(m, key("j"))
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "#80 ci") || strings.Contains(v, "#100 ci") {
		t.Errorf("cursor on #80 not scrolled into view:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n > 8 {
		t.Errorf("view is %d lines, taller than height 8", n)
	}
}

func TestCurrentIsRunUnderCursor(t *testing.T) {
	m := newModel(t)

	m, _ = send(m, key("tab"), key("j"))
	repo, r, ok := m.Current()
	if !ok || repo != "EvilNick2/dotfiles" || r.ID != 15 {
		t.Errorf("Current() = %q, %d, %v, want dotfiles run 15", repo, r.ID, ok)
	}
	empty := New([]string{"o/r"}, func() time.Time { return now })
	if _, _, ok := empty.Current(); ok {
		t.Error("Current() ok with no runs")
	}
}
