// Package cachesview is the Cache tab: each watched repo's Actions caches,
// with their total size in the repo header.
package cachesview

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/caches"
	"github.com/EvilNick2/gh-argus/internal/grouped"
	"github.com/EvilNick2/gh-argus/internal/theme"
	"github.com/EvilNick2/gh-argus/internal/timefmt"
)

// LoadedMsg delivers one repo's caches, or the error fetching them.
type LoadedMsg struct {
	Repo   string
	Caches []caches.Cache
	Err    error
}

type Model struct {
	list grouped.Model[caches.Cache]
	now  func() time.Time
}

func New(repos []string, now func() time.Time) Model {
	return Model{list: grouped.New(repos, func(c caches.Cache) int64 { return c.ID }), now: now}
}

func (m Model) SetSize(w, h int) Model {
	m.list = m.list.SetSize(w, h)
	return m
}

// Current returns the cache under the cursor.
func (m Model) Current() (string, caches.Cache, bool) {
	return m.list.Current()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		m.list = m.list.Load(msg.Repo, msg.Caches, msg.Err)
	case tea.KeyPressMsg:
		m.list = m.list.Key(msg.String())
	}
	return m, nil
}

func size(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%d KB", n/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:max(0, n-2)]) + ".."
}

func (m Model) View() string {
	keyW, refW := 10, 6
	for _, c := range m.list.All() {
		keyW = max(keyW, len([]rune(c.Key)))
		refW = max(refW, len([]rune(c.Branch())))
	}
	keyW, refW = min(keyW, 50), min(refW, 30)
	return m.list.Render(grouped.View[caches.Cache]{
		Title: "caches",
		Row: func(c caches.Cache, _ bool) string {
			used := timefmt.Age(m.now().Sub(c.LastAccessedAt))
			return "   " + theme.Text().Render(pad(truncate(c.Key, keyW), keyW)) + "  " +
				theme.Accent().UnsetBold().Render(pad(truncate(c.Branch(), refW), refW)) +
				fmt.Sprintf("  %8s  ", size(c.SizeInBytes)) + theme.Muted().Render("used "+used+" ago")
		},
		Empty: "no caches",
		Header: func(repo string, cs []caches.Cache) string {
			if len(cs) == 0 {
				return ""
			}
			var total int64
			for _, c := range cs {
				total += c.SizeInBytes
			}
			noun := "caches"
			if len(cs) == 1 {
				noun = "cache"
			}
			return fmt.Sprintf("%d %s, %s", len(cs), noun, size(total))
		},
	})
}
