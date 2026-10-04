// Package grouped is a list of items grouped under each watched repo, with a
// cursor that moves between items and skips the repo headers. The Workflows,
// Runners and Cache tabs are built on it.
package grouped

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// View says how to draw a Model.
type View[T any] struct {
	Row func(item T, selected bool) string
	// Empty is shown under a repo that loaded with no items.
	Empty string
	// Error renders a failed load. Nil shows the error in red.
	Error func(err error) string
	// Header, if set, adds text after the repo name.
	Header func(repo string, items []T) string
}

type group[T any] struct {
	items  []T
	err    error
	loaded bool
}

// position is a selectable item.
type position struct {
	repo string
	idx  int
}

type Model[T any] struct {
	repos  []string
	groups map[string]*group[T]
	id     func(T) int64
	cursor int
	// moved is set once the user moves the cursor. Before that it stays on
	// the first item as repos load in whatever order they answer.
	moved         bool
	width, height int
}

// New makes an empty list for repos. id identifies an item across reloads.
func New[T any](repos []string, id func(T) int64) Model[T] {
	m := Model[T]{repos: repos, groups: map[string]*group[T]{}, id: id}
	for _, r := range repos {
		m.groups[r] = &group[T]{}
	}
	return m
}

func (m Model[T]) SetSize(w, h int) Model[T] {
	m.width, m.height = w, h
	return m
}

func (m Model[T]) positions() []position {
	var out []position
	for _, r := range m.repos {
		for i := range m.groups[r].items {
			out = append(out, position{r, i})
		}
	}
	return out
}

// Current returns the item under the cursor and its repo.
func (m Model[T]) Current() (string, T, bool) {
	ps := m.positions()
	if len(ps) == 0 {
		var zero T
		return "", zero, false
	}
	p := ps[min(m.cursor, len(ps)-1)]
	return p.repo, m.groups[p.repo].items[p.idx], true
}

// Load replaces a repo's items, or records the error loading them, keeping
// the cursor on the same item once the user has moved it.
func (m Model[T]) Load(repo string, items []T, err error) Model[T] {
	g, ok := m.groups[repo]
	if !ok {
		return m
	}
	curRepo, cur, had := m.Current()
	g.err = err
	if err == nil {
		g.items, g.loaded = items, true
	}
	if had && m.moved {
		for i, p := range m.positions() {
			if p.repo == curRepo && m.id(m.groups[p.repo].items[p.idx]) == m.id(cur) {
				m.cursor = i
			}
		}
	}
	return m
}

// Key handles a movement key: j, k, g, G or the arrows, home and end.
func (m Model[T]) Key(k string) Model[T] {
	n := len(m.positions())
	switch k {
	case "down", "j":
		m.cursor++
	case "up", "k":
		m.cursor--
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = n
	default:
		return m
	}
	m.moved = true
	m.cursor = max(0, min(m.cursor, n-1))
	return m
}

var (
	boldStyle = lipgloss.NewStyle().Bold(true)
	dimStyle  = lipgloss.NewStyle().Faint(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// Render draws the list, scrolled to keep the cursor visible in the height.
func (m Model[T]) Render(v View[T]) string {
	var rows []string
	cursorRow, i := 0, 0
	for _, r := range m.repos {
		g := m.groups[r]
		header := boldStyle.Render(r)
		if v.Header != nil && g.loaded {
			if extra := v.Header(r, g.items); extra != "" {
				header += "  " + dimStyle.Render(extra)
			}
		}
		rows = append(rows, header)
		switch {
		case g.err != nil && v.Error != nil:
			rows = append(rows, "  "+v.Error(g.err))
		case g.err != nil:
			rows = append(rows, "  "+errStyle.Render(g.err.Error()))
		case !g.loaded:
			rows = append(rows, dimStyle.Render("  loading"))
		case len(g.items) == 0:
			rows = append(rows, dimStyle.Render("  "+v.Empty))
		}
		for _, it := range g.items {
			if i == m.cursor {
				cursorRow = len(rows)
			}
			rows = append(rows, v.Row(it, i == m.cursor))
			i++
		}
	}

	h := max(1, m.height)
	offset := 0
	if cursorRow >= h {
		offset = cursorRow - h + 1
	}
	// On the last item, show the end of the list, where repos without items
	// have nothing to select, without scrolling the cursor away.
	if i > 0 && m.cursor == i-1 {
		offset = min(cursorRow, max(offset, len(rows)-h))
	}
	return strings.Join(rows[offset:min(len(rows), offset+h)], "\n")
}

// All returns every loaded item in display order.
func (m Model[T]) All() []T {
	var out []T
	for _, r := range m.repos {
		out = append(out, m.groups[r].items...)
	}
	return out
}
