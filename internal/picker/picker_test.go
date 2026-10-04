package picker

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EvilNick2/gh-argus/internal/repos"
)

var sample = []repos.Repo{
	{FullName: "EvilNick2/dotfiles", Owner: "EvilNick2", Name: "dotfiles", Private: true},
	{FullName: "Bath-Impact-Lab/aXR-www", Owner: "Bath-Impact-Lab", Name: "aXR-www"},
	{FullName: "EvilNick2/fonp", Owner: "EvilNick2", Name: "fonp"},
	{FullName: "psyeo2/homelab-gitops", Owner: "psyeo2", Name: "homelab-gitops"},
}

// requests records every batch of names the picker asks statuses for.
type requests struct{ batches [][]string }

func (r *requests) fetch(names []string) tea.Cmd {
	r.batches = append(r.batches, slices.Clone(names))
	return nil
}

func (r *requests) all() []string {
	var out []string
	for _, b := range r.batches {
		out = append(out, b...)
	}
	return out
}

func newModel(t *testing.T, rs []repos.Repo, selected []string, height int) (Model, *requests) {
	t.Helper()
	req := &requests{}
	m := New(rs, selected, req.fetch)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: height})
	return m, req
}

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(s string) tea.Msg {
	switch s {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func typeText(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, key(string(r)))
	}
	return out
}

func TestSpaceTogglesAndEnterConfirmsSelection(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, key("space"), key("j"), key("j"), key("space"), key("enter"))
	if !m.Done() || m.Cancelled() {
		t.Fatalf("done=%v cancelled=%v", m.Done(), m.Cancelled())
	}
	got := m.Selected()
	want := []string{"EvilNick2/dotfiles", "EvilNick2/fonp"}
	if !slices.Equal(got, want) {
		t.Errorf("Selected() = %v, want %v", got, want)
	}
}

func TestSpaceTwiceDeselects(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, key("space"), key("space"), key("j"), key("space"), key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"Bath-Impact-Lab/aXR-www"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestEnterWithNothingSelectedTakesCursorRepo(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, key("j"), key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"Bath-Impact-Lab/aXR-www"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestInitialSelectionIsKeptAndDroppedReposIgnored(t *testing.T) {
	m, _ := newModel(t, sample, []string{"psyeo2/homelab-gitops", "gone/deleted"}, 20)

	m = send(m, key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"psyeo2/homelab-gitops"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestQuitKeysCancel(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		m, _ := newModel(t, sample, nil, 20)
		next, cmd := m.Update(key(k))
		m = next.(Model)
		if !m.Cancelled() || cmd == nil {
			t.Errorf("%s: cancelled=%v cmd=%v, want cancelled with a quit command", k, m.Cancelled(), cmd)
		}
	}
}

func TestCursorStopsAtEnds(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, key("k"), key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"EvilNick2/dotfiles"}) {
		t.Errorf("after k at top: %v", got)
	}
	m, _ = newModel(t, sample, nil, 20)
	m = send(m, key("j"), key("j"), key("j"), key("j"), key("j"), key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"psyeo2/homelab-gitops"}) {
		t.Errorf("after j past bottom: %v", got)
	}
}

func TestFilterNarrowsListAndCursorFollowsMatches(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	msgs := append([]tea.Msg{key("/")}, typeText("homelab")...)
	msgs = append(msgs, key("enter"), key("enter"))
	m = send(m, msgs...)
	if got := m.Selected(); !slices.Equal(got, []string{"psyeo2/homelab-gitops"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestFilterModeTreatsLettersAsText(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	// q and j would quit and move outside filter mode.
	msgs := append([]tea.Msg{key("/")}, typeText("fonpq")...)
	msgs = append(msgs, key("backspace"), key("enter"), key("enter"))
	m = send(m, msgs...)
	if m.Cancelled() {
		t.Fatal("q in filter mode quit")
	}
	if got := m.Selected(); !slices.Equal(got, []string{"EvilNick2/fonp"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestEscInFilterModeClearsFilter(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	msgs := append([]tea.Msg{key("/")}, typeText("homelab")...)
	msgs = append(msgs, key("esc"), key("enter"))
	m = send(m, msgs...)
	if m.Cancelled() {
		t.Fatal("esc in filter mode quit")
	}
	if got := m.Selected(); !slices.Equal(got, []string{"EvilNick2/dotfiles"}) {
		t.Errorf("Selected() = %v, want the first repo of the unfiltered list", got)
	}
}

func TestOwnerCyclesThroughOwnersThenAll(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	// Owners in first-seen order: EvilNick2, Bath-Impact-Lab, psyeo2.
	m = send(m, key("o"), key("o"))
	if v := m.View().Content; !strings.Contains(v, "aXR-www") || strings.Contains(v, "dotfiles") {
		t.Errorf("owner Bath-Impact-Lab view:\n%s", v)
	}
	m = send(m, key("o"), key("o"))
	if v := m.View().Content; !strings.Contains(v, "aXR-www") || !strings.Contains(v, "dotfiles") {
		t.Errorf("back to all owners view:\n%s", v)
	}
}

func TestSelectionSurvivesFiltering(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	msgs := []tea.Msg{key("space"), key("/")}
	msgs = append(msgs, typeText("fonp")...)
	msgs = append(msgs, key("enter"), key("space"), key("enter"))
	m = send(m, msgs...)
	if got := m.Selected(); !slices.Equal(got, []string{"EvilNick2/dotfiles", "EvilNick2/fonp"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func many(n int) []repos.Repo {
	var out []repos.Repo
	for i := range n {
		name := "r" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		out = append(out, repos.Repo{FullName: "o/" + name, Owner: "o", Name: name})
	}
	return out
}

func TestStatusesRequestedOnlyForRowsOnScreen(t *testing.T) {
	// Height 10 leaves fewer than 10 rows once header and footer are drawn.
	_, req := newModel(t, many(60), nil, 10)

	got := req.all()
	if len(got) == 0 || len(got) >= 10 {
		t.Fatalf("requested %d statuses, want the rows on screen only: %v", len(got), got)
	}
	if got[0] != "o/raa" {
		t.Errorf("first requested %q, want o/raa", got[0])
	}
}

func TestStatusesNotRequestedTwice(t *testing.T) {
	m, req := newModel(t, many(60), nil, 10)
	before := len(req.all())

	m = send(m, key("j"), key("k"), tea.WindowSizeMsg{Width: 80, Height: 10})
	if after := len(req.all()); after != before {
		t.Errorf("requested %d more statuses for rows already requested", after-before)
	}
}

func TestScrollingRequestsNewRows(t *testing.T) {
	m, req := newModel(t, many(60), nil, 10)
	before := req.all()

	var msgs []tea.Msg
	for range 30 {
		msgs = append(msgs, key("j"))
	}
	send(m, msgs...)
	after := req.all()
	if len(after) <= len(before) || !slices.Contains(after, "o/rbe") {
		t.Errorf("after scrolling to row 30 requested %v", after[len(before):])
	}
}

func TestStatusBatchesAreAtMost20(t *testing.T) {
	_, req := newModel(t, many(60), nil, 60)

	for _, b := range req.batches {
		if len(b) > 20 {
			t.Errorf("batch of %d", len(b))
		}
	}
	if len(req.all()) < 40 {
		t.Errorf("requested %d, want every row on a 60 line screen", len(req.all()))
	}
}

func TestStatusMsgShowsBadge(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, StatusMsg{Statuses: map[string]repos.Status{"EvilNick2/dotfiles": repos.StatusFailing}})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, "dotfiles") {
			if !strings.Contains(line, "fail") {
				t.Errorf("dotfiles row has no fail badge: %q", line)
			}
			return
		}
	}
	t.Error("no dotfiles row in view")
}

func TestReposMsgReplacesListKeepingSelection(t *testing.T) {
	m, _ := newModel(t, sample[:2], nil, 20)

	m = send(m, key("space"), ReposMsg{Repos: sample}, key("j"), key("j"), key("j"), key("space"), key("enter"))
	if got := m.Selected(); !slices.Equal(got, []string{"EvilNick2/dotfiles", "psyeo2/homelab-gitops"}) {
		t.Errorf("Selected() = %v", got)
	}
}

func TestViewMarksPrivateRepos(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, "EvilNick2/fonp") && strings.Contains(line, "private") {
			t.Errorf("public repo marked private: %q", line)
		}
		if strings.Contains(line, "EvilNick2/dotfiles") && !strings.Contains(line, "private") {
			t.Errorf("private repo not marked: %q", line)
		}
	}
}

func TestInitRunsGivenCommand(t *testing.T) {
	ran := false
	m := New(sample, nil, nil).WithInit(func() tea.Msg { ran = true; return nil })

	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init returned no command")
	} else {
		cmd()
	}
	if !ran {
		t.Error("Init did not return the command given to WithInit")
	}
}

func TestReposMsgWithErrorStillAppliesRepos(t *testing.T) {
	m, _ := newModel(t, sample[:1], nil, 20)

	m = send(m, ReposMsg{Repos: sample, Err: errors.New("saving cache: disk full")})
	v := m.View().Content
	if !strings.Contains(v, "homelab-gitops") || !strings.Contains(v, "disk full") {
		t.Errorf("want new repos and the error shown, got:\n%s", v)
	}
}

func TestReposMsgWithOnlyErrorKeepsList(t *testing.T) {
	m, _ := newModel(t, sample, nil, 20)

	m = send(m, ReposMsg{Err: errors.New("offline")})
	v := m.View().Content
	if !strings.Contains(v, "dotfiles") || !strings.Contains(v, "offline") {
		t.Errorf("want old repos kept and the error shown, got:\n%s", v)
	}
}

func TestEmptyListShowsLoadingUntilReposArrive(t *testing.T) {
	m, _ := newModel(t, nil, nil, 20)
	if !strings.Contains(m.View().Content, "loading repos") {
		t.Errorf("empty picker shows no loading hint:\n%s", m.View().Content)
	}

	m = send(m, ReposMsg{Repos: []repos.Repo{}})
	if strings.Contains(m.View().Content, "loading repos") {
		t.Error("still loading after an empty list arrived")
	}
}
