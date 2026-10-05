// Package theme is the look of argus: a palette taken from a peacock's tail,
// where Hera set the hundred eyes of Argus, in a dark and a light variant,
// plus the panes, bars and status icons every screen is drawn with.
package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette is one variant of the theme's colours.
type Palette struct {
	Plume     color.Color // accent: focus, active tab, the name
	Eye       color.Color // running runs and search matches
	Pass      color.Color
	Fail      color.Color
	Text      color.Color
	Muted     color.Color // secondary text such as ages and paths
	Frame     color.Color // idle pane borders
	Bar       color.Color // header and status bar background
	Selection color.Color // selected row background
}

var darkPalette = Palette{
	Plume:     lipgloss.Color("#2AA7B8"),
	Eye:       lipgloss.Color("#E0B341"),
	Pass:      lipgloss.Color("#3FB97F"),
	Fail:      lipgloss.Color("#E5534B"),
	Text:      lipgloss.Color("#E6EDF3"),
	Muted:     lipgloss.Color("#8B95A1"),
	Frame:     lipgloss.Color("#3A4450"),
	Bar:       lipgloss.Color("#1C232B"),
	Selection: lipgloss.Color("#24303A"),
}

var lightPalette = Palette{
	Plume:     lipgloss.Color("#12798A"),
	Eye:       lipgloss.Color("#A67C10"),
	Pass:      lipgloss.Color("#1A7F4E"),
	Fail:      lipgloss.Color("#C0362C"),
	Text:      lipgloss.Color("#1F2328"),
	Muted:     lipgloss.Color("#6A737D"),
	Frame:     lipgloss.Color("#C9D1D9"),
	Bar:       lipgloss.Color("#EEF1F4"),
	Selection: lipgloss.Color("#DDE7EC"),
}

var p = darkPalette

// SetDark picks the variant for the terminal's background. Dark is the
// default until the terminal answers.
func SetDark(isDark bool) {
	if isDark {
		p = darkPalette
	} else {
		p = lightPalette
	}
}

// Current returns the palette in use.
func Current() Palette {
	return p
}

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func Accent() lipgloss.Style { return fg(p.Plume).Bold(true) }
func Gold() lipgloss.Style   { return fg(p.Eye) }
func Pass() lipgloss.Style   { return fg(p.Pass) }
func Fail() lipgloss.Style   { return fg(p.Fail) }
func Text() lipgloss.Style   { return fg(p.Text) }
func Bold() lipgloss.Style   { return fg(p.Text).Bold(true) }
func Muted() lipgloss.Style  { return fg(p.Muted) }

// Match highlights search matches.
func Match() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(p.Bar).Background(p.Eye).Bold(true)
}

// Icon is a run, job or step state as one ASCII glyph, so it reads without
// colour: + passed, x failed, * running, o queued, ! cancelled, - skipped.
func Icon(status, conclusion string) string {
	switch status {
	case "completed":
	case "in_progress":
		return Gold().Render("*")
	default:
		return Muted().Render("o")
	}
	switch conclusion {
	case "success":
		return Pass().Render("+")
	case "failure", "timed_out", "startup_failure":
		return Fail().Render("x")
	case "cancelled":
		return Muted().Render("!")
	case "skipped":
		return Muted().Render("-")
	}
	return Muted().Render("?")
}

// Legend explains the icons, for the status bar.
func Legend() string {
	return Pass().Render("+") + Muted().Render(" pass  ") +
		Fail().Render("x") + Muted().Render(" fail  ") +
		Gold().Render("*") + Muted().Render(" run  ") +
		Muted().Render("o queued")
}

// Pane draws body in a rounded border of exactly width by height with title
// set into the top edge. Body lines are cut to fit. The border takes the
// accent colour when focused.
func Pane(title, body string, width, height int, focused bool) string {
	width, height = max(width, 4), max(height, 2)
	inner := width - 2
	edge, titleStyle := fg(p.Frame), Bold()
	if focused {
		edge, titleStyle = fg(p.Plume), Accent()
	}

	var top string
	if t := ansi.Truncate(title, inner-3, ".."); t == "" {
		top = edge.Render("╭" + strings.Repeat("─", inner) + "╮")
	} else {
		fill := inner - 3 - ansi.StringWidth(t)
		top = edge.Render("╭─ ") + titleStyle.Render(t) + edge.Render(" "+strings.Repeat("─", fill)+"╮")
	}

	out := []string{top}
	body = strings.TrimSuffix(body, "\n")
	var rows []string
	if body != "" {
		rows = strings.Split(body, "\n")
	}
	side := edge.Render("│")
	for i := range height - 2 {
		l := ""
		if i < len(rows) {
			l = ansi.Truncate(rows[i], inner, "")
		}
		out = append(out, side+l+strings.Repeat(" ", inner-ansi.StringWidth(l))+side)
	}
	out = append(out, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(out, "\n")
}

// strip paints s across width on background bg. Styled pieces inside s end
// with a reset, so bg is put back after each one.
func strip(s string, width int, bg color.Color) string {
	marker := lipgloss.NewStyle().Background(bg).Render("\x00")
	on := marker[:strings.IndexByte(marker, 0)]
	s = ansi.Truncate(s, width, "")
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+on)
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+on)
	return on + s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s))) + "\x1b[m"
}

// Selected highlights a row across the full width.
func Selected(s string, width int) string {
	return strip(s, width, p.Selection)
}

// Bar puts left and right at either end of a strip, for the header and
// status bars. Right is dropped when both do not fit.
func Bar(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return strip(left, width, p.Bar)
	}
	return strip(left+strings.Repeat(" ", gap)+right, width, p.Bar)
}

// StateWord names a run, job or step state in plain words for headers.
func StateWord(status, conclusion string) string {
	switch status {
	case "completed":
	case "in_progress":
		return "running"
	default:
		return "queued"
	}
	switch conclusion {
	case "success":
		return "passed"
	case "failure":
		return "failed"
	case "startup_failure":
		return "failed to start"
	case "":
		return "completed"
	}
	return strings.ReplaceAll(conclusion, "_", " ")
}
