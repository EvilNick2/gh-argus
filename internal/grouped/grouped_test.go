package grouped

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type thing struct {
	id   int64
	name string
}

var view = View[thing]{
	Row: func(t thing, selected bool) string {
		if selected {
			return "> " + t.name
		}
		return "  " + t.name
	},
	Empty: "nothing here",
}

func newModel(h int, repos ...string) Model[thing] {
	return New(repos, func(t thing) int64 { return t.id }).SetSize(80, h)
}

var a = []thing{{1, "a1"}, {2, "a2"}, {3, "a3"}}
var b = []thing{{4, "b1"}}

func loaded() Model[thing] {
	m := newModel(20, "o/a", "o/b")
	m = m.Load("o/a", a, nil)
	return m.Load("o/b", b, nil)
}

func TestLoadingErrorAndEmptyStates(t *testing.T) {
	m := newModel(20, "o/a", "o/b", "o/c")
	m = m.Load("o/b", nil, errors.New("502 Bad Gateway"))
	m = m.Load("o/c", []thing{}, nil)

	v := m.Render(view)
	for _, want := range []string{"o/a", "loading", "o/b", "502 Bad Gateway", "o/c", "nothing here"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in:\n%s", want, v)
		}
	}
}

func TestCustomErrorRow(t *testing.T) {
	v := view
	v.Error = func(err error) string { return "not permitted" }
	m := newModel(20, "o/a").Load("o/a", nil, errors.New("404"))

	if out := m.Render(v); !strings.Contains(out, "not permitted") || strings.Contains(out, "404") {
		t.Errorf("view:\n%s", out)
	}
}

func TestHeaderExtra(t *testing.T) {
	v := view
	v.Header = func(repo string, items []thing) string { return fmt.Sprintf("%d items", len(items)) }

	if out := loaded().Render(v); !strings.Contains(out, "o/a") || !strings.Contains(out, "3 items") {
		t.Errorf("view:\n%s", out)
	}
}

func TestCursorMovesAcrossGroupsSkippingHeaders(t *testing.T) {
	m := loaded()

	if repo, it, ok := m.Current(); !ok || repo != "o/a" || it.id != 1 {
		t.Fatalf("start %q %v %v", repo, it, ok)
	}
	m = m.Key("j").Key("j").Key("j")
	if repo, it, _ := m.Current(); repo != "o/b" || it.id != 4 {
		t.Errorf("after 3 j: %q %v", repo, it)
	}
	m = m.Key("j")
	if _, it, _ := m.Current(); it.id != 4 {
		t.Errorf("moved past the end to %v", it)
	}
	m = m.Key("k")
	if _, it, _ := m.Current(); it.id != 3 {
		t.Errorf("k: %v", it)
	}
	m = m.Key("g")
	if _, it, _ := m.Current(); it.id != 1 {
		t.Errorf("g: %v", it)
	}
	m = m.Key("G")
	if _, it, _ := m.Current(); it.id != 4 {
		t.Errorf("G: %v", it)
	}
}

func TestSelectedRowRendered(t *testing.T) {
	out := loaded().Key("j").Render(view)

	if !strings.Contains(out, "> a2") || strings.Contains(out, "> a1") {
		t.Errorf("view:\n%s", out)
	}
}

func TestCurrentWithNothingLoaded(t *testing.T) {
	if _, _, ok := newModel(20, "o/a").Current(); ok {
		t.Error("Current() ok with nothing loaded")
	}
}

func TestCursorKeptOnItemWhenReloadedAfterMoving(t *testing.T) {
	m := loaded().Key("j") // a2

	m = m.Load("o/a", []thing{{9, "new"}, {1, "a1"}, {2, "a2"}}, nil)
	if _, it, _ := m.Current(); it.id != 2 {
		t.Errorf("Current() = %v, want a2 still", it)
	}
}

func TestCursorStaysAtTopWhenLaterGroupsLoadAbove(t *testing.T) {
	m := newModel(20, "o/a", "o/b")

	m = m.Load("o/b", b, nil)
	m = m.Load("o/a", a, nil)
	if repo, it, _ := m.Current(); repo != "o/a" || it.id != 1 {
		t.Errorf("Current() = %q %v, want the first item of the first repo", repo, it)
	}
}

func TestScrollsToKeepCursorVisible(t *testing.T) {
	var many []thing
	for i := range 30 {
		many = append(many, thing{int64(i), fmt.Sprintf("t%02d", i)})
	}
	m := newModel(6, "o/a").Load("o/a", many, nil)
	for range 20 {
		m = m.Key("j")
	}

	out := m.Render(view)
	if !strings.Contains(out, "t20") || strings.Contains(out, "t00") {
		t.Errorf("not scrolled:\n%s", out)
	}
	if n := strings.Count(out, "\n") + 1; n > 6 {
		t.Errorf("%d lines, taller than 6", n)
	}
}

func TestLastItemShowsTrailingGroupsWithoutItems(t *testing.T) {
	m := newModel(4, "o/a", "o/empty").Load("o/a", a, nil).Load("o/empty", []thing{}, nil)

	out := m.Key("G").Render(view)
	if !strings.Contains(out, "o/empty") || !strings.Contains(out, "nothing here") || !strings.Contains(out, "a3") {
		t.Errorf("end not shown:\n%s", out)
	}
}

func TestCursorVisibleWithManyTrailingGroups(t *testing.T) {
	repos := []string{"o/a"}
	for i := range 10 {
		repos = append(repos, fmt.Sprintf("o/empty%d", i))
	}
	m := newModel(4, repos...).Load("o/a", a, nil)
	for _, r := range repos[1:] {
		m = m.Load(r, []thing{}, nil)
	}

	if out := m.Key("G").Render(view); !strings.Contains(out, "a3") {
		t.Errorf("cursor scrolled away:\n%s", out)
	}
}

func TestLoadForUnknownRepoIgnored(t *testing.T) {
	m := loaded().Load("o/zzz", b, nil)

	if strings.Contains(m.Render(view), "o/zzz") {
		t.Error("unknown repo rendered")
	}
}

func TestAllListsEveryLoadedItemInOrder(t *testing.T) {
	got := loaded().All()

	if len(got) != 4 || got[0].id != 1 || got[3].id != 4 {
		t.Errorf("All() = %v", got)
	}
}
