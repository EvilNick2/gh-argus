// Package runners decodes a repository's self-hosted runners.
package runners

import "encoding/json"

type Label struct {
	Name string `json:"name"`
}

type Runner struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	OS     string  `json:"os"`
	Status string  `json:"status"` // online or offline
	Busy   bool    `json:"busy"`
	Labels []Label `json:"labels"`
}

// Decode parses a GET /repos/{owner}/{repo}/actions/runners response.
func Decode(body []byte) ([]Runner, error) {
	var page struct {
		Runners []Runner `json:"runners"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Runners, nil
}

func (r Runner) LabelNames() []string {
	names := make([]string, len(r.Labels))
	for i, l := range r.Labels {
		names[i] = l.Name
	}
	return names
}
