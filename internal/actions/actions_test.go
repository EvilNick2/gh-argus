package actions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EvilNick2/gh-argus/internal/runs"
)

func TestDoPostsToEndpoint(t *testing.T) {
	cases := []struct {
		kind   Kind
		path   string
		status int
	}{
		{RerunFailed, "/repos/o/r/actions/runs/16/rerun-failed-jobs", http.StatusCreated},
		{RerunAll, "/repos/o/r/actions/runs/16/rerun", http.StatusCreated},
		{Cancel, "/repos/o/r/actions/runs/16/cancel", http.StatusAccepted},
		{ForceCancel, "/repos/o/r/actions/runs/16/force-cancel", http.StatusAccepted},
	}
	for _, c := range cases {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.WriteHeader(c.status)
		}))

		err := Do(context.Background(), srv.Client(), srv.URL, "o/r", 16, c.kind)
		srv.Close()
		if err != nil {
			t.Errorf("%v: %v", c.kind, err)
		}
		if gotMethod != http.MethodPost || gotPath != c.path {
			t.Errorf("%v: sent %s %s, want POST %s", c.kind, gotMethod, gotPath, c.path)
		}
	}
}

func TestDoReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Cannot cancel a workflow run that is completed."}`, http.StatusConflict)
	}))
	defer srv.Close()

	err := Do(context.Background(), srv.Client(), srv.URL, "o/r", 16, Cancel)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusConflict {
		t.Fatalf("got %v, want *StatusError 409", err)
	}
	if se.Message != "Cannot cancel a workflow run that is completed." {
		t.Errorf("message %q, want the API's message", se.Message)
	}
}

func TestAllowed(t *testing.T) {
	running := runs.Run{Status: "in_progress"}
	queued := runs.Run{Status: "queued"}
	passed := runs.Run{Status: "completed", Conclusion: "success"}
	failed := runs.Run{Status: "completed", Conclusion: "failure"}
	cancelled := runs.Run{Status: "completed", Conclusion: "cancelled"}

	cases := []struct {
		kind Kind
		run  runs.Run
		want bool
	}{
		{Cancel, running, true},
		{Cancel, queued, true},
		{Cancel, passed, false},
		{ForceCancel, running, true},
		{ForceCancel, passed, false},
		{RerunAll, passed, true},
		{RerunAll, failed, true},
		{RerunAll, running, false},
		{RerunFailed, failed, true},
		{RerunFailed, cancelled, true},
		{RerunFailed, passed, false},
		{RerunFailed, running, false},
	}
	for _, c := range cases {
		if got := c.kind.Allowed(c.run); got != c.want {
			t.Errorf("%v.Allowed(%s/%s) = %v, want %v", c.kind, c.run.Status, c.run.Conclusion, got, c.want)
		}
	}
}

func TestSetWorkflowEnabled(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		var gotMethod, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}))
		err := SetWorkflowEnabled(context.Background(), srv.Client(), srv.URL, "o/r", 42, enabled)
		srv.Close()
		want := "/repos/o/r/actions/workflows/42/disable"
		if enabled {
			want = "/repos/o/r/actions/workflows/42/enable"
		}
		if err != nil || gotMethod != http.MethodPut || gotPath != want {
			t.Errorf("enabled=%v: err %v, sent %s %s, want PUT %s", enabled, err, gotMethod, gotPath, want)
		}
	}
}

func TestDispatchSendsRefAndInputs(t *testing.T) {
	var gotPath string
	var got struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("sent %s with Content-Type %q", r.Method, r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := Dispatch(context.Background(), srv.Client(), srv.URL, "o/r", 42, "main", map[string]string{"level": "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/o/r/actions/workflows/42/dispatches" || got.Ref != "main" || got.Inputs["level"] != "debug" {
		t.Errorf("sent %s %+v", gotPath, got)
	}
}

func TestDispatchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Workflow does not have 'workflow_dispatch' trigger"}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	err := Dispatch(context.Background(), srv.Client(), srv.URL, "o/r", 42, "main", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 422 || se.Message != "Workflow does not have 'workflow_dispatch' trigger" {
		t.Fatalf("got %v", err)
	}
}

func TestDeleteCache(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := DeleteCache(context.Background(), srv.Client(), srv.URL, "o/r", 505)
	if err != nil || gotMethod != http.MethodDelete || gotPath != "/repos/o/r/actions/caches/505" {
		t.Errorf("err %v, sent %s %s", err, gotMethod, gotPath)
	}
}

func TestDeleteRun(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := DeleteRun(context.Background(), srv.Client(), srv.URL, "o/r", 16)
	if err != nil || gotMethod != http.MethodDelete || gotPath != "/repos/o/r/actions/runs/16" {
		t.Errorf("err %v, sent %s %s", err, gotMethod, gotPath)
	}
}
