package runs

import "testing"

func TestDecodeJobsReadsJobsAndSteps(t *testing.T) {
	body := []byte(`{"total_count":1,"jobs":[{"id":111508379133,"run_id":37226925135,"name":"check",
		"status":"completed","conclusion":"failure","started_at":"2026-10-04T19:05:26Z",
		"completed_at":"2026-10-04T19:05:33Z","steps":[{"name":"Set up job","number":1,
		"status":"completed","conclusion":"success","started_at":"2026-10-04T19:05:27Z",
		"completed_at":"2026-10-04T19:05:29Z"},{"name":"Run actions/checkout@v4","number":2,
		"status":"in_progress","conclusion":null}]}]}`)

	got, err := DecodeJobs(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d jobs, want 1", len(got))
	}
	j := got[0]
	if j.ID != 111508379133 || j.RunID != 37226925135 || j.Name != "check" ||
		j.Status != "completed" || j.Conclusion != "failure" || len(j.Steps) != 2 {
		t.Errorf("decoded %+v", j)
	}
	if s := j.Steps[0]; s.CompletedAt.Sub(s.StartedAt).Seconds() != 2 {
		t.Errorf("step times not parsed: %v to %v", s.StartedAt, s.CompletedAt)
	}
	if s := j.Steps[1]; s.Number != 2 || s.Name != "Run actions/checkout@v4" || s.Status != "in_progress" || s.Conclusion != "" {
		t.Errorf("decoded step %+v", s)
	}
}

func TestDiffJobsReportsNewJobWithoutSteps(t *testing.T) {
	next := []Job{{ID: 1, Status: "queued", Steps: []Step{{Number: 1, Status: "queued"}}}}

	got := DiffJobs(nil, next)
	if len(got) != 1 || got[0].Prev != nil || got[0].Job.ID != 1 || len(got[0].Steps) != 0 {
		t.Fatalf("got %+v, want one new job and no step changes", got)
	}
}

func TestDiffJobsReportsJobStateChange(t *testing.T) {
	prev := []Job{{ID: 1, Status: "in_progress"}}
	next := []Job{{ID: 1, Status: "completed", Conclusion: "success"}}

	got := DiffJobs(prev, next)
	if len(got) != 1 || got[0].Prev == nil || got[0].Prev.Status != "in_progress" || got[0].Job.Conclusion != "success" {
		t.Fatalf("got %+v, want in_progress -> completed/success", got)
	}
}

func TestDiffJobsReportsStepChangesUnderUnchangedJob(t *testing.T) {
	prev := []Job{{ID: 1, Status: "in_progress", Steps: []Step{
		{Number: 1, Status: "completed", Conclusion: "success"},
		{Number: 2, Status: "in_progress"},
	}}}
	next := []Job{{ID: 1, Status: "in_progress", Steps: []Step{
		{Number: 1, Status: "completed", Conclusion: "success"},
		{Number: 2, Status: "completed", Conclusion: "failure"},
		{Number: 3, Status: "in_progress"},
	}}}

	got := DiffJobs(prev, next)
	if len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
	steps := got[0].Steps
	if len(steps) != 2 {
		t.Fatalf("got step changes %+v, want steps 2 and 3", steps)
	}
	if steps[0].Prev == nil || steps[0].Prev.Status != "in_progress" || steps[0].Step.Conclusion != "failure" {
		t.Errorf("step 2 change %+v", steps[0])
	}
	if steps[1].Prev != nil || steps[1].Step.Number != 3 {
		t.Errorf("step 3 change %+v, want new step", steps[1])
	}
}

func TestDiffJobsIgnoresUnchanged(t *testing.T) {
	jobs := []Job{{ID: 1, Status: "completed", Steps: []Step{{Number: 1, Status: "completed"}}}}

	if got := DiffJobs(jobs, jobs); len(got) != 0 {
		t.Errorf("got %+v, want no changes", got)
	}
}
