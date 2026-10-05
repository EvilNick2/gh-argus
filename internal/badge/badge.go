// Package badge names Actions states with short words, which the metrics
// history uses.
package badge

// Word is a word of at most 4 letters for an Actions status and conclusion,
// which runs, jobs and steps share.
func Word(status, conclusion string) string {
	switch status {
	case "completed":
	case "in_progress":
		return "run"
	default:
		return "wait"
	}
	switch conclusion {
	case "success":
		return "pass"
	case "failure", "timed_out", "startup_failure":
		return "fail"
	case "cancelled":
		return "canc"
	case "skipped":
		return "skip"
	case "action_required":
		return "act"
	}
	return conclusion[:min(4, len(conclusion))]
}
