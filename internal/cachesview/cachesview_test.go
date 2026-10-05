package cachesview

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/caches"
	"github.com/EvilNick2/gh-argus/internal/mouse"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

var orpheus = []caches.Cache{
	{ID: 1, Key: "Linux-node-208b2f", Ref: "refs/heads/main", SizeInBytes: 3 * 1024 * 1024, LastAccessedAt: now.Add(-2 * time.Hour)},
	{ID: 2, Key: "Linux-go-build-91aa", Ref: "refs/heads/dev", SizeInBytes: 512 * 1024, LastAccessedAt: now.Add(-50 * time.Hour)},
}

func newModel() Model {
	m := New([]string{"EvilNick2/orpheus", "EvilNick2/fonp"}, func() time.Time { return now }).SetSize(100, 20)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/orpheus", Caches: orpheus})
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/fonp", Caches: []caches.Cache{}})
	return m
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

func TestCacheRows(t *testing.T) {
	v := view(newModel())

	if l := line(t, v, "Linux-node-208b2f"); !strings.Contains(l, "main") || !strings.Contains(l, "3.0 MB") || !strings.Contains(l, "2h") {
		t.Errorf("node row %q", l)
	}
	if l := line(t, v, "Linux-go-build-91aa"); !strings.Contains(l, "dev") || !strings.Contains(l, "512 KB") || !strings.Contains(l, "2d") {
		t.Errorf("go row %q", l)
	}
}

func TestHeaderTotals(t *testing.T) {
	if l := line(t, view(newModel()), "EvilNick2/orpheus"); !strings.Contains(l, "2 caches, 3.5 MB") {
		t.Errorf("header %q", l)
	}
}

func TestNoCaches(t *testing.T) {
	if !strings.Contains(view(newModel()), "no caches") {
		t.Errorf("view:\n%s", view(newModel()))
	}
}

func TestCurrent(t *testing.T) {
	m := newModel()

	repo, c, ok := m.Current()
	if !ok || repo != "EvilNick2/orpheus" || c.ID != 1 {
		t.Errorf("Current() = %q %d %v", repo, c.ID, ok)
	}
}

func TestSize(t *testing.T) {
	cases := map[int64]string{
		0:                      "0 B",
		900:                    "900 B",
		2048:                   "2 KB",
		1536 * 1024:            "1.5 MB",
		3 * 1024 * 1024 * 1024: "3.0 GB",
	}
	for n, want := range cases {
		if got := size(n); got != want {
			t.Errorf("size(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestClickSelectsCache(t *testing.T) {
	// orpheus header at y 1, its caches at y 2 and 3.
	m := newModel().Mouse(mouse.Event{X: 10, Y: 3, Kind: mouse.DoubleClick})

	if _, c, _ := m.Current(); c.ID != 2 {
		t.Errorf("Current() = %d, want 2", c.ID)
	}
}
