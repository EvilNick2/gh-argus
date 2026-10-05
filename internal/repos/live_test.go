package repos

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// TestLiveListAndStatuses runs both queries against the real API and logs
// counts and timings. Run with ARGUS_LIVE=1 and -v.
func TestLiveListAndStatuses(t *testing.T) {
	if os.Getenv("ARGUS_LIVE") == "" {
		t.Skip("set ARGUS_LIVE=1 to run against api.github.com")
	}
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	start := time.Now()
	list, err := List(ctx, gql)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("List: %d repos in %v", len(list), time.Since(start).Round(time.Millisecond))
	if len(list) == 0 {
		t.Fatal("no repos listed")
	}
	for i := 1; i < len(list); i++ {
		if list[i].PushedAt.After(list[i-1].PushedAt) {
			t.Fatalf("not ordered by pushedAt at %d: %s after %s", i, list[i].FullName, list[i-1].FullName)
		}
	}

	var names []string
	for _, r := range list[:min(20, len(list))] {
		names = append(names, r.FullName)
	}
	start = time.Now()
	st, err := Statuses(ctx, gql, names)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Statuses: %d repos in %v", len(st), time.Since(start).Round(time.Millisecond))
	for _, n := range names {
		t.Logf("  %-40s %v", n, st[n])
	}
}

// TestLiveBranchesFollowsPages lists cli/cli's branches, which run past one
// page of 100. Run with ARGUS_LIVE=1 and -v.
func TestLiveBranchesFollowsPages(t *testing.T) {
	if os.Getenv("ARGUS_LIVE") == "" {
		t.Skip("set ARGUS_LIVE=1 to run against api.github.com")
	}
	rest, err := api.DefaultRESTClient()
	if err != nil {
		t.Fatal(err)
	}
	get := func(ctx context.Context, path string) ([]byte, error) {
		var raw json.RawMessage
		err := rest.DoWithContext(ctx, "GET", strings.TrimPrefix(path, "/"), nil, &raw)
		return raw, err
	}

	got, err := Branches(context.Background(), get, "cli/cli")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cli/cli: %d branches", len(got))
	if len(got) <= 100 {
		t.Errorf("got %d branches, want more than one page", len(got))
	}
	envs, err := Environments(context.Background(), get, "EvilNick2/infra")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("EvilNick2/infra environments: %v", envs)
}
