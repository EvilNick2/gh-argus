package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// pagedGetter serves n branch names in pages of the per_page asked for and
// records the paths requested.
type pagedGetter struct {
	n     int
	paths []string
}

func (g *pagedGetter) branches(ctx context.Context, path string) ([]byte, error) {
	g.paths = append(g.paths, path)
	var page, per int
	fmt.Sscanf(path[strings.Index(path, "per_page="):], "per_page=%d&page=%d", &per, &page)
	var out []map[string]string
	for i := (page - 1) * per; i < min(g.n, page*per); i++ {
		out = append(out, map[string]string{"name": fmt.Sprintf("b%03d", i)})
	}
	return json.Marshal(out)
}

func TestBranchesFollowsPages(t *testing.T) {
	g := &pagedGetter{n: 250}

	got, err := Branches(context.Background(), g.branches, "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 || got[0] != "b000" || got[249] != "b249" {
		t.Errorf("got %d branches, first %q", len(got), got[0])
	}
	want := []string{
		"/repos/o/r/branches?per_page=100&page=1",
		"/repos/o/r/branches?per_page=100&page=2",
		"/repos/o/r/branches?per_page=100&page=3",
	}
	if !slices.Equal(g.paths, want) {
		t.Errorf("paths %v", g.paths)
	}
}

func TestBranchesStopsAfterAFullLastPageReturnsEmpty(t *testing.T) {
	g := &pagedGetter{n: 100}

	got, err := Branches(context.Background(), g.branches, "o/r")
	if err != nil || len(got) != 100 || len(g.paths) != 2 {
		t.Errorf("got %d branches from %d requests, err %v", len(got), len(g.paths), err)
	}
}

func TestBranchesCappedAtTenPages(t *testing.T) {
	g := &pagedGetter{n: 5000}

	got, _ := Branches(context.Background(), g.branches, "o/r")
	if len(got) != 1000 || len(g.paths) != 10 {
		t.Errorf("got %d branches from %d requests, want 1000 from 10", len(got), len(g.paths))
	}
}

func TestBranchesError(t *testing.T) {
	get := func(ctx context.Context, path string) ([]byte, error) { return nil, errors.New("502") }

	if _, err := Branches(context.Background(), get, "o/r"); err == nil {
		t.Error("error not returned")
	}
}

func TestEnvironments(t *testing.T) {
	var paths []string
	get := func(ctx context.Context, path string) ([]byte, error) {
		paths = append(paths, path)
		return []byte(`{"total_count":2,"environments":[{"name":"github-pages"},{"name":"production"}]}`), nil
	}

	got, err := Environments(context.Background(), get, "EvilNick2/infra")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"github-pages", "production"}) {
		t.Errorf("got %v", got)
	}
	if paths[0] != "/repos/EvilNick2/infra/environments?per_page=100&page=1" {
		t.Errorf("path %q", paths[0])
	}
}
