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
		{"C", "force cancel a stuck run"},
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
	{"Workflows, Metrics, Cache, Runners", [][2]string{
		{"r", "refresh"},
	}},
	{"Mouse", [][2]string{
		{"click", "select, or switch tab"},
		{"double-click", "open, run or tick"},
		{"wheel", "move the selection, scroll a log"},
		{"back button", "go back"},
		{"shift+drag", "select text to copy"},
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
			out = append(out, "   "+theme.Accent().UnsetBold().Render(pad(k[0], 13))+theme.Text().Render(k[1]))
		}
	}
	return out
}

func pad(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}

// helpRows lays out the key reference in two columns.
func helpRows(width int) []string {
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
	return rows
}

// helpMaxOffset is the furthest the key reference scrolls in a pane of
// width by height.
func helpMaxOffset(width, height int) int {
	return max(0, len(helpRows(width))-(height-2))
}

// helpView is the key reference inside a pane, scrolled down offset rows.
func helpView(width, height, offset int) string {
	rows := helpRows(width)
	offset = max(0, min(offset, helpMaxOffset(width, height)))
	return theme.Pane("keys", strings.Join(rows[offset:], "\n"), width, height, true)
}
