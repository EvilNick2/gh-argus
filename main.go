package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/runs"
	"github.com/EvilNick2/gh-argus/internal/watch"
	"github.com/cli/go-gh/v2/pkg/api"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var repos []string
	flag.Func("R", "repository to watch as `owner/repo`, repeatable", func(s string) error {
		if strings.Count(s, "/") != 1 || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
			return errors.New("want owner/repo")
		}
		repos = append(repos, s)
		return nil
	})
	flag.Parse()
	if len(repos) == 0 {
		return errors.New("usage: gh argus -R owner/repo [-R owner/repo ...]")
	}

	client, err := api.DefaultHTTPClient()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	w := &watch.Watcher{
		Fetcher:   fetch.New(client, "https://api.github.com"),
		Intervals: watch.DefaultIntervals,
	}
	events := make(chan watch.Event)
	for _, repo := range repos {
		go w.Watch(ctx, repo, events)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			printEvent(ev)
		}
	}
}

func printEvent(ev watch.Event) {
	ts := time.Now().Format("15:04:05")
	switch {
	case ev.Err != nil:
		fmt.Printf("%s %s error: %v\n", ts, ev.Repo, ev.Err)
	case ev.Initial:
		fmt.Printf("%s %s watching, %d recent runs\n", ts, ev.Repo, len(ev.Runs))
		for i, r := range ev.Runs {
			if i == 0 || r.Status != "completed" {
				fmt.Printf("%s %s %s %s\n", ts, ev.Repo, describe(r), state(r))
			}
		}
	default:
		// Diff lists newest first. Print oldest first so output reads in order.
		for _, c := range slices.Backward(ev.Changes) {
			from := "new"
			if c.Prev != nil {
				from = state(*c.Prev)
			}
			fmt.Printf("%s %s %s %s -> %s\n", ts, ev.Repo, describe(c.Run), from, state(c.Run))
		}
	}
}

func describe(r runs.Run) string {
	s := fmt.Sprintf("#%d %s (%s)", r.RunNumber, r.Name, r.HeadBranch)
	if r.RunAttempt > 1 {
		s += fmt.Sprintf(" attempt %d", r.RunAttempt)
	}
	return s
}

func state(r runs.Run) string {
	if r.Conclusion != "" {
		return r.Status + "/" + r.Conclusion
	}
	return r.Status
}
