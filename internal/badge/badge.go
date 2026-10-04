// Package badge renders the short coloured state words used for runs, jobs
// and steps.
package badge

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

var styles = map[string]lipgloss.Style{
	"pass": lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	"fail": lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	"err":  lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	"run":  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	"wait": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
}

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

// Render pads word to 4 columns and colours it.
func Render(word string) string {
	return styles[word].Render(fmt.Sprintf("%-4s", word))
}
