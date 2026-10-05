package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/EvilNick2/gh-argus/internal/actions"
	"github.com/EvilNick2/gh-argus/internal/app"
	"github.com/EvilNick2/gh-argus/internal/caches"
	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/repos"
	"github.com/EvilNick2/gh-argus/internal/runners"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/snapshot"
	"github.com/EvilNick2/gh-argus/internal/store"
	"github.com/EvilNick2/gh-argus/internal/version"
	"github.com/EvilNick2/gh-argus/internal/watch"
	"github.com/EvilNick2/gh-argus/internal/workflows"
)

const apiURL = "https://api.github.com"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var given []string
	flag.Func("R", "repository to watch as `owner/repo`, repeatable", func(s string) error {
		if strings.Count(s, "/") != 1 || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
			return errors.New("want owner/repo")
		}
		given = append(given, s)
		return nil
	})
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("gh argus", version.String())
		return nil
	}

	theme := os.Getenv("ARGUS_THEME")
	if theme != "" && theme != "light" && theme != "dark" {
		return fmt.Errorf("ARGUS_THEME is %q, want light or dark", theme)
	}

	st, err := store.Open()
	if err != nil {
		return err
	}
	client, err := api.DefaultHTTPClient()
	if err != nil {
		return err
	}
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return err
	}
	p, err := newPicker(st, gql)
	if err != nil {
		return err
	}

	snaps, err := snapshot.Load(st, time.Now)
	if err != nil {
		return err
	}

	w := &watch.Watcher{
		Fetcher:   fetch.New(client, apiURL),
		Intervals: watch.DefaultIntervals,
	}
	deps := app.Deps{
		Watch: func(ctx context.Context, rs []string) <-chan watch.Event {
			raw, out := make(chan watch.Event), make(chan watch.Event)
			var wg sync.WaitGroup
			for _, r := range rs {
				wg.Go(func() { w.Watch(ctx, r, snaps.SeedOnce(r), raw) })
			}
			go func() {
				wg.Wait()
				close(raw)
			}()
			// Record each event for the next session on its way to the app.
			// Once ctx is done the app has stopped reading, so drain instead.
			go func() {
				for ev := range raw {
					snaps.Record(ev)
					select {
					case out <- ev:
					case <-ctx.Done():
					}
				}
				close(out)
			}()
			return out
		},
		RunAttempt: func(ctx context.Context, repo string, id int64, n int) (runs.Run, error) {
			res, err := w.Fetcher.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/attempts/%d", repo, id, n))
			if err != nil {
				return runs.Run{}, err
			}
			var r runs.Run
			return r, json.Unmarshal(res.Body, &r)
		},
		WatchRun: func(ctx context.Context, repo string, id int64, attempt int) <-chan watch.RunEvent {
			ch := make(chan watch.RunEvent)
			go func() {
				w.WatchRun(ctx, repo, id, attempt, ch)
				close(ch)
			}()
			return ch
		},
		FetchLog: func(ctx context.Context, repo string, id int64) ([]joblog.Line, error) {
			body, err := joblog.Fetch(ctx, client, apiURL, repo, id)
			if err != nil {
				return nil, err
			}
			return joblog.Parse(body), nil
		},
		Act: func(ctx context.Context, repo string, id int64, k actions.Kind) error {
			return actions.Do(ctx, client, apiURL, repo, id, k)
		},
		SaveSelection: func(rs []string) error { return st.Save("selection", rs) },
		Seed:          snaps.Seed,
		Remaining:     w.Fetcher.Remaining,
		ListWorkflows: func(ctx context.Context, repo string) ([]workflows.Workflow, error) {
			res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/actions/workflows?per_page=100")
			if err != nil {
				return nil, err
			}
			return workflows.Decode(res.Body)
		},
		DispatchSpec: func(ctx context.Context, repo string, wf workflows.Workflow, ref string) (string, workflows.DispatchSpec, error) {
			if ref == "" {
				res, err := w.Fetcher.Get(ctx, "/repos/"+repo)
				if err != nil {
					return "", workflows.DispatchSpec{}, err
				}
				var info struct {
					DefaultBranch string `json:"default_branch"`
				}
				if err := json.Unmarshal(res.Body, &info); err != nil {
					return "", workflows.DispatchSpec{}, err
				}
				ref = info.DefaultBranch
			}
			res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/contents/"+wf.Path+"?ref="+url.QueryEscape(ref))
			if err != nil {
				return ref, workflows.DispatchSpec{}, err
			}
			src, err := workflows.DecodeContent(res.Body)
			if err != nil {
				return ref, workflows.DispatchSpec{}, err
			}
			spec, err := workflows.ParseDispatch(src)
			return ref, spec, err
		},
		RecentRuns: func(ctx context.Context, repo string) ([]runs.Run, error) {
			res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/actions/runs?per_page=100")
			if err != nil {
				return nil, err
			}
			return runs.Decode(res.Body)
		},
		ListRunners: func(ctx context.Context, repo string) ([]runners.Runner, error) {
			res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/actions/runners?per_page=100")
			if err != nil {
				return nil, err
			}
			return runners.Decode(res.Body)
		},
		ListCaches: func(ctx context.Context, repo string) ([]caches.Cache, error) {
			res, err := w.Fetcher.Get(ctx, "/repos/"+repo+"/actions/caches?per_page=100")
			if err != nil {
				return nil, err
			}
			return caches.Decode(res.Body)
		},
		DeleteRun: func(ctx context.Context, repo string, id int64) error {
			return actions.DeleteRun(ctx, client, apiURL, repo, id)
		},
		DeleteCache: func(ctx context.Context, repo string, id int64) error {
			return actions.DeleteCache(ctx, client, apiURL, repo, id)
		},
		Branches: func(ctx context.Context, repo string) ([]string, error) {
			return repos.Branches(ctx, getBody(w.Fetcher), repo)
		},
		Environments: func(ctx context.Context, repo string) ([]string, error) {
			return repos.Environments(ctx, getBody(w.Fetcher), repo)
		},
		Dispatch: func(ctx context.Context, repo string, id int64, ref string, inputs map[string]string) error {
			return actions.Dispatch(ctx, client, apiURL, repo, id, ref, inputs)
		},
		SetWorkflow: func(ctx context.Context, repo string, id int64, enabled bool) error {
			return actions.SetWorkflowEnabled(ctx, client, apiURL, repo, id, enabled)
		},
		Now:     time.Now,
		Version: version.String(),
		Theme:   theme,
	}

	stopFlush := make(chan struct{})
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				// A failed write is retried on the next tick and on exit.
				snaps.Flush()
			case <-stopFlush:
				return
			}
		}
	}()

	_, err = tea.NewProgram(app.New(deps, p, given)).Run()
	close(stopFlush)
	if ferr := snaps.Flush(); err == nil {
		err = ferr
	}
	return err
}

// getBody adapts a Fetcher to repos.Getter.
func getBody(f *fetch.Fetcher) repos.Getter {
	return func(ctx context.Context, path string) ([]byte, error) {
		res, err := f.Get(ctx, path)
		return res.Body, err
	}
}

// newPicker opens the picker on the cached repo list and last selection, and
// refreshes the list in the background.
func newPicker(st *store.Store, gql *api.GraphQLClient) (picker.Model, error) {
	var cached []repos.Repo
	if _, err := st.Load("repos", &cached); err != nil {
		return picker.Model{}, err
	}
	var selection []string
	if _, err := st.Load("selection", &selection); err != nil {
		return picker.Model{}, err
	}

	ctx := context.Background()
	fetch := func(names []string) tea.Cmd {
		return func() tea.Msg {
			s, err := repos.Statuses(ctx, gql, names)
			return picker.StatusMsg{Statuses: s, Err: err}
		}
	}
	refresh := func() tea.Msg {
		list, err := repos.List(ctx, gql)
		if err != nil {
			return picker.ReposMsg{Err: fmt.Errorf("refreshing repos: %w", err)}
		}
		if err := st.Save("repos", list); err != nil {
			return picker.ReposMsg{Repos: list, Err: fmt.Errorf("saving repo cache: %w", err)}
		}
		return picker.ReposMsg{Repos: list}
	}
	return picker.New(cached, selection, fetch).WithInit(refresh), nil
}
