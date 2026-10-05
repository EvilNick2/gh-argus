package dispatchform

import (
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/workflows"
)

var wf = workflows.Workflow{ID: 9, Name: "Publish embedder"}

var inputs = []workflows.Input{
	{Name: "tag", Description: "Readable tag for this build", Type: "string"},
	{Name: "force", Description: "Force build", Type: "boolean", Default: "false"},
	{Name: "level", Type: "choice", Options: []string{"info", "debug", "trace"}, Default: "info"},
	{Name: "count", Type: "number", Default: "1"},
}

var branches = []string{"dev", "main", "release"}

func newModel(ins []workflows.Input) Model {
	return New("o/r", wf, "main", branches, ins).SetSize(80, 20)
}

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func typed(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, key(string(r)))
	}
	return out
}

// send applies msgs and returns the model and the last command.
func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func view(m Model) string { return ansi.Strip(m.View()) }

// submit runs cmd and returns the SubmitMsg it sends.
func submit(t *testing.T, cmd tea.Cmd) SubmitMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	msg, ok := cmd().(SubmitMsg)
	if !ok {
		t.Fatalf("got %#v, want SubmitMsg", cmd())
	}
	return msg
}

// submitted runs cmd and returns the inputs of the SubmitMsg it sends.
func submitted(t *testing.T, cmd tea.Cmd) map[string]string {
	t.Helper()
	return submit(t, cmd).Inputs
}

func TestShowsFieldsWithDefaultsAndDescriptions(t *testing.T) {
	v := view(newModel(inputs))

	for _, want := range []string{"Publish embedder", "main", "tag", "Readable tag for this build", "force", "false", "level", "info", "count", "1"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestEnterSubmitsDefaults(t *testing.T) {
	_, cmd := send(newModel(inputs), key("enter"))

	got := submitted(t, cmd)
	want := map[string]string{"force": "false", "level": "info", "count": "1"}
	if !maps.Equal(got, want) {
		t.Errorf("inputs %v, want %v with empty optional tag left out", got, want)
	}
}

func TestTypingEditsTextField(t *testing.T) {
	msgs := append(typed("clap-qj"), key("backspace"), key("enter"))
	_, cmd := send(newModel(inputs), msgs...)

	if got := submitted(t, cmd)["tag"]; got != "clap-q" {
		t.Errorf("tag %q, want clap-q (j and q typed as text, backspace removes j)", got)
	}
}

func TestNavigationAndToggles(t *testing.T) {
	m := newModel(inputs)

	m, _ = send(m, key("tab"), key("space"))                // force -> true
	m, _ = send(m, key("down"), key("right"), key("right")) // level -> trace
	m, _ = send(m, key("right"))                            // wraps -> info
	m, _ = send(m, key("left"))                             // back -> trace
	m, _ = send(m, key("shift+tab"), key("up"))             // back on tag
	m, cmd := send(m, append(typed("x"), key("enter"))...)

	got := submitted(t, cmd)
	if got["force"] != "true" || got["level"] != "trace" || got["tag"] != "x" {
		t.Errorf("inputs %v", got)
	}
}

func TestRequiredFieldBlocksSubmit(t *testing.T) {
	ins := []workflows.Input{{Name: "version", Type: "string", Required: true}}
	m := newModel(ins)

	m, cmd := send(m, key("enter"))
	if cmd != nil {
		t.Fatal("submitted with a required field empty")
	}
	if v := view(m); !strings.Contains(v, "version is required") {
		t.Errorf("view:\n%s", v)
	}
	_, cmd = send(m, append(typed("1.2.3"), key("enter"))...)
	if got := submitted(t, cmd)["version"]; got != "1.2.3" {
		t.Errorf("version %q", got)
	}
}

func TestNumberFieldValidated(t *testing.T) {
	m := newModel([]workflows.Input{{Name: "count", Type: "number", Default: "1"}})

	m, cmd := send(m, append(typed("x"), key("enter"))...)
	if cmd != nil {
		t.Fatal("submitted a non-number")
	}
	if v := view(m); !strings.Contains(v, "count must be a number") {
		t.Errorf("view:\n%s", v)
	}
	_, cmd = send(m, key("backspace"), key("enter"))
	if got := submitted(t, cmd)["count"]; got != "1" {
		t.Errorf("count %q", got)
	}
}

func TestSpaceTypesInTextFields(t *testing.T) {
	_, cmd := send(newModel(inputs), append(typed("a b"), key("enter"))...)

	if got := submitted(t, cmd)["tag"]; got != "a b" {
		t.Errorf("tag %q", got)
	}
}

func TestEscCancels(t *testing.T) {
	_, cmd := send(newModel(inputs), key("esc"))

	if cmd == nil {
		t.Fatal("no command")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Error("esc did not send CancelMsg")
	}
}

func TestLongDescriptionsWrapToWidth(t *testing.T) {
	long := "Readable tag for this build, e.g. clap-music-1. It is what EMBEDDER_VERSION pins, so name the model, not the repo version."
	m := New("o/r", wf, "main", nil, []workflows.Input{{Name: "tag", Description: long}}).SetSize(50, 20)

	v := view(m)
	for _, l := range strings.Split(v, "\n") {
		if ansi.StringWidth(l) > 50 {
			t.Errorf("line wider than 50: %q", l)
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(v), " "), "not the repo version.") {
		t.Errorf("end of description lost:\n%s", v)
	}
}

func TestBranchDefaultsToGivenRef(t *testing.T) {
	m := newModel(inputs)

	if v := view(m); !strings.Contains(v, "branch") || !strings.Contains(v, "main") {
		t.Errorf("view:\n%s", v)
	}
	_, cmd := send(m, key("enter"))
	if got := submit(t, cmd).Ref; got != "main" {
		t.Errorf("Ref %q, want main", got)
	}
}

func TestCursorStartsOnFirstInputThenBranchIsAbove(t *testing.T) {
	m := newModel(inputs)

	// shift+tab from the first input reaches the branch field.
	m, _ = send(m, key("shift+tab"))
	m, _ = send(m, key("backspace"), key("backspace"), key("backspace"), key("backspace"))
	_, cmd := send(m, append(typed("v1.0.0"), key("enter"))...)
	got := submit(t, cmd)
	if got.Ref != "v1.0.0" || got.Inputs["tag"] != "" {
		t.Errorf("got %+v, want ref v1.0.0 typed into the branch field", got)
	}
}

func TestNoInputsStartsOnBranch(t *testing.T) {
	m := newModel(nil)

	_, cmd := send(m, key("right"), key("enter"))
	if got := submit(t, cmd).Ref; got != "release" {
		t.Errorf("Ref %q, want release, the branch after main", got)
	}
}

func TestLeftRightCycleBranches(t *testing.T) {
	m := newModel(nil)

	m, _ = send(m, key("left"))
	_, cmd := send(m, key("enter"))
	if got := submit(t, cmd).Ref; got != "dev" {
		t.Errorf("left from main: %q, want dev", got)
	}
	m, _ = send(m, key("left"))
	_, cmd = send(m, key("enter"))
	if got := submit(t, cmd).Ref; got != "release" {
		t.Errorf("left once more wraps to %q, want release", got)
	}
}

func TestTypedBranchCyclesFromFirst(t *testing.T) {
	m := newModel(nil)

	for range 4 {
		m, _ = send(m, key("backspace"))
	}
	m, _ = send(m, append(typed("feat"), key("right"))...)
	_, cmd := send(m, key("enter"))
	if got := submit(t, cmd).Ref; got != "dev" {
		t.Errorf("right from an unlisted name: %q, want the first branch", got)
	}
}

func TestEmptyBranchBlocksSubmit(t *testing.T) {
	m := newModel(nil)

	for range 4 {
		m, _ = send(m, key("backspace"))
	}
	m, cmd := send(m, key("enter"))
	if cmd != nil {
		t.Fatal("submitted with no branch")
	}
	if v := view(m); !strings.Contains(v, "branch is required") {
		t.Errorf("view:\n%s", v)
	}
}

func TestFormIsOnePaneOfFullSize(t *testing.T) {
	lines := strings.Split(view(newModel(inputs)), "\n")

	if len(lines) != 20 {
		t.Fatalf("%d lines, want 20", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 80 {
			t.Errorf("line %d is %d wide, want 80: %q", i, w, l)
		}
	}
	if !strings.Contains(lines[0], "run Publish embedder on main") {
		t.Errorf("title %q", lines[0])
	}
}
