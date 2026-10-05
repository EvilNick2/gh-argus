package runsview

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/watch"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

var dotfilesRuns = []runs.Run{
	{ID: 16, RunNumber: 16, Name: "Manifest check", DisplayTitle: "feat: add wrapper", HeadBranch: "main", Status: "in_progress", CreatedAt: ago(12 * time.Second)},
	{ID: 15, RunNumber: 15, Name: "Manifest check", DisplayTitle: "docs: setup notes", HeadBranch: "main", Status: "completed", Conclusion: "success", CreatedAt: ago(time.Hour)},
}

var orpheusRuns = []runs.Run{
	{ID: 41, RunNumber: 41, Name: "Release", DisplayTitle: "chore: release 1.4", HeadBranch: "main", Status: "completed", Conclusion: "failure", CreatedAt: ago(3 * time.Minute)},
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

// panes splits a body line where the repos pane meets the runs pane. Border
// lines have no such junction and split into nothing.
func panes(l string) (side, runs string) {
	if i := strings.Index(l, "││"); i >= 0 {
		return l[:i+len("│")], l[i+len("│"):]
	}
	return "", ""
}

// sideRow is the repos pane part of the first line whose repos pane holds name.
func sideRow(t *testing.T, view, name string) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if side, _ := panes(l); strings.Contains(side, name) {
			return side
		}
	}
	t.Fatalf("no repos pane row with %q in:\n%s", name, view)
	return ""
}

// runRow is the runs pane part of the first line whose runs pane holds substr.
func runRow(t *testing.T, view, substr string) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if _, runs := panes(l); strings.Contains(runs, substr) {
			return runs
		}
	}
	t.Fatalf("no runs pane row with %q in:\n%s", substr, view)
	return ""
}

func TestSidebarBadgesFromRuns(t *testing.T) {
	v := newModel(t).View()

	// dotfiles has a run in progress, orpheus's latest run failed.
	if l := sideRow(t, v, "orpheus"); !strings.Contains(l, "x orpheus") {
		t.Errorf("orpheus sidebar row %q, want x", l)
	}
	if l := sideRow(t, v, "dotfiles"); !strings.Contains(l, "* dotfiles") {
		t.Errorf("dotfiles sidebar row %q, want *", l)
	}
}

func TestRunsPaneShowsHighlightedRepo(t *testing.T) {
	m := newModel(t)

	v := ansi.Strip(m.View())
	if !strings.Contains(v, "feat: add wrapper") || strings.Contains(v, "#41") {
		t.Errorf("first repo's runs not shown alone:\n%s", v)
	}
	m, _ = send(m, key("j"))
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "chore: release 1.4") || strings.Contains(v, "#16") {
		t.Errorf("j in sidebar did not switch repo:\n%s", v)
	}
}

func TestRunRowsShowStateBranchAndAge(t *testing.T) {
	v := newModel(t).View()

	l := runRow(t, v, "#15")
	for _, want := range []string{"+ #15", "main", "1h ago", "Manifest check"} {
		if !strings.Contains(l, want) {
			t.Errorf("run row %q missing %q", l, want)
		}
	}
	if l := runRow(t, v, "#16"); !strings.Contains(l, "* #16") || !strings.Contains(l, "12s ago") {
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
	if l := runRow(t, v, "#16"); !strings.Contains(l, "x #16") {
		t.Errorf("updated run row %q, want x", l)
	}
	if l := sideRow(t, v, "dotfiles"); !strings.Contains(l, "x dotfiles") {
		t.Errorf("sidebar row %q, want x after the active run failed", l)
	}
}

func TestErrorEventShowsOnRepo(t *testing.T) {
	m := newModel(t)

	m, _ = send(m, watch.Event{Repo: "EvilNick2/orpheus", Err: errors.New("GET /repos/EvilNick2/orpheus/actions/runs: 502")}, key("j"))
	v := m.View()
	if l := sideRow(t, v, "orpheus"); !strings.Contains(l, "? orpheus") {
		t.Errorf("sidebar row %q, want ?", l)
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
	if !strings.Contains(v, "#80 ") || strings.Contains(v, "#100 ") {
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

func TestSeedShowsCachedRunsUntilFirstPoll(t *testing.T) {
	m := New([]string{"EvilNick2/dotfiles"}, func() time.Time { return now }).SetSize(100, 20)

	m = m.Seed("EvilNick2/dotfiles", dotfilesRuns)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "feat: add wrapper") || !strings.Contains(v, "cached, refreshing") {
		t.Errorf("seeded view:\n%s", v)
	}
	m, _ = send(m, watch.Event{Repo: "EvilNick2/dotfiles", Initial: true, Runs: dotfilesRuns})
	if v := ansi.Strip(m.View()); strings.Contains(v, "cached") {
		t.Errorf("still marked cached after first poll:\n%s", v)
	}
}

func TestChangesSinceLastSessionAreMarked(t *testing.T) {
	m := New([]string{"EvilNick2/dotfiles"}, func() time.Time { return now }).SetSize(100, 20)
	m = m.Seed("EvilNick2/dotfiles", dotfilesRuns[1:])

	m, _ = send(m, watch.Event{Repo: "EvilNick2/dotfiles", Initial: true, Runs: dotfilesRuns,
		Changes: []runs.Change{{Run: dotfilesRuns[0]}}})
	v := ansi.Strip(m.View())
	if l := runRow(t, v, "#16"); !strings.Contains(l, "new") {
		t.Errorf("new run row %q, want new tag", l)
	}
	if l := runRow(t, v, "#15"); strings.Contains(l, "new") {
		t.Errorf("unchanged run row %q marked", l)
	}
}

func TestLiveChangesAreNotMarked(t *testing.T) {
	m := newModel(t)
	done := dotfilesRuns[0]
	done.Status, done.Conclusion = "completed", "success"

	m, _ = send(m, watch.Event{Repo: "EvilNick2/dotfiles", Runs: []runs.Run{done, dotfilesRuns[1]},
		Changes: []runs.Change{{Prev: &dotfilesRuns[0], Run: done}}})
	if l := runRow(t, m.View(), "#16"); strings.Contains(l, "new") {
		t.Errorf("live change marked: %q", l)
	}
}

func TestSidebarMarksUnviewedRepoWithChangesUntilVisited(t *testing.T) {
	m := newModel(t)

	m, _ = send(m, watch.Event{Repo: "EvilNick2/orpheus", Initial: true, Runs: orpheusRuns,
		Changes: []runs.Change{{Run: orpheusRuns[0]}}})
	if l := sideRow(t, m.View(), "orpheus"); !strings.Contains(l, "new") {
		t.Errorf("sidebar row %q, want new tag", l)
	}
	m, _ = send(m, key("j"))
	if l := sideRow(t, m.View(), "orpheus"); strings.Contains(l, "new") {
		t.Errorf("marker kept after visiting: %q", l)
	}
}

func TestTwoBorderedPanesFillTheSize(t *testing.T) {
	v := ansi.Strip(newModel(t).View())

	lines := strings.Split(v, "\n")
	if len(lines) != 20 {
		t.Fatalf("%d lines, want 20", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 100 {
			t.Errorf("line %d is %d wide, want 100: %q", i, w, l)
		}
	}
	if !strings.Contains(lines[0], "repos") || !strings.Contains(lines[0], "EvilNick2/dotfiles") {
		t.Errorf("pane titles missing from %q", lines[0])
	}
}

func TestRunRowHasTitleOnSecondLine(t *testing.T) {
	v := ansi.Strip(newModel(t).View())

	lines := strings.Split(v, "\n")
	for i, l := range lines {
		if _, runs := panes(l); strings.Contains(runs, "#15") {
			if _, next := panes(lines[i+1]); !strings.Contains(next, "docs: setup notes") {
				t.Errorf("line after #15 is %q, want its title", next)
			}
			return
		}
	}
	t.Fatalf("no #15 row in:\n%s", v)
}

func TestRunWithoutCreationTimeShowsNoAge(t *testing.T) {
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(100, 10)
	m, _ = send(m, watch.Event{Repo: "o/r", Initial: true, Runs: []runs.Run{{ID: 1, RunNumber: 7, Name: "build", Status: "completed", Conclusion: "success"}}})

	if l := runRow(t, m.View(), "#7"); strings.Contains(l, "ago") {
		t.Errorf("row %q shows an age for a run with no creation time", l)
	}
}

// Layout at 100x20 with two repos: the repos pane is 16 wide, so repo i is
// at y 1+i inside it, and in the runs pane the status line is y 1 and run i
// starts at y 2+2i.

func click(x, y int) mouse.Event  { return mouse.Event{X: x, Y: y, Kind: mouse.Click} }
func double(x, y int) mouse.Event { return mouse.Event{X: x, Y: y, Kind: mouse.DoubleClick} }

func TestClickRepoSelectsIt(t *testing.T) {
	m := newModel(t)

	m, _ = m.Mouse(click(5, 2))
	if repo, r, _ := m.Current(); repo != "EvilNick2/orpheus" || r.ID != 41 {
		t.Errorf("Current() = %q %d, want orpheus run 41", repo, r.ID)
	}
}

func TestClickRunSelectsAndFocusesRuns(t *testing.T) {
	m := newModel(t)

	m, cmd := m.Mouse(click(40, 4)) // second run's first line
	if cmd != nil {
		t.Error("single click opened something")
	}
	if _, r, _ := m.Current(); r.ID != 15 {
		t.Errorf("Current() = %d, want 15", r.ID)
	}
	m, _ = m.Mouse(click(40, 5)) // its title line selects it too
	if _, r, _ := m.Current(); r.ID != 15 {
		t.Errorf("click on title line: Current() = %d, want 15", r.ID)
	}
	// Focus moved to the runs pane, so j moves the run cursor.
	m, _ = send(m, key("k"))
	if _, r, _ := m.Current(); r.ID != 16 {
		t.Errorf("k after clicking a run moved to %d, want run 16", r.ID)
	}
}

func TestDoubleClickRunOpensIt(t *testing.T) {
	m := newModel(t)

	_, cmd := m.Mouse(double(40, 2))
	if cmd == nil {
		t.Fatal("double click returned no command")
	}
	if msg, ok := cmd().(OpenRunMsg); !ok || msg.Run.ID != 16 {
		t.Errorf("got %#v, want OpenRunMsg for run 16", cmd())
	}
}

func TestClickOnEmptySpaceDoesNothing(t *testing.T) {
	m := newModel(t)

	m, cmd := m.Mouse(click(40, 15))
	if cmd != nil {
		t.Error("click below the runs returned a command")
	}
	if repo, r, _ := m.Current(); repo != "EvilNick2/dotfiles" || r.ID != 16 {
		t.Errorf("Current() = %q %d, want unchanged", repo, r.ID)
	}
}

func TestWheelMovesSelectionInPaneUnderPointer(t *testing.T) {
	m := newModel(t)

	m, _ = m.Mouse(mouse.Event{X: 40, Y: 5, Kind: mouse.WheelDown})
	if repo, r, _ := m.Current(); repo != "EvilNick2/dotfiles" || r.ID != 15 {
		t.Errorf("wheel over runs: %q %d, want dotfiles run 15", repo, r.ID)
	}
	m, _ = m.Mouse(mouse.Event{X: 5, Y: 5, Kind: mouse.WheelDown})
	if repo, _, _ := m.Current(); repo != "EvilNick2/orpheus" {
		t.Errorf("wheel over repos: %q, want orpheus", repo)
	}
}
