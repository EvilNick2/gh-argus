package joblog

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// TestLiveFetchAndParse fetches the log of the latest dotfiles job through
// gh's HTTP client, redirect included. Run with ARGUS_LIVE=1 and -v.
func TestLiveFetchAndParse(t *testing.T) {
	if os.Getenv("ARGUS_LIVE") == "" {
		t.Skip("set ARGUS_LIVE=1 to run against api.github.com")
	}
	client, err := api.DefaultHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	rest, err := api.DefaultRESTClient()
	if err != nil {
		t.Fatal(err)
	}
	var runsResp struct {
		WorkflowRuns []struct{ ID int64 } `json:"workflow_runs"`
	}
	if err := rest.Get("repos/EvilNick2/dotfiles/actions/runs?per_page=1&status=completed", &runsResp); err != nil {
		t.Fatal(err)
	}
	var jobsResp struct{ Jobs []struct{ ID int64 } }
	if err := rest.Get(fmt.Sprintf("repos/EvilNick2/dotfiles/actions/runs/%d/jobs", runsResp.WorkflowRuns[0].ID), &jobsResp); err != nil {
		t.Fatal(err)
	}

	body, err := Fetch(context.Background(), client, "https://api.github.com", "EvilNick2/dotfiles", jobsResp.Jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	lines := Parse(body)
	kinds := map[Kind]int{}
	for _, l := range lines {
		kinds[l.Kind]++
		for _, r := range l.Text {
			if r == 0x1b {
				t.Fatalf("escape left in %q", l.Text)
			}
		}
	}
	t.Logf("%d bytes, %d lines, %d groups, %d errors, %d warnings, %d commands",
		len(body), len(lines), kinds[Group], kinds[Error], kinds[Warning], kinds[Command])
	if len(lines) == 0 || kinds[Group] == 0 {
		t.Error("expected lines and at least one group")
	}
}
