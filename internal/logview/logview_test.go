package logview

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/mouse"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/theme"
)

var job = runs.Job{ID: 70, Name: "check", Status: "completed", Conclusion: "failure"}

// numbered returns n plain lines "line 0" to "line n-1".
func numbered(n int) []joblog.Line {
	var out []joblog.Line
	for i := range n {
		out = append(out, joblog.Line{Text: fmt.Sprintf("line %d", i)})
	}
	return out
}

func newModel(lines []joblog.Line, w, h int) Model {
	m := New("EvilNick2/dotfiles", job).SetSize(w, h)
	m, _ = m.Update(LogMsg{Lines: lines})
	return m
}

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func typed(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, key(string(r)))
	}
	return out
}

func view(m Model) string { return ansi.Strip(m.View()) }

// content is a pane row without its side borders and the one space of
// padding inside them.
func content(l string) string {
	l = strings.TrimPrefix(l, "│ ")
	l = strings.TrimSuffix(l, "│")
	return strings.TrimRight(l, " ")
}

// shows reports whether the exact log line text appears as a row.
func shows(v, text string) bool {
	for _, l := range strings.Split(v, "\n") {
		if content(l) == text {
			return true
		}
	}
	return false
}

func TestLoadingUntilLogArrives(t *testing.T) {
	m := New("o/r", job).SetSize(80, 10)

	if v := view(m); !strings.Contains(v, "loading log") {
		t.Errorf("view:\n%s", v)
	}
}

func TestHeaderNamesRepoAndJob(t *testing.T) {
	l := strings.Join(strings.Split(view(newModel(numbered(3), 80, 10)), "\n")[:2], " ")

	for _, want := range []string{"EvilNick2/dotfiles", "check", "x failed"} {
		if !strings.Contains(l, want) {
			t.Errorf("header %q missing %q", l, want)
		}
	}
}

func TestOpensAtEndWithoutErrors(t *testing.T) {
	v := view(newModel(numbered(50), 80, 10))

	if !shows(v, "line 49") || shows(v, "line 0") {
		t.Errorf("not at end:\n%s", v)
	}
}

func TestOpensAtFirstErrorWithContext(t *testing.T) {
	lines := numbered(50)
	lines[30] = joblog.Line{Kind: joblog.Error, Text: "Process completed with exit code 1."}
	lines[40] = joblog.Line{Kind: joblog.Error, Text: "second error"}

	v := view(newModel(lines, 80, 10))
	if !strings.Contains(v, "Error: Process completed with exit code 1.") {
		t.Errorf("first error not shown:\n%s", v)
	}
	if !shows(v, "line 28") || shows(v, "line 49") {
		t.Errorf("want context above the error and not the end:\n%s", v)
	}
}

func TestMarkersRenderLikeGitHub(t *testing.T) {
	lines := []joblog.Line{
		{Kind: joblog.Group, Text: "Run actions/checkout@v4"},
		{Kind: joblog.Warning, Text: "Node 16 is deprecated"},
		{Kind: joblog.Command, Text: "/usr/bin/git version"},
	}

	v := view(newModel(lines, 80, 10))
	for _, want := range []string{"> Run actions/checkout@v4", "Warning: Node 16 is deprecated", "/usr/bin/git version"} {
		if !shows(v, want) {
			t.Errorf("missing row %q in:\n%s", want, v)
		}
	}
}

func TestScrollKeys(t *testing.T) {
	m := newModel(numbered(50), 80, 10)

	m = send(m, key("g"))
	if v := view(m); !shows(v, "line 0") {
		t.Errorf("g did not go to top:\n%s", v)
	}
	m = send(m, key("j"), key("j"))
	if v := view(m); shows(v, "line 1") || !shows(v, "line 2") {
		t.Errorf("j twice from top:\n%s", v)
	}
	m = send(m, key("k"))
	if v := view(m); !shows(v, "line 1") {
		t.Errorf("k:\n%s", v)
	}
	m = send(m, key("pgdown"))
	if v := view(m); shows(v, "line 1") {
		t.Errorf("pgdown did not move a page:\n%s", v)
	}
	m = send(m, key("G"))
	if v := view(m); !shows(v, "line 49") {
		t.Errorf("G did not go to end:\n%s", v)
	}
}

func TestScrollStopsAtEnds(t *testing.T) {
	m := newModel(numbered(50), 80, 10)

	m = send(m, key("j"), key("j"))
	if v := view(m); !shows(v, "line 49") {
		t.Errorf("scrolled past the end:\n%s", v)
	}
	m = send(m, key("g"), key("k"))
	if v := view(m); !shows(v, "line 0") {
		t.Errorf("scrolled above the top:\n%s", v)
	}
}

func TestWrapTogglesLongLines(t *testing.T) {
	long := strings.Repeat("abcdefghij", 12) // 120 columns
	m := newModel([]joblog.Line{{Text: long}, {Text: "after"}}, 40, 10)

	v := view(m)
	if !strings.Contains(v, strings.Repeat("abcdefghij", 3)) || !strings.Contains(v, "after") {
		t.Errorf("wrapped view:\n%s", v)
	}
	for _, l := range strings.Split(v, "\n") {
		if ansi.StringWidth(l) > 40 {
			t.Errorf("row wider than 40: %q", l)
		}
	}

	m = send(m, key("w"))
	v = view(m)
	rows := 0
	for _, l := range strings.Split(v, "\n") {
		if c := content(l); strings.HasPrefix(c, "abcdefghij") {
			rows++
			if !strings.HasSuffix(c, "..") || ansi.StringWidth(l) > 40 {
				t.Errorf("unwrapped row %q, want truncated to the pane with ..", l)
			}
		}
	}
	if rows != 1 {
		t.Errorf("long line drawn as %d rows with wrap off", rows)
	}
}

func TestSearchJumpsBetweenMatches(t *testing.T) {
	lines := numbered(100)
	lines[20].Text = "first NEEDLE here"
	lines[70].Text = "second needle here"

	m := newModel(lines, 80, 10)
	m = send(m, key("g"))
	m = send(m, append(append([]tea.Msg{key("/")}, typed("needle")...), key("enter"))...)
	v := view(m)
	if !shows(v, "first NEEDLE here") || !strings.Contains(v, "1/2") {
		t.Errorf("after search:\n%s", v)
	}
	m = send(m, key("n"))
	v = view(m)
	if !shows(v, "second needle here") || shows(v, "first NEEDLE here") || !strings.Contains(v, "2/2") {
		t.Errorf("after n:\n%s", v)
	}
	m = send(m, key("n"))
	if v := view(m); !strings.Contains(v, "1/2") {
		t.Errorf("n did not wrap around:\n%s", v)
	}
	m = send(m, key("N"))
	if v := view(m); !strings.Contains(v, "2/2") {
		t.Errorf("N did not go back:\n%s", v)
	}
}

func TestSearchWithNoMatchesSaysSo(t *testing.T) {
	m := newModel(numbered(10), 80, 10)

	m = send(m, append(append([]tea.Msg{key("/")}, typed("zzz")...), key("enter"))...)
	if v := view(m); !strings.Contains(v, "no matches") {
		t.Errorf("view:\n%s", v)
	}
}

func TestSearchModeTreatsKeysAsText(t *testing.T) {
	lines := numbered(10)
	lines[3].Text = "quit with g"

	m := newModel(lines, 80, 10)
	m = send(m, append(append([]tea.Msg{key("/")}, typed("quit wix")...), key("backspace"), key("t"), key("h"), key("enter"))...)
	if v := view(m); !strings.Contains(v, "1/1") {
		t.Errorf("search for %q failed:\n%s", "quit with", v)
	}
}

func TestEscInSearchCancelsAndEscOutsideGoesBack(t *testing.T) {
	m := newModel(numbered(10), 80, 10)

	m = send(m, key("/"), key("x"))
	m, cmd := m.Update(key("esc"))
	if cmd != nil {
		t.Error("esc in search mode left the log view")
	}
	_, cmd = m.Update(key("esc"))
	if cmd == nil {
		t.Fatal("esc returned no command")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Error("esc did not send BackMsg")
	}
}

func TestRSendsReload(t *testing.T) {
	m := newModel(numbered(3), 80, 10)

	m, cmd := m.Update(key("r"))
	if cmd == nil {
		t.Fatal("no command")
	}
	if _, ok := cmd().(ReloadMsg); !ok {
		t.Error("r did not send ReloadMsg")
	}
	if v := view(m); !strings.Contains(v, "loading log") {
		t.Errorf("not loading after r:\n%s", v)
	}
}

func TestErrorShown(t *testing.T) {
	m := New("o/r", job).SetSize(80, 10)

	m = send(m, LogMsg{Err: errors.New("fetching log: 404 Not Found")})
	if v := view(m); !strings.Contains(v, "404") {
		t.Errorf("view:\n%s", v)
	}
}

func TestViewFitsHeight(t *testing.T) {
	v := view(newModel(numbered(100), 80, 10))

	if n := strings.Count(v, "\n") + 1; n > 10 {
		t.Errorf("view is %d lines, taller than 10", n)
	}
}

func TestMatchesAreHighlighted(t *testing.T) {
	lines := numbered(5)
	lines[2].Text = "find the needle"

	m := newModel(lines, 80, 10)
	m = send(m, append(append([]tea.Msg{key("/")}, typed("NEEDLE")...), key("enter"))...)
	raw := m.View()
	if !strings.Contains(raw, theme.Match().Render("needle")) {
		t.Errorf("match not highlighted:\n%q", raw)
	}
}

func TestLogIsOnePaneOfFullSize(t *testing.T) {
	lines := strings.Split(view(newModel(numbered(50), 60, 12)), "\n")

	if len(lines) != 12 {
		t.Fatalf("%d lines, want 12", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("line %d is %d wide, want 60: %q", i, w, l)
		}
	}
}

func TestEndStartsOnALineBoundary(t *testing.T) {
	// Three-row wrapped lines at width 23 (20 inside the pane, less padding).
	var lines []joblog.Line
	for i := range 10 {
		lines = append(lines, joblog.Line{Text: fmt.Sprintf("line%d ", i) + strings.Repeat("x", 45)})
	}
	m := newModel(lines, 23, 10) // 7 rows of log fit, so the end falls mid-line

	v := view(m)
	first := content(strings.Split(v, "\n")[2])
	if !strings.HasPrefix(first, "line") {
		t.Errorf("top row %q starts mid-line:\n%s", first, v)
	}
	if !strings.Contains(v, "line9") {
		t.Errorf("last line not shown:\n%s", v)
	}
}

func TestWheelScrollsThreeLines(t *testing.T) {
	m := newModel(numbered(50), 80, 10)
	m = send(m, key("g"))

	m, _ = m.Mouse(mouse.Event{X: 5, Y: 5, Kind: mouse.WheelDown})
	if v := view(m); !shows(v, "line 3") || shows(v, "line 2") {
		t.Errorf("wheel down:\n%s", v)
	}
	m, _ = m.Mouse(mouse.Event{X: 5, Y: 5, Kind: mouse.WheelUp})
	if v := view(m); !shows(v, "line 0") {
		t.Errorf("wheel up:\n%s", v)
	}
}
