package theme

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func lines(s string) []string { return strings.Split(s, "\n") }

func TestPaneFillsExactSize(t *testing.T) {
	out := Pane("EvilNick2/dotfiles", "one\ntwo", 30, 6, false)

	ls := lines(out)
	if len(ls) != 6 {
		t.Fatalf("%d lines, want 6:\n%s", len(ls), ansi.Strip(out))
	}
	for i, l := range ls {
		if w := ansi.StringWidth(l); w != 30 {
			t.Errorf("line %d is %d wide, want 30: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestPaneTitleSitsInTopBorder(t *testing.T) {
	top := ansi.Strip(lines(Pane("repos", "", 20, 3, false))[0])

	if !strings.HasPrefix(top, "╭─ repos ─") || !strings.HasSuffix(top, "╮") {
		t.Errorf("top border %q", top)
	}
}

func TestPaneTruncatesLongBodyAndTitle(t *testing.T) {
	long := strings.Repeat("abcdefghij", 10)
	out := Pane(long, long+"\n"+long+"\nthree\nfour", 20, 4, false)

	ls := lines(out)
	if len(ls) != 4 {
		t.Fatalf("%d lines, want 4", len(ls))
	}
	for i, l := range ls {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
	if strings.Contains(ansi.Strip(out), "three") {
		t.Error("body taller than the pane was not cut")
	}
}

func TestPaneBorderColourShowsFocus(t *testing.T) {
	SetDark(true)
	focused := lines(Pane("x", "", 10, 3, true))[0]
	idle := lines(Pane("x", "", 10, 3, false))[0]

	if focused == idle {
		t.Error("focused and idle panes look the same")
	}
}

func TestIcons(t *testing.T) {
	cases := []struct{ status, conclusion, want string }{
		{"completed", "success", "+"},
		{"completed", "failure", "x"},
		{"completed", "timed_out", "x"},
		{"completed", "cancelled", "!"},
		{"completed", "skipped", "-"},
		{"in_progress", "", "*"},
		{"queued", "", "o"},
		{"waiting", "", "o"},
	}
	for _, c := range cases {
		if got := ansi.Strip(Icon(c.status, c.conclusion)); got != c.want {
			t.Errorf("Icon(%s, %s) = %q, want %q", c.status, c.conclusion, got, c.want)
		}
	}
}

func TestBarsFillWidthWithLeftAndRight(t *testing.T) {
	out := Bar("log loaded", "q quit", 40)

	if w := ansi.StringWidth(out); w != 40 {
		t.Errorf("bar is %d wide, want 40", w)
	}
	s := ansi.Strip(out)
	if !strings.Contains(s, "log loaded") || !strings.HasSuffix(strings.TrimRight(s, " "), "q quit") {
		t.Errorf("bar %q", s)
	}
}

func TestBarDropsRightSideWhenTooNarrow(t *testing.T) {
	out := Bar("a long status message here", "and a long list of hints", 30)

	if w := ansi.StringWidth(out); w != 30 {
		t.Errorf("bar is %d wide, want 30", w)
	}
	if !strings.Contains(ansi.Strip(out), "a long status message") {
		t.Errorf("left side lost: %q", ansi.Strip(out))
	}
}

func TestSelectedFillsWidth(t *testing.T) {
	if w := ansi.StringWidth(Selected("row", 25)); w != 25 {
		t.Errorf("selected row is %d wide, want 25", w)
	}
}

func TestLightAndDarkDiffer(t *testing.T) {
	SetDark(true)
	dark := Icon("completed", "success")
	SetDark(false)
	light := Icon("completed", "success")
	SetDark(true)

	if dark == light {
		t.Error("light and dark palettes render the same")
	}
}

func TestSelectedKeepsBackgroundAfterStyledPieces(t *testing.T) {
	out := Selected(Pass().Render("+")+" #16 main", 20)

	marker := strings.Split(Selected("\x00", 3), "\x00")[0]
	afterReset := out[strings.Index(out, "\x1b[m")+len("\x1b[m"):]
	if !strings.HasPrefix(afterReset, marker) {
		t.Errorf("background not restored after the icon's reset: %q", out)
	}
}

func TestStateWord(t *testing.T) {
	cases := []struct{ status, conclusion, want string }{
		{"in_progress", "", "running"},
		{"queued", "", "queued"},
		{"waiting", "", "queued"},
		{"completed", "success", "passed"},
		{"completed", "failure", "failed"},
		{"completed", "timed_out", "timed out"},
		{"completed", "startup_failure", "failed to start"},
		{"completed", "cancelled", "cancelled"},
		{"completed", "skipped", "skipped"},
	}
	for _, c := range cases {
		if got := StateWord(c.status, c.conclusion); got != c.want {
			t.Errorf("StateWord(%s, %s) = %q, want %q", c.status, c.conclusion, got, c.want)
		}
	}
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		x := float64(v) / 0xffff
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG contrast ratio between two colours.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// TestTextIsReadable checks every text colour reaches 4.5:1 on the
// terminal backgrounds each variant is meant for, on the bars and on a
// selected row, and that idle pane borders stay visible.
func TestTextIsReadable(t *testing.T) {
	variants := []struct {
		name     string
		p        Palette
		terminal []string
	}{
		// GitHub dark, Windows Terminal Campbell, and black.
		{"dark", darkPalette, []string{"#0D1117", "#0C0C0C", "#000000"}},
		// White, One Half Light, and Solarized Light.
		{"light", lightPalette, []string{"#FFFFFF", "#FAFAFA", "#FDF6E3"}},
	}
	for _, v := range variants {
		var surfaces []color.Color
		for _, hex := range v.terminal {
			surfaces = append(surfaces, lipgloss.Color(hex))
		}
		surfaces = append(surfaces, v.p.Bar, v.p.Selection)
		text := map[string]color.Color{"Plume": v.p.Plume, "Eye": v.p.Eye, "Pass": v.p.Pass, "Fail": v.p.Fail, "Text": v.p.Text, "Muted": v.p.Muted}
		for name, fg := range text {
			for _, bg := range surfaces {
				if c := contrast(fg, bg); c < 4.5 {
					t.Errorf("%s %s on %v: %.2f, want 4.5", v.name, name, bg, c)
				}
			}
		}
		for _, hex := range v.terminal {
			if c := contrast(v.p.Frame, lipgloss.Color(hex)); c < 1.8 {
				t.Errorf("%s Frame on %s: %.2f, want 1.8", v.name, hex, c)
			}
		}
	}
}
