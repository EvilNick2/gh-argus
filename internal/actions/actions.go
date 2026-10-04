// Package actions sends the run actions: rerun failed jobs, rerun all jobs
// and cancel.
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/EvilNick2/gh-argus/internal/runs"
)

type Kind int

const (
	RerunFailed Kind = iota
	RerunAll
	Cancel
)

func (k Kind) String() string {
	return [...]string{"rerun failed jobs", "rerun all jobs", "cancel"}[k]
}

func (k Kind) endpoint() string {
	return [...]string{"rerun-failed-jobs", "rerun", "cancel"}[k]
}

// Allowed reports whether k makes sense for r: cancel while it is unfinished,
// rerun all once it has completed, rerun failed once it has completed
// without succeeding.
func (k Kind) Allowed(r runs.Run) bool {
	done := r.Status == "completed"
	switch k {
	case Cancel:
		return !done
	case RerunAll:
		return done
	case RerunFailed:
		return done && r.Conclusion != "success"
	}
	return false
}

type StatusError struct {
	StatusCode int
	Message    string
}

func (e *StatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}
	return fmt.Sprintf("%d %s", e.StatusCode, e.Message)
}

// Do sends action k for run id of repo. It needs a token with write access,
// which collaborators may not have.
func Do(ctx context.Context, client *http.Client, baseURL, repo string, id int64, k Kind) error {
	url := fmt.Sprintf("%s/repos/%s/actions/runs/%d/%s", strings.TrimSuffix(baseURL, "/"), repo, id, k.endpoint())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	var apiErr struct{ Message string }
	json.Unmarshal(body, &apiErr)
	return &StatusError{StatusCode: resp.StatusCode, Message: apiErr.Message}
}
