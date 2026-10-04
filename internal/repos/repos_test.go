package repos

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeGQL answers each query with the next canned response and records the
// queries and variables it was sent.
type fakeGQL struct {
	responses []string
	queries   []string
	vars      []map[string]interface{}
}

func (f *fakeGQL) DoWithContext(ctx context.Context, query string, vars map[string]interface{}, resp interface{}) error {
	f.queries = append(f.queries, query)
	f.vars = append(f.vars, vars)
	body := f.responses[0]
	f.responses = f.responses[1:]
	return json.Unmarshal([]byte(body), resp)
}

func TestListFollowsPages(t *testing.T) {
	gql := &fakeGQL{responses: []string{
		`{"viewer":{"repositories":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},"nodes":[
			{"nameWithOwner":"EvilNick2/dotfiles","name":"dotfiles","owner":{"login":"EvilNick2"},
			 "isPrivate":true,"pushedAt":"2026-10-04T19:05:21Z"}]}}}`,
		`{"viewer":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":"c2"},"nodes":[
			{"nameWithOwner":"psyeo2/homelab-gitops","name":"homelab-gitops","owner":{"login":"psyeo2"},
			 "isPrivate":false,"pushedAt":"2026-10-03T08:00:00Z"}]}}}`,
	}}

	got, err := List(context.Background(), gql)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2", len(got))
	}
	if got[0].FullName != "EvilNick2/dotfiles" || got[0].Owner != "EvilNick2" || !got[0].Private || got[0].PushedAt.IsZero() {
		t.Errorf("first repo %+v", got[0])
	}
	if got[1].FullName != "psyeo2/homelab-gitops" || got[1].Private {
		t.Errorf("second repo %+v", got[1])
	}
	if gql.vars[0]["cursor"] != nil || gql.vars[1]["cursor"] != "c1" {
		t.Errorf("cursors sent %v then %v, want nil then c1", gql.vars[0]["cursor"], gql.vars[1]["cursor"])
	}
}

func TestListQueryIncludesOrganisationRepos(t *testing.T) {
	gql := &fakeGQL{responses: []string{`{"viewer":{"repositories":{"pageInfo":{},"nodes":[]}}}`}}

	if _, err := List(context.Background(), gql); err != nil {
		t.Fatal(err)
	}
	q := gql.queries[0]
	// Without ownerAffiliations the list drops all but 1 of the 117
	// Bath-Impact-Lab repos.
	for _, want := range []string{
		" affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]",
		"ownerAffiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]",
		"isArchived: false",
		"PUSHED_AT",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q", want)
		}
	}
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		name   string
		suites []Suite
		want   Status
	}{
		{"no suites", nil, StatusNone},
		{"all success", []Suite{{"COMPLETED", "SUCCESS"}, {"COMPLETED", "SUCCESS"}}, StatusPassing},
		{"one failure", []Suite{{"COMPLETED", "SUCCESS"}, {"COMPLETED", "FAILURE"}}, StatusFailing},
		{"timed out", []Suite{{"COMPLETED", "TIMED_OUT"}}, StatusFailing},
		{"startup failure", []Suite{{"COMPLETED", "STARTUP_FAILURE"}}, StatusFailing},
		{"running beats failure", []Suite{{"COMPLETED", "FAILURE"}, {"IN_PROGRESS", ""}}, StatusRunning},
		{"queued", []Suite{{"QUEUED", ""}}, StatusRunning},
		{"only skipped or cancelled", []Suite{{"COMPLETED", "SKIPPED"}, {"COMPLETED", "CANCELLED"}}, StatusNone},
	}
	for _, c := range cases {
		if got := Summarize(c.suites); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStatusesQueriesEachRepoAndSummarizes(t *testing.T) {
	gql := &fakeGQL{responses: []string{`{
		"r0":{"defaultBranchRef":{"target":{"checkSuites":{"nodes":[{"status":"COMPLETED","conclusion":"FAILURE"}]}}}},
		"r1":{"defaultBranchRef":{"target":{"checkSuites":{"nodes":[]}}}},
		"r2":{"defaultBranchRef":null}}`}}

	got, err := Statuses(context.Background(), gql, []string{"EvilNick2/dotfiles", "EvilNick2/infra", "o/empty"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Status{"EvilNick2/dotfiles": StatusFailing, "EvilNick2/infra": StatusNone, "o/empty": StatusNone}
	for name, s := range want {
		if got[name] != s {
			t.Errorf("%s: got %v, want %v", name, got[name], s)
		}
	}
	q := gql.queries[0]
	if !strings.Contains(q, "appId: 15368") {
		t.Error("query does not filter check suites to the Actions app")
	}
	if !strings.Contains(q, `r0: repository(owner: "EvilNick2", name: "dotfiles")`) {
		t.Errorf("query does not alias repos as expected:\n%s", q)
	}
}

func TestStatusesWithNoReposMakesNoRequest(t *testing.T) {
	gql := &fakeGQL{}

	got, err := Statuses(context.Background(), gql, nil)
	if err != nil || len(got) != 0 || len(gql.queries) != 0 {
		t.Errorf("got %v, %v after %d queries", got, err, len(gql.queries))
	}
}
