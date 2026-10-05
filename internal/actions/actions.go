// Package actions sends requests that change things on GitHub: run reruns
// and cancels, workflow enable, disable and dispatch, and cache deletes.
package actions

import (
	"bytes"
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
	return send(ctx, client, http.MethodPost, url, nil)
}

// SetWorkflowEnabled enables or disables workflow id of repo.
func SetWorkflowEnabled(ctx context.Context, client *http.Client, baseURL, repo string, id int64, enabled bool) error {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%d/%s", strings.TrimSuffix(baseURL, "/"), repo, id, verb)
	return send(ctx, client, http.MethodPut, url, nil)
}

// Dispatch triggers workflow id of repo on ref, which must accept
// workflow_dispatch. Inputs left out take their defaults from the workflow.
func Dispatch(ctx context.Context, client *http.Client, baseURL, repo string, id int64, ref string, inputs map[string]string) error {
	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%d/dispatches", strings.TrimSuffix(baseURL, "/"), repo, id)
	body := struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs,omitempty"`
	}{ref, inputs}
	return send(ctx, client, http.MethodPost, url, body)
}

// send makes a request with an optional JSON body and turns a non-2xx
// response into a StatusError carrying the API's message.
func send(ctx context.Context, client *http.Client, method, url string, body any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	msg, _ := io.ReadAll(resp.Body)
	var apiErr struct{ Message string }
	json.Unmarshal(msg, &apiErr)
	return &StatusError{StatusCode: resp.StatusCode, Message: apiErr.Message}
}

// DeleteCache deletes cache id of repo.
func DeleteCache(ctx context.Context, client *http.Client, baseURL, repo string, id int64) error {
	url := fmt.Sprintf("%s/repos/%s/actions/caches/%d", strings.TrimSuffix(baseURL, "/"), repo, id)
	return send(ctx, client, http.MethodDelete, url, nil)
}
