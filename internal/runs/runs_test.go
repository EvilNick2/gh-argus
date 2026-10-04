package runs

import (
	"testing"
)

func TestDecodeReadsWorkflowRuns(t *testing.T) {
	body := []byte(`{"total_count":1,"workflow_runs":[{"id":37226925135,"name":"Manifest check",
		"display_title":"feat: x","status":"completed","conclusion":"failure","head_branch":"main",
		"event":"push","run_attempt":2,"run_number":15,"html_url":"https://example/runs/1",
		"created_at":"2026-10-04T19:05:24Z","run_started_at":"2026-10-04T19:05:24Z",
		"updated_at":"2026-10-04T19:05:33Z","workflow_id":370320723}]}`)

	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d runs, want 1", len(got))
	}
	r := got[0]
	if r.ID != 37226925135 || r.Name != "Manifest check" || r.Status != "completed" ||
		r.Conclusion != "failure" || r.RunAttempt != 2 || r.RunNumber != 15 || r.HeadBranch != "main" {
		t.Errorf("decoded %+v", r)
	}
	if r.UpdatedAt.Sub(r.CreatedAt).Seconds() != 9 {
		t.Errorf("timestamps not parsed: created %v updated %v", r.CreatedAt, r.UpdatedAt)
	}
}

func TestActive(t *testing.T) {
	cases := []struct {
		statuses []string
		want     bool
	}{
		{nil, false},
		{[]string{"completed", "completed"}, false},
		{[]string{"completed", "in_progress"}, true},
		{[]string{"queued"}, true},
		{[]string{"waiting"}, true},
	}
	for _, c := range cases {
		var rs []Run
		for _, s := range c.statuses {
			rs = append(rs, Run{Status: s})
		}
		if got := Active(rs); got != c.want {
			t.Errorf("Active(%v) = %v, want %v", c.statuses, got, c.want)
		}
	}
}

func TestDiffReportsNewRun(t *testing.T) {
	prev := []Run{{ID: 1, Status: "completed", Conclusion: "success"}}
	next := []Run{{ID: 2, Status: "queued"}, {ID: 1, Status: "completed", Conclusion: "success"}}

	got := Diff(prev, next)
	if len(got) != 1 || got[0].Run.ID != 2 || got[0].Prev != nil {
		t.Fatalf("got %+v, want one new-run change for id 2", got)
	}
}

func TestDiffReportsStatusConclusionAndAttemptChanges(t *testing.T) {
	cases := []struct {
		name       string
		prev, next Run
	}{
		{"status", Run{ID: 1, Status: "queued"}, Run{ID: 1, Status: "in_progress"}},
		{"conclusion", Run{ID: 1, Status: "completed"}, Run{ID: 1, Status: "completed", Conclusion: "failure"}},
		{"attempt", Run{ID: 1, Status: "completed", RunAttempt: 1}, Run{ID: 1, Status: "completed", RunAttempt: 2}},
	}
	for _, c := range cases {
		got := Diff([]Run{c.prev}, []Run{c.next})
		if len(got) != 1 || got[0].Prev == nil || *got[0].Prev != c.prev || got[0].Run != c.next {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestDiffIgnoresUnchangedAndDroppedRuns(t *testing.T) {
	prev := []Run{{ID: 1, Status: "completed"}, {ID: 2, Status: "completed"}}
	next := []Run{{ID: 1, Status: "completed"}}

	if got := Diff(prev, next); len(got) != 0 {
		t.Errorf("got %+v, want no changes", got)
	}
}
