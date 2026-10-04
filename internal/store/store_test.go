package store

import (
	"os"
	"path/filepath"
	"testing"
)

type sample struct {
	Repos []string
	N     int
}

func TestLoadMissingReportsNotFound(t *testing.T) {
	s := New(t.TempDir())

	var v sample
	found, err := s.Load("selection", &v)
	if err != nil || found {
		t.Errorf("got found=%v err=%v, want false, nil", found, err)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "not", "yet", "created"))
	in := sample{Repos: []string{"EvilNick2/dotfiles", "Bath-Impact-Lab/aXR-www"}, N: 3}

	if err := s.Save("selection", in); err != nil {
		t.Fatal(err)
	}
	var out sample
	found, err := s.Load("selection", &out)
	if err != nil || !found {
		t.Fatalf("got found=%v err=%v", found, err)
	}
	if len(out.Repos) != 2 || out.Repos[1] != "Bath-Impact-Lab/aXR-www" || out.N != 3 {
		t.Errorf("loaded %+v", out)
	}
}

func TestSaveReplacesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	if err := s.Save("selection", sample{N: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("selection", sample{N: 2}); err != nil {
		t.Fatal(err)
	}
	var out sample
	if _, err := s.Load("selection", &out); err != nil || out.N != 2 {
		t.Errorf("got %+v, %v, want N=2", out, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "selection.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir holds %v, want only selection.json", names)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "selection.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	var v sample
	if _, err := New(dir).Load("selection", &v); err == nil {
		t.Error("loading a corrupt file did not error")
	}
}

func TestOpenUsesUserConfigDir(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("AppData", cfg)
	t.Setenv("HOME", cfg)

	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("selection", sample{}); err != nil {
		t.Fatal(err)
	}
	want, _ := os.UserConfigDir()
	if _, err := os.Stat(filepath.Join(want, "gh-argus", "selection.json")); err != nil {
		t.Errorf("file not under %s/gh-argus: %v", want, err)
	}
}
