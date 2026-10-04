package runs

import (
	"encoding/json"
	"time"
)

type Job struct {
	ID          int64     `json:"id"`
	RunID       int64     `json:"run_id"`
	RunAttempt  int       `json:"run_attempt"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	HTMLURL     string    `json:"html_url"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Steps       []Step    `json:"steps"`
}

type Step struct {
	Number      int       `json:"number"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// JobChange is a job that is new (Prev nil), whose state moved, or whose
// steps moved. Steps of a new job are not reported.
type JobChange struct {
	Prev  *Job
	Job   Job
	Steps []StepChange
}

type StepChange struct {
	Prev *Step
	Step Step
}

// DecodeJobs parses a GET /repos/{owner}/{repo}/actions/runs/{id}/jobs response.
func DecodeJobs(body []byte) ([]Job, error) {
	var page struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Jobs, nil
}

func DiffJobs(prev, next []Job) []JobChange {
	old := make(map[int64]Job, len(prev))
	for _, j := range prev {
		old[j.ID] = j
	}
	var out []JobChange
	for _, j := range next {
		p, ok := old[j.ID]
		if !ok {
			out = append(out, JobChange{Job: j})
			continue
		}
		steps := diffSteps(p.Steps, j.Steps)
		if p.Status != j.Status || p.Conclusion != j.Conclusion || len(steps) > 0 {
			out = append(out, JobChange{Prev: &p, Job: j, Steps: steps})
		}
	}
	return out
}

func diffSteps(prev, next []Step) []StepChange {
	old := make(map[int]Step, len(prev))
	for _, s := range prev {
		old[s.Number] = s
	}
	var out []StepChange
	for _, s := range next {
		p, ok := old[s.Number]
		switch {
		case !ok:
			out = append(out, StepChange{Step: s})
		case p.Status != s.Status || p.Conclusion != s.Conclusion:
			out = append(out, StepChange{Prev: &p, Step: s})
		}
	}
	return out
}
