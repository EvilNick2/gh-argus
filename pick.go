package main

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/EvilNick2/gh-argus/internal/picker"
	"github.com/EvilNick2/gh-argus/internal/repos"
	"github.com/EvilNick2/gh-argus/internal/store"
)

// pick runs the repo picker and returns the chosen repos, or nil if the user
// quit. The picker opens on the cached repo list and refreshes it behind.
func pick(ctx context.Context) ([]string, error) {
	st, err := store.Open()
	if err != nil {
		return nil, err
	}
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}

	var cached []repos.Repo
	if _, err := st.Load("repos", &cached); err != nil {
		return nil, err
	}
	var selection []string
	if _, err := st.Load("selection", &selection); err != nil {
		return nil, err
	}

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

	final, err := tea.NewProgram(picker.New(cached, selection, fetch).WithInit(refresh)).Run()
	if err != nil {
		return nil, err
	}
	m := final.(picker.Model)
	if !m.Done() {
		return nil, nil
	}
	chosen := m.Selected()
	if err := st.Save("selection", chosen); err != nil {
		return nil, err
	}
	return chosen, nil
}
