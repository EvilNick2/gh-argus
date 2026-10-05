package metricsview

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/runs"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// run makes a completed run of workflow wf created hoursAgo hours before
// now that ran for d seconds.
func run(wf int64, name string, hoursAgo int, conclusion string, d int) runs.Run {
	created := now.Add(-time.Duration(hoursAgo) * time.Hour)
	return runs.Run{
		ID: int64(1000 - hoursAgo), WorkflowID: wf, Name: name, Status: "completed", Conclusion: conclusion,
		RunAttempt: 1, CreatedAt: created, RunStartedAt: created, UpdatedAt: created.Add(time.Duration(d) * time.Second),
	}
}

// dotfiles runs, newest first: ci passes, fails, passes, then deploy fails
// once. ci durations rise from 10s to 20s.
var dotfiles = []runs.Run{
	run(1, "Manifest check", 1, "success", 20),
	run(2, "Build and publish", 2, "failure", 62),
	run(1, "Manifest check", 3, "success", 20),
	run(1, "Manifest check", 4, "failure", 10),
	run(1, "Manifest check", 5, "success", 10),
}

func newModel() Model {
	m := New([]string{"EvilNick2/dotfiles", "o/empty"}, func() time.Time { return now }).SetSize(110, 20)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Runs: dotfiles})
	m, _ = m.Update(LoadedMsg{Repo: "o/empty", Runs: []runs.Run{}})
	return m
}

func key(s string) tea.Msg {
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func view(m Model) string { return ansi.Strip(m.View()) }

func line(t *testing.T, v, substr string) string {
	t.Helper()
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substr, v)
	return ""
}

// fields splits a pane row into words, without the side borders.
func fields(l string) []string {
	return strings.Fields(strings.Trim(l, "│"))
}

func TestLoadingBeforeRunsArrive(t *testing.T) {
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(110, 20)

	if v := view(m); !strings.Contains(v, "o/r") || !strings.Contains(v, "loading") {
		t.Errorf("view:\n%s", v)
	}
}

func TestHeaderNamesColumns(t *testing.T) {
	l := strings.Split(view(newModel()), "\n")[1]

	for _, want := range []string{"runs", "success", "median", "trend", "reruns", "recent"} {
		if !strings.Contains(l, want) {
			t.Errorf("header %q missing %q", l, want)
		}
	}
}

func TestRepoTotalRow(t *testing.T) {
	f := fields(line(t, view(newModel()), "EvilNick2/dotfiles"))

	// 5 runs, 3 of 5 passed, median of 20,62,20,10,10 is 20s.
	if strings.Join(f[1:4], " ") != "5 60% 20s" {
		t.Errorf("total row fields %q", f)
	}
}

func TestWorkflowRowsWithTrendAndHistory(t *testing.T) {
	v := view(newModel())

	ci := line(t, v, "Manifest check")
	// 4 runs, 3 of 4 passed, median 15s, newer half 20s vs older 10s = +100%.
	for _, want := range []string{" 4 ", "75%", "15s", "+100%", "+x++"} {
		if !strings.Contains(ci, want) {
			t.Errorf("ci row %q missing %q", ci, want)
		}
	}
	deploy := line(t, v, "Build and publish")
	if !strings.Contains(deploy, "1m02s") || !strings.Contains(deploy, "0%") {
		t.Errorf("deploy row %q", deploy)
	}
}

func TestWorkflowsListedUnderTheirRepo(t *testing.T) {
	v := view(newModel())

	lines := strings.Split(v, "\n")
	idx := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	if !(idx("EvilNick2/dotfiles") < idx("Manifest check") && idx("Manifest check") < idx("Build and publish") &&
		idx("Build and publish") < idx("o/empty")) {
		t.Errorf("order wrong:\n%s", v)
	}
}

func TestUndefinedValuesShowDash(t *testing.T) {
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(110, 20)
	m, _ = m.Update(LoadedMsg{Repo: "o/r", Runs: []runs.Run{run(1, "ci", 1, "cancelled", 10)}})

	f := fields(line(t, view(m), "o/r"))
	if f[2] != "-" || f[4] != "-" {
		t.Errorf("fields %q, want - for success rate and trend", f)
	}
}

func TestRepoWithNoRuns(t *testing.T) {
	if l := line(t, view(newModel()), "o/empty"); !strings.Contains(l, "no completed runs") {
		t.Errorf("row %q", l)
	}
}

func TestDetailLineFollowsCursor(t *testing.T) {
	m := newModel()

	if v := view(m); !strings.Contains(v, "EvilNick2/dotfiles: last run 1h ago, queue median 0s, 0 cancelled") {
		t.Errorf("detail for total:\n%s", v)
	}
	m, _ = m.Update(key("j"))
	m, _ = m.Update(key("j"))
	if v := view(m); !strings.Contains(v, "Build and publish: last run 2h ago") {
		t.Errorf("detail after j j:\n%s", v)
	}
}

func TestErrorShownForRepo(t *testing.T) {
	m := newModel()

	m, _ = m.Update(LoadedMsg{Repo: "o/empty", Err: errors.New("GET runs: 502")})
	if v := view(m); !strings.Contains(v, "502") {
		t.Errorf("view:\n%s", v)
	}
}

func TestScrollsToKeepCursorVisible(t *testing.T) {
	var rs []runs.Run
	for i := range 30 {
		rs = append(rs, run(int64(i), fmt.Sprintf("wf%02d", i), i+1, "success", 10))
	}
	m := New([]string{"o/r"}, func() time.Time { return now }).SetSize(110, 8)
	m, _ = m.Update(LoadedMsg{Repo: "o/r", Runs: rs})
	for range 20 {
		m, _ = m.Update(key("j"))
	}
	v := view(m)
	if !strings.Contains(v, "wf19") || strings.Contains(v, "wf00 ") {
		t.Errorf("cursor not scrolled into view:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n > 8 {
		t.Errorf("view is %d lines, taller than 8", n)
	}
}

func TestMetricsIsOnePaneOfFullSize(t *testing.T) {
	lines := strings.Split(view(newModel()), "\n")

	if len(lines) != 20 {
		t.Fatalf("%d lines, want 20", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 110 {
			t.Errorf("line %d is %d wide, want 110: %q", i, w, l)
		}
	}
}
