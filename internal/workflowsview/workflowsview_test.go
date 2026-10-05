package workflowsview

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/workflows"
)

var dotfiles = []workflows.Workflow{
	{ID: 1, Name: "Build and publish", Path: ".github/workflows/deploy-pages.yml", State: "active"},
	{ID: 2, Name: "Manifest check", Path: ".github/workflows/manifest-check.yml", State: "disabled_manually"},
	{ID: 3, Name: "pages-build-deployment", Path: "dynamic/pages/pages-build-deployment", State: "active"},
}

var orpheus = []workflows.Workflow{
	{ID: 4, Name: "Release", Path: ".github/workflows/release.yml", State: "disabled_inactivity"},
}

func newModel() Model {
	m := New([]string{"EvilNick2/dotfiles", "EvilNick2/orpheus"}).SetSize(100, 20)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Workflows: dotfiles})
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/orpheus", Workflows: orpheus})
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

func TestLoadingBeforeWorkflowsArrive(t *testing.T) {
	m := New([]string{"o/r"}).SetSize(100, 20)

	v := view(m)
	if !strings.Contains(v, "o/r") || !strings.Contains(v, "loading") {
		t.Errorf("view:\n%s", v)
	}
}

func TestWorkflowsGroupedUnderRepos(t *testing.T) {
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
	if !(idx("EvilNick2/dotfiles") < idx("Build and publish") &&
		idx("Build and publish") < idx("EvilNick2/orpheus") &&
		idx("EvilNick2/orpheus") < idx("Release")) {
		t.Errorf("not grouped in order:\n%s", v)
	}
}

func TestStatesAndPaths(t *testing.T) {
	v := view(newModel())

	if l := line(t, v, "Build and publish"); !strings.Contains(l, "on") || !strings.Contains(l, ".github/workflows/deploy-pages.yml") {
		t.Errorf("active row %q", l)
	}
	if l := line(t, v, "Manifest check"); !strings.Contains(l, "off") || !strings.Contains(l, "disabled manually") {
		t.Errorf("disabled row %q", l)
	}
	if l := line(t, v, "Release"); !strings.Contains(l, "disabled inactivity") {
		t.Errorf("inactive row %q", l)
	}
	if l := line(t, v, "pages-build-deployment"); !strings.Contains(l, "dynamic, cannot dispatch") {
		t.Errorf("dynamic row %q", l)
	}
}

func TestCursorMovesAcrossReposSkippingHeaders(t *testing.T) {
	m := newModel()

	repo, wf, ok := m.Current()
	if !ok || repo != "EvilNick2/dotfiles" || wf.ID != 1 {
		t.Fatalf("start: %q %d %v", repo, wf.ID, ok)
	}
	for range 3 {
		m, _ = m.Update(key("j"))
	}
	if repo, wf, _ := m.Current(); repo != "EvilNick2/orpheus" || wf.ID != 4 {
		t.Errorf("after 3 j: %q %d, want orpheus workflow 4", repo, wf.ID)
	}
	m, _ = m.Update(key("j"))
	if _, wf, _ := m.Current(); wf.ID != 4 {
		t.Errorf("moved past the last workflow to %d", wf.ID)
	}
	m, _ = m.Update(key("k"))
	if _, wf, _ := m.Current(); wf.ID != 3 {
		t.Errorf("k: %d, want 3", wf.ID)
	}
}

func TestCurrentWithNothingLoaded(t *testing.T) {
	if _, _, ok := New([]string{"o/r"}).Current(); ok {
		t.Error("Current() ok with nothing loaded")
	}
}

func TestCursorKeptOnWorkflowWhenReloaded(t *testing.T) {
	m := newModel()
	m, _ = m.Update(key("j")) // Manifest check

	enabled := append([]workflows.Workflow(nil), dotfiles...)
	enabled[1].State = "active"
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Workflows: enabled})
	if _, wf, _ := m.Current(); wf.ID != 2 || wf.State != "active" {
		t.Errorf("Current() = %+v, want Manifest check now active", wf)
	}
}

func TestErrorShownUnderRepo(t *testing.T) {
	m := newModel()

	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/orpheus", Err: errors.New("GET workflows: 502")})
	v := view(m)
	if !strings.Contains(v, "502") || !strings.Contains(v, "Build and publish") {
		t.Errorf("view:\n%s", v)
	}
}

func TestRepoWithNoWorkflows(t *testing.T) {
	m := New([]string{"o/r"}).SetSize(100, 20)

	m, _ = m.Update(LoadedMsg{Repo: "o/r", Workflows: []workflows.Workflow{}})
	if v := view(m); !strings.Contains(v, "no workflows") {
		t.Errorf("view:\n%s", v)
	}
}

func TestScrollsToKeepCursorVisible(t *testing.T) {
	var many []workflows.Workflow
	for i := range 30 {
		many = append(many, workflows.Workflow{ID: int64(i), Name: fmt.Sprintf("wf%02d", i), Path: ".github/workflows/x.yml", State: "active"})
	}
	m := New([]string{"o/r"}).SetSize(100, 8)
	m, _ = m.Update(LoadedMsg{Repo: "o/r", Workflows: many})
	for range 20 {
		m, _ = m.Update(key("j"))
	}
	v := view(m)
	if !strings.Contains(v, "wf20") || strings.Contains(v, "wf00") {
		t.Errorf("cursor not scrolled into view:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n > 8 {
		t.Errorf("view is %d lines, taller than 8", n)
	}
}

func TestCursorStaysAtTopWhenLaterReposLoadAbove(t *testing.T) {
	m := New([]string{"EvilNick2/dotfiles", "EvilNick2/orpheus"}).SetSize(100, 20)

	// orpheus answers first, then dotfiles lands above it.
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/orpheus", Workflows: orpheus})
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Workflows: dotfiles})
	if repo, wf, _ := m.Current(); repo != "EvilNick2/dotfiles" || wf.ID != 1 {
		t.Errorf("Current() = %q %d, want the first workflow of the first repo", repo, wf.ID)
	}
}

func TestLastWorkflowShowsTrailingReposWithout(t *testing.T) {
	m := New([]string{"EvilNick2/dotfiles", "o/empty"}).SetSize(100, 5)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Workflows: dotfiles})
	m, _ = m.Update(LoadedMsg{Repo: "o/empty", Workflows: []workflows.Workflow{}})

	m, _ = m.Update(key("G"))
	v := view(m)
	if !strings.Contains(v, "o/empty") || !strings.Contains(v, "no workflows") || !strings.Contains(v, "pages-build-deployment") {
		t.Errorf("end of list not shown:\n%s", v)
	}
}

func TestCursorVisibleWithManyTrailingReposWithout(t *testing.T) {
	repos := []string{"EvilNick2/dotfiles"}
	for i := range 10 {
		repos = append(repos, fmt.Sprintf("o/empty%d", i))
	}
	m := New(repos).SetSize(100, 4)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/dotfiles", Workflows: dotfiles})
	for _, r := range repos[1:] {
		m, _ = m.Update(LoadedMsg{Repo: r, Workflows: []workflows.Workflow{}})
	}

	m, _ = m.Update(key("G"))
	if v := view(m); !strings.Contains(v, "pages-build-deployment") {
		t.Errorf("cursor scrolled out of view:\n%s", v)
	}
}

func TestDoubleClickWorkflowActivates(t *testing.T) {
	// dotfiles header at y 1, its workflows at y 2 to 4.
	m, activated := newModel().Mouse(mouse.Event{X: 10, Y: 3, Kind: mouse.DoubleClick})

	if !activated {
		t.Error("double click did not activate")
	}
	if _, wf, _ := m.Current(); wf.ID != 2 {
		t.Errorf("Current() = %d, want 2", wf.ID)
	}
}
