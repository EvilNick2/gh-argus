package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/EvilNick2/gh-argus/internal/actions"
	"github.com/EvilNick2/gh-argus/internal/app"
	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/joblog"
	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/repos"
	"github.com/EvilNick2/gh-argus/internal/snapshot"
	"github.com/EvilNick2/gh-argus/internal/store"
	"github.com/EvilNick2/gh-argus/internal/watch"
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
	flag.Parse()

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
				wg.Go(func() { w.Watch(ctx, r, snaps.Seed(r), raw) })
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
		WatchRun: func(ctx context.Context, repo string, id int64) <-chan watch.RunEvent {
			ch := make(chan watch.RunEvent)
			go func() {
				w.WatchRun(ctx, repo, id, ch)
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
		Now:           time.Now,
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
