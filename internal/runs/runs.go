// Package runs decodes GitHub Actions workflow runs and compares snapshots.
package runs

import (
	"encoding/json"
	"time"
)

type Run struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	DisplayTitle string    `json:"display_title"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadBranch   string    `json:"head_branch"`
	Event        string    `json:"event"`
	RunAttempt   int       `json:"run_attempt"`
	RunNumber    int       `json:"run_number"`
	WorkflowID   int64     `json:"workflow_id"`
	HTMLURL      string    `json:"html_url"`
	CreatedAt    time.Time `json:"created_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Change is a run that is new (Prev nil) or whose state moved since Prev.
type Change struct {
	Prev *Run
	Run  Run
}

// Decode parses a GET /repos/{owner}/{repo}/actions/runs response.
func Decode(body []byte) ([]Run, error) {
	var page struct {
		WorkflowRuns []Run `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.WorkflowRuns, nil
}

// Active reports whether any run has not completed.
func Active(rs []Run) bool {
	for _, r := range rs {
		if r.Status != "completed" {
			return true
		}
	}
	return false
}

// Diff returns changes in the order of next. Runs absent from next have
// only paged out, so they are not reported.
func Diff(prev, next []Run) []Change {
	old := make(map[int64]Run, len(prev))
	for _, r := range prev {
		old[r.ID] = r
	}
	var out []Change
	for _, r := range next {
		p, ok := old[r.ID]
		switch {
		case !ok:
			out = append(out, Change{Run: r})
		case p.Status != r.Status || p.Conclusion != r.Conclusion || p.RunAttempt != r.RunAttempt:
			out = append(out, Change{Prev: &p, Run: r})
		}
	}
	return out
}
