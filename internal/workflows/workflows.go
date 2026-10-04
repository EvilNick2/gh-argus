// Package workflows decodes a repository's Actions workflows.
package workflows

import (
	"encoding/json"
	"strings"
)

type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// Decode parses a GET /repos/{owner}/{repo}/actions/workflows response.
func Decode(body []byte) ([]Workflow, error) {
	var page struct {
		Workflows []Workflow `json:"workflows"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Workflows, nil
}

// Dynamic reports a workflow GitHub generates, such as Pages builds. It has
// a synthetic path, no source file, and cannot be dispatched.
func (w Workflow) Dynamic() bool {
	return strings.HasPrefix(w.Path, "dynamic/")
}

func (w Workflow) Enabled() bool {
	return w.State == "active"
}
