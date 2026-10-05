package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/theme"
)

type helpSection struct {
	title string
	keys  [][2]string
}

var helpLeft = []helpSection{
	{"Everywhere", [][2]string{
		{"1-5", "switch tab"},
		{"j / k", "move down / up"},
		{"g / G", "top / bottom"},
		{"p", "pick repos to watch"},
		{"?", "show or hide these keys"},
		{"q", "quit"},
	}},
	{"Runs", [][2]string{
		{"tab", "switch between repos and runs"},
		{"enter", "open the run"},
		{"r", "rerun failed jobs"},
		{"R", "rerun all jobs"},
		{"c", "cancel the run"},
	}},
	{"Run", [][2]string{
		{"enter", "open the job's log"},
		{"esc", "back to runs"},
	}},
}

var helpRight = []helpSection{
	{"Log", [][2]string{
		{"/", "search"},
		{"n / N", "next / previous match"},
		{"w", "toggle word wrap"},
		{"r", "reload"},
		{"esc", "back to the run"},
	}},
	{"Workflows", [][2]string{
		{"enter", "run on the default branch"},
		{"i", "choose branch and inputs"},
		{"e / d", "enable / disable"},
	}},
	{"Cache", [][2]string{
		{"d", "delete the cache"},
	}},
}

func helpColumn(sections []helpSection) []string {
	var out []string
	for i, s := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, " "+theme.Bold().Render(s.title))
		for _, k := range s.keys {
			out = append(out, "   "+theme.Accent().UnsetBold().Render(pad(k[0], 8))+theme.Text().Render(k[1]))
		}
	}
	return out
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

// helpView is the key reference, in two columns inside a pane.
func helpView(width, height int) string {
	left, right := helpColumn(helpLeft), helpColumn(helpRight)
	colW := max(40, (width-2)/2)
	var rows []string
	for i := range max(len(left), len(right)) {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows = append(rows, pad(l, colW)+r)
	}
	return theme.Pane("keys", strings.Join(rows, "\n"), width, height, true)
}
