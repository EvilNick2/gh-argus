package fetch

import (
	"context"
	"os"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// TestLive304IsFree checks against the real API that gh's HTTP client passes
// a 304 through with headers intact and that it costs no rate limit.
// Run with ARGUS_LIVE=1.
func TestLive304IsFree(t *testing.T) {
	if os.Getenv("ARGUS_LIVE") == "" {
		t.Skip("set ARGUS_LIVE=1 to run against api.github.com")
	}
	client, err := api.DefaultHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	f := New(client, "https://api.github.com")
	ctx := context.Background()
	const path = "/repos/EvilNick2/dotfiles/actions/runs?per_page=5"

	first, err := f.Get(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.Get(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !second.NotModified {
		t.Fatal("second request was not a 304")
	}
	if len(second.Body) == 0 || string(second.Body) != string(first.Body) {
		t.Error("304 did not return the cached body")
	}
	if second.RateRemaining < 0 || second.RateRemaining != first.RateRemaining {
		t.Errorf("rate remaining went %d -> %d, want unchanged", first.RateRemaining, second.RateRemaining)
	}
}
