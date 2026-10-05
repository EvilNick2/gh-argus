package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EvilNick2/gh-argus/internal/mouse"
)

// doubleClick is how close together two clicks on one cell must be to count
// as a double click.
const doubleClick = 400 * time.Millisecond

type lastClick struct {
	x, y int
	at   time.Time
	set  bool
}

// Lines above a tab's body: the header bar and the tab line. The run, log
// and form screens have only the header bar.
const (
	tabLine    = 1
	tabBodyTop = 2
	screenTop  = 1
)

// tabLabel is how tab i reads on the tab line.
func tabLabel(i int) string {
	return fmt.Sprintf("[%d] %s", i+1, tabs[i])
}

// tabAt returns the tab whose label covers column x of the tab line, or -1.
// The line is a space, then the labels three spaces apart.
func tabAt(x int) int {
	start := 1
	for i := range tabs {
		w := len(tabLabel(i))
		if x >= start && x < start+w {
			return i
		}
		start += w + 3
	}
	return -1
}

// mouseMsg turns Bubble Tea mouse messages into mouse events, recognising
// double clicks, and treats the mouse back button as esc.
func (m Model) mouseMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		switch msg.Button {
		case tea.MouseBackward:
			return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		case tea.MouseLeft:
		default:
			return m, nil
		}
		kind, now := mouse.Click, m.deps.Now()
		last := m.last
		if last.set && last.x == msg.X && last.y == msg.Y && now.Sub(last.at) <= doubleClick {
			kind, m.last = mouse.DoubleClick, lastClick{}
		} else {
			m.last = lastClick{x: msg.X, y: msg.Y, at: now, set: true}
		}
		return m.mouse(mouse.Event{X: msg.X, Y: msg.Y, Kind: kind})
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.mouse(mouse.Event{X: msg.X, Y: msg.Y, Kind: mouse.WheelUp})
		case tea.MouseWheelDown:
			return m.mouse(mouse.Event{X: msg.X, Y: msg.Y, Kind: mouse.WheelDown})
		}
	}
	return m, nil
}

// mouse routes an event to the screen under it in that screen's own
// coordinates. A pending prompt takes only keys, and a click closes help.
func (m Model) mouse(ev mouse.Event) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		return m, nil
	}
	if m.help {
		if ev.Clicked() {
			m.help = false
		}
		m.scrollHelp(ev.Wheel())
		return m, nil
	}
	var cmd tea.Cmd
	switch m.screen {
	case screenPicker:
		m.picker, cmd = m.picker.Mouse(ev)
	case screenForm:
		m.form, cmd = m.form.Mouse(ev.Shift(0, screenTop))
	case screenRun:
		m.run, cmd = m.run.Mouse(ev.Shift(0, screenTop))
	case screenLog:
		m.log, cmd = m.log.Mouse(ev.Shift(0, screenTop))
	case screenTabs:
		if ev.Y == tabLine && ev.Clicked() {
			if t := tabAt(ev.X); t >= 0 {
				return m.key(tea.KeyPressMsg{Code: rune('1' + t), Text: string(rune('1' + t))})
			}
			return m, nil
		}
		body := ev.Shift(0, tabBodyTop)
		switch m.tab {
		case 0:
			m.runs, cmd = m.runs.Mouse(body)
		case 1:
			var activated bool
			if m.wfs, activated = m.wfs.Mouse(body); activated {
				return m.checkDispatch(false)
			}
		case 2:
			m.mets, cmd = m.mets.Mouse(body)
		case 3:
			m.cchs = m.cchs.Mouse(body)
		case 4:
			m.rnrs = m.rnrs.Mouse(body)
		}
	}
	return m, cmd
}
