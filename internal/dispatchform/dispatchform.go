// Package dispatchform is the form for a workflow's workflow_dispatch inputs.
// Submitting it dispatches the workflow, so it is its own confirmation.
package dispatchform

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/theme"
	"github.com/EvilNick2/gh-argus/internal/workflows"
)

// SubmitMsg carries the inputs to dispatch with. Empty values are left out
// so GitHub applies the workflow's defaults.
type SubmitMsg struct {
	Ref    string
	Inputs map[string]string
}

// CancelMsg closes the form without dispatching.
type CancelMsg struct{}

type field struct {
	in    workflows.Input
	value string
}

// branchType marks field 0, the ref to dispatch on. It is not one of the
// input types a workflow file can declare.
const branchType = "branch"

type Model struct {
	repo     string
	wf       workflows.Workflow
	branches []string
	// fields[0] is the branch, the workflow's inputs follow.
	fields []field
	cursor int
	err    string

	width, height int
}

// New opens the form on ref with the cursor on the first input, or on the
// branch when there are no inputs. Left and right cycle the branch through
// branches. Any ref can be typed, tags included.
func New(repo string, wf workflows.Workflow, ref string, branches []string, inputs []workflows.Input) Model {
	m := Model{repo: repo, wf: wf, branches: branches}
	m.fields = append(m.fields, field{in: workflows.Input{Name: "branch", Type: branchType, Required: true}, value: ref})
	if len(inputs) > 0 {
		m.cursor = 1
	}
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
	case "choice", branchType:
		opts := f.in.Options
		if f.in.Type == branchType {
			opts = m.branches
		}
		if len(opts) == 0 || k.String() != "left" && k.String() != "right" {
			if f.in.Type == branchType {
				editText(f, k)
			}
			break
		}
		// From a value not in the list, right goes to the first option and
		// left to the last.
		i := slices.Index(opts, f.value)
		switch {
		case k.String() == "right" && i < 0:
			f.value = opts[0]
		case k.String() == "left" && i < 0:
			f.value = opts[len(opts)-1]
		case k.String() == "right":
			f.value = opts[(i+1)%len(opts)]
		default:
			f.value = opts[(i-1+len(opts))%len(opts)]
		}
	default:
		editText(f, k)
	}
	return m, nil
}

func editText(f *field, k tea.KeyPressMsg) {
	switch {
	case k.String() == "backspace":
		if r := []rune(f.value); len(r) > 0 {
			f.value = string(r[:len(r)-1])
		}
	case k.Text != "":
		f.value += k.Text
	}
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
		if v != "" && i > 0 {
			inputs[f.in.Name] = v
		}
	}
	ref := strings.TrimSpace(m.fields[0].value)
	return m, func() tea.Msg { return SubmitMsg{Ref: ref, Inputs: inputs} }
}

func (m Model) View() string {
	inner := max(10, m.width-2)
	lines := []string{" " + theme.Muted().Render(m.repo)}
	if m.err != "" {
		lines = append(lines, " "+theme.Fail().Render(m.err))
	} else {
		lines = append(lines, "")
	}
	nameW := 0
	for _, f := range m.fields {
		nameW = max(nameW, len(f.in.Name)+1)
	}
	for i, f := range m.fields {
		name := f.in.Name
		if f.in.Required && f.in.Type != branchType {
			name += "*"
		}
		name += strings.Repeat(" ", nameW-len(name))

		var value string
		switch f.in.Type {
		case "boolean":
			box := "[ ]"
			if f.value == "true" {
				box = theme.Accent().UnsetBold().Render("[x]")
			}
			value = box + " " + f.value
		case "choice":
			value = theme.Muted().Render("< ") + f.value + theme.Muted().Render(" >")
		default:
			value = f.value
			if i == m.cursor {
				value += theme.Accent().Render("_")
			}
			if f.in.Type == branchType && i == m.cursor && len(m.branches) > 0 {
				value += theme.Muted().Render(fmt.Sprintf("  left/right picks from %d branches", len(m.branches)))
			}
		}
		row := " " + theme.Bold().Render(name) + "  " + theme.Text().Render(value)
		if i == m.cursor {
			row = theme.Selected(row, inner)
		}
		lines = append(lines, row)
		if f.in.Description != "" {
			for _, d := range strings.Split(ansi.Wordwrap(f.in.Description, max(10, inner-4), " "), "\n") {
				lines = append(lines, "   "+theme.Muted().Render(d))
			}
		}
	}
	if len(m.fields) == 1 {
		lines = append(lines, "", " "+theme.Muted().Render("no inputs"))
	}
	title := fmt.Sprintf("run %s on %s", m.wf.Name, m.fields[0].value)
	return theme.Pane(title, strings.Join(lines, "\n"), m.width, m.height, true)
}
