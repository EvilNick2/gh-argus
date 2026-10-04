package workflows

import (
	"slices"
	"testing"
)

func TestParseDispatchForms(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want bool
	}{
		{"map with null trigger", "on:\n  push:\n  pull_request:\n  workflow_dispatch:\n", true},
		{"string", "on: workflow_dispatch\n", true},
		{"list", "on: [push, workflow_dispatch]\n", true},
		{"block list", "on:\n  - push\n  - workflow_dispatch\n", true},
		{"no dispatch map", "on:\n  push:\n    branches: [main]\n", false},
		{"no dispatch string", "on: push\n", false},
		{"no on key", "name: x\njobs: {}\n", false},
		{"quoted on key", "\"on\":\n  workflow_dispatch:\n", true},
	}
	for _, c := range cases {
		got, err := ParseDispatch([]byte(c.yaml))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.Dispatchable != c.want {
			t.Errorf("%s: Dispatchable = %v, want %v", c.name, got.Dispatchable, c.want)
		}
	}
}

func TestParseDispatchInputsInOrder(t *testing.T) {
	// Shapes from fonp/docker-publish.yml and orpheus/publish-embedder.yml.
	yaml := `
on:
  push:
    branches: [main, dev]
  workflow_dispatch:
    inputs:
      force:
        description: "Force build even if version unchanged (main only)"
        type: boolean
        default: false
      tag:
        description: 'Readable tag for this build'
        required: false
        type: string
      level:
        description: Log level
        required: true
        type: choice
        options: [info, debug]
        default: info
      note:
        required: true
`
	got, err := ParseDispatch([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Dispatchable || len(got.Inputs) != 4 {
		t.Fatalf("got %+v", got)
	}
	force, tag, level, note := got.Inputs[0], got.Inputs[1], got.Inputs[2], got.Inputs[3]
	if force.Name != "force" || force.Type != "boolean" || force.Default != "false" || force.Required ||
		force.Description != "Force build even if version unchanged (main only)" {
		t.Errorf("force %+v", force)
	}
	if tag.Name != "tag" || tag.Type != "string" || tag.Required || tag.Default != "" {
		t.Errorf("tag %+v", tag)
	}
	if level.Type != "choice" || !level.Required || level.Default != "info" || !slices.Equal(level.Options, []string{"info", "debug"}) {
		t.Errorf("level %+v", level)
	}
	if note.Type != "string" || !note.Required {
		t.Errorf("note %+v, want type defaulting to string", note)
	}
}

func TestNeedsInput(t *testing.T) {
	spec := DispatchSpec{Dispatchable: true, Inputs: []Input{
		{Name: "a", Required: false},
		{Name: "b", Required: true, Default: "x"},
	}}
	if spec.NeedsInput() {
		t.Error("inputs that are optional or have defaults reported as needed")
	}
	spec.Inputs = append(spec.Inputs, Input{Name: "c", Required: true})
	if !spec.NeedsInput() {
		t.Error("required input without default not reported")
	}
}

func TestParseDispatchInvalidYAML(t *testing.T) {
	if _, err := ParseDispatch([]byte("on: [unclosed\n")); err == nil {
		t.Error("invalid YAML parsed without error")
	}
}

func TestDecodeContent(t *testing.T) {
	// GitHub wraps the base64 at 60 columns with newlines.
	body := []byte(`{"encoding":"base64","content":"b246IHdvcmtmbG93X2Rp\nc3BhdGNoCg=="}`)

	got, err := DecodeContent(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "on: workflow_dispatch\n" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeContentRejectsOtherEncodings(t *testing.T) {
	if _, err := DecodeContent([]byte(`{"encoding":"none","content":""}`)); err == nil {
		t.Error("non-base64 encoding accepted")
	}
}
