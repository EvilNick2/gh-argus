// Package dispatchform is the form for a workflow's workflow_dispatch inputs.
// Submitting it dispatches the workflow, so it is its own confirmation.
package dispatchform

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/workflows"
)

// SubmitMsg carries the inputs to dispatch with. Empty values are left out
// so GitHub applies the workflow's defaults.
type SubmitMsg struct {
	Inputs map[string]string
}

// CancelMsg closes the form without dispatching.
type CancelMsg struct{}

type field struct {
	in    workflows.Input
	value string
}

type Model struct {
	repo   string
	wf     workflows.Workflow
	ref    string
	fields []field
	cursor int
	err    string

	width, height int
}

func New(repo string, wf workflows.Workflow, ref string, inputs []workflows.Input) Model {
	m := Model{repo: repo, wf: wf, ref: ref}
	for _, in := range inputs {
		v := in.Default
		switch {
		case in.Type == "boolean" && v == "":
			v = "false"
		case in.Type == "choice" && v == "" && len(in.Options) > 0:
			v = in.Options[0]
		}
		m.fields = append(m.fields, field{in: in, value: v})
	}
	return m
}

func (m Model) SetSize(w, h int) Model {
	m.width, m.height = w, h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc":
		return m, func() tea.Msg { return CancelMsg{} }
	case "enter":
		return m.submit()
	}
	if len(m.fields) == 0 {
		return m, nil
	}
	switch k.String() {
	case "tab", "down":
		m.cursor = (m.cursor + 1) % len(m.fields)
		return m, nil
	case "shift+tab", "up":
		m.cursor = (m.cursor - 1 + len(m.fields)) % len(m.fields)
		return m, nil
	}

	f := &m.fields[m.cursor]
	m.err = ""
	switch f.in.Type {
	case "boolean":
		if k.String() == "space" {
			f.value = strconv.FormatBool(f.value != "true")
		}
	case "choice":
		opts := f.in.Options
		if len(opts) == 0 {
			break
		}
		i := max(0, slices.Index(opts, f.value))
		switch k.String() {
		case "right":
			f.value = opts[(i+1)%len(opts)]
		case "left":
			f.value = opts[(i-1+len(opts))%len(opts)]
		}
	default:
		switch {
		case k.String() == "backspace":
			if r := []rune(f.value); len(r) > 0 {
				f.value = string(r[:len(r)-1])
			}
		case k.Text != "":
			f.value += k.Text
		}
	}
	return m, nil
}

func (m Model) submit() (Model, tea.Cmd) {
	inputs := map[string]string{}
	for i, f := range m.fields {
		v := strings.TrimSpace(f.value)
		switch {
		case f.in.Required && v == "":
			m.err, m.cursor = f.in.Name+" is required", i
			return m, nil
		case f.in.Type == "number" && v != "":
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				m.err, m.cursor = f.in.Name+" must be a number", i
				return m, nil
			}
		}
		if v != "" {
			inputs[f.in.Name] = v
		}
	}
	return m, func() tea.Msg { return SubmitMsg{Inputs: inputs} }
}

var (
	boldStyle   = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	cursorStyle = lipgloss.NewStyle().Reverse(true)
)

func (m Model) View() string {
	lines := []string{
		boldStyle.Render(fmt.Sprintf("run %s on %s", m.wf.Name, m.ref)) + "  " + dimStyle.Render(m.repo),
		errStyle.Render(m.err),
	}
	if len(m.fields) == 0 {
		lines = append(lines, dimStyle.Render("no inputs"))
	}
	nameW := 0
	for _, f := range m.fields {
		nameW = max(nameW, len(f.in.Name)+1)
	}
	for i, f := range m.fields {
		name := f.in.Name
		if f.in.Required {
			name += "*"
		}
		name += strings.Repeat(" ", nameW-len(name))

		var value string
		switch f.in.Type {
		case "boolean":
			box := "[ ]"
			if f.value == "true" {
				box = "[x]"
			}
			value = box + " " + f.value
		case "choice":
			value = "< " + f.value + " >"
		default:
			value = f.value
			if i == m.cursor {
				value += "_"
			}
		}
		marker := "  "
		if i == m.cursor {
			marker, name = "> ", cursorStyle.Render(name)
		}
		lines = append(lines, marker+name+"  "+value)
		if f.in.Description != "" {
			for _, d := range strings.Split(ansi.Wordwrap(f.in.Description, max(10, m.width-4), " "), "\n") {
				lines = append(lines, "    "+dimStyle.Render(d))
			}
		}
	}
	return strings.Join(lines, "\n")
}
