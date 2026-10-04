// Package repos lists the repositories the picker offers and their CI status.
package repos

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GraphQL is satisfied by go-gh's *api.GraphQLClient.
type GraphQL interface {
	DoWithContext(ctx context.Context, query string, vars map[string]interface{}, resp interface{}) error
}

type Repo struct {
	FullName string
	Owner    string
	Name     string
	Private  bool
	PushedAt time.Time
}

// ownerAffiliations adds every repo of the viewer's organisations, not only
// those the viewer has been given access to individually.
const listQuery = `query($cursor: String) {
  viewer {
    repositories(first: 100, after: $cursor, isArchived: false,
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER],
      ownerAffiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER],
      orderBy: {field: PUSHED_AT, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { nameWithOwner name owner { login } isPrivate pushedAt }
    }
  }
}`

// List returns non-archived repos the viewer owns, collaborates on or that
// belong to the viewer's organisations, most recently pushed first.
func List(ctx context.Context, gql GraphQL) ([]Repo, error) {
	var out []Repo
	var cursor interface{}
	for {
		var resp struct {
			Viewer struct {
				Repositories struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []struct {
						NameWithOwner string
						Name          string
						Owner         struct{ Login string }
						IsPrivate     bool
						PushedAt      time.Time
					}
				}
			}
		}
		if err := gql.DoWithContext(ctx, listQuery, map[string]interface{}{"cursor": cursor}, &resp); err != nil {
			return nil, err
		}
		page := resp.Viewer.Repositories
		for _, n := range page.Nodes {
			out = append(out, Repo{
				FullName: n.NameWithOwner,
				Owner:    n.Owner.Login,
				Name:     n.Name,
				Private:  n.IsPrivate,
				PushedAt: n.PushedAt,
			})
		}
		if !page.PageInfo.HasNextPage {
			return out, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}

type Status int

const (
	StatusNone Status = iota
	StatusPassing
	StatusFailing
	StatusRunning
)

func (s Status) String() string {
	return [...]string{"none", "passing", "failing", "running"}[s]
}

// Suite is one check suite as GraphQL reports it, upper case.
type Suite struct {
	Status     string
	Conclusion string
}

// Summarize folds the suites of a commit into one status. Anything still
// going wins, then any failure, then any success.
func Summarize(suites []Suite) Status {
	out := StatusNone
	for _, s := range suites {
		if s.Status != "COMPLETED" {
			return StatusRunning
		}
		switch s.Conclusion {
		case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED":
			out = StatusFailing
		case "SUCCESS":
			if out == StatusNone {
				out = StatusPassing
			}
		}
	}
	return out
}

// actionsAppID is the GitHub Actions app. Without filtering to it, suites
// from Pages, Render and other apps sit at QUEUED forever.
const actionsAppID = 15368

// Statuses fetches the default-branch CI status of each "owner/name" in one
// request. Callers should pass a page of names, since latency grows with the
// count.
func Statuses(ctx context.Context, gql GraphQL, names []string) (map[string]Status, error) {
	out := make(map[string]Status, len(names))
	if len(names) == 0 {
		return out, nil
	}
	var q strings.Builder
	q.WriteString("query {\n")
	for i, full := range names {
		owner, name, ok := strings.Cut(full, "/")
		if !ok {
			return nil, fmt.Errorf("repo %q: want owner/name", full)
		}
		fmt.Fprintf(&q, `  r%d: repository(owner: %q, name: %q) {
    defaultBranchRef { target { ... on Commit {
      checkSuites(first: 50, filterBy: {appId: %d}) { nodes { status conclusion } }
    } } }
  }
`, i, owner, name, actionsAppID)
	}
	q.WriteString("}")

	var resp map[string]*struct {
		DefaultBranchRef *struct {
			Target struct {
				CheckSuites struct{ Nodes []Suite }
			}
		}
	}
	if err := gql.DoWithContext(ctx, q.String(), nil, &resp); err != nil {
		return nil, err
	}
	for i, full := range names {
		r := resp[fmt.Sprintf("r%d", i)]
		if r == nil || r.DefaultBranchRef == nil {
			out[full] = StatusNone
			continue
		}
		out[full] = Summarize(r.DefaultBranchRef.Target.CheckSuites.Nodes)
	}
	return out, nil
}
