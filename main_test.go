package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func inRepo(host, owner, name string) func() (repository.Repository, error) {
	return func() (repository.Repository, error) {
		return repository.Repository{Host: host, Owner: owner, Name: name}, nil
	}
}

func notInRepo() (repository.Repository, error) {
	return repository.Repository{}, errors.New("not a git repository")
}

func TestStartRepos(t *testing.T) {
	cases := []struct {
		name    string
		given   []string
		picker  bool
		current func() (repository.Repository, error)
		want    []string
	}{
		{"bare in a checkout watches it", nil, false, inRepo("github.com", "EvilNick2", "gh-argus"), []string{"EvilNick2/gh-argus"}},
		{"picker flag opens the picker", nil, true, inRepo("github.com", "EvilNick2", "gh-argus"), nil},
		{"outside a checkout opens the picker", nil, false, notInRepo, nil},
		{"enterprise remote opens the picker", nil, false, inRepo("ghe.example.com", "team", "app"), nil},
		{"-R wins over the checkout", []string{"o/a", "o/b"}, false, inRepo("github.com", "EvilNick2", "gh-argus"), []string{"o/a", "o/b"}},
		{"-R wins over the picker flag", []string{"o/a"}, true, notInRepo, []string{"o/a"}},
	}
	for _, c := range cases {
		if got := startRepos(c.given, c.picker, c.current); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
