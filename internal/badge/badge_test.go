package badge

import "testing"

func TestWord(t *testing.T) {
	cases := []struct{ status, conclusion, want string }{
		{"queued", "", "wait"},
		{"waiting", "", "wait"},
		{"pending", "", "wait"},
		{"in_progress", "", "run"},
		{"completed", "success", "pass"},
		{"completed", "failure", "fail"},
		{"completed", "timed_out", "fail"},
		{"completed", "startup_failure", "fail"},
		{"completed", "cancelled", "canc"},
		{"completed", "skipped", "skip"},
		{"completed", "action_required", "act"},
		{"completed", "neutral", "neut"},
		{"completed", "", ""},
	}
	for _, c := range cases {
		if got := Word(c.status, c.conclusion); got != c.want {
			t.Errorf("Word(%q, %q) = %q, want %q", c.status, c.conclusion, got, c.want)
		}
	}
}
