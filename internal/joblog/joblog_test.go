package joblog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseStripsBOMAndTimestamps(t *testing.T) {
	body := "\xef\xbb\xbf2026-10-04T22:00:18.6135812Z Current runner version: '2.337.0'\n" +
		"2026-10-04T22:00:18.6164426Z Hosted Compute Agent\n"

	got := Parse([]byte(body))
	if len(got) != 2 || got[0].Text != "Current runner version: '2.337.0'" || got[1].Text != "Hosted Compute Agent" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Kind != Plain {
		t.Errorf("kind %v, want Plain", got[0].Kind)
	}
}

func TestParseHandlesCRLFAndCarriageReturns(t *testing.T) {
	body := "2026-10-04T22:00:18.1Z windows line\r\n" +
		"2026-10-04T22:00:18.2Z progress 10%\rprogress 50%\rprogress 100%\n"

	got := Parse([]byte(body))
	if len(got) != 2 || got[0].Text != "windows line" || got[1].Text != "progress 100%" {
		t.Fatalf("got %q", texts(got))
	}
}

func TestParseRecognisesMarkers(t *testing.T) {
	body := "2026-10-04T22:00:18.1Z ##[group]Run actions/checkout@v4\n" +
		"2026-10-04T22:00:18.2Z with:\n" +
		"2026-10-04T22:00:18.3Z ##[endgroup]\n" +
		"2026-10-04T22:00:18.4Z ##[error]Process completed with exit code 1.\n" +
		"2026-10-04T22:00:18.5Z ##[warning]Node 16 is deprecated\n" +
		"2026-10-04T22:00:18.6Z [command]/usr/bin/git version\n"

	got := Parse([]byte(body))
	want := []Line{
		{Kind: Group, Text: "Run actions/checkout@v4"},
		{Kind: Plain, Text: "with:"},
		{Kind: Error, Text: "Process completed with exit code 1."},
		{Kind: Warning, Text: "Node 16 is deprecated"},
		{Kind: Command, Text: "/usr/bin/git version"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v (endgroup dropped)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseStripsEscapeSequences(t *testing.T) {
	body := "2026-10-04T22:00:18.1Z \x1b[36;1mcoloured\x1b[0m and \x1b]0;title\x07\x1b[2Kcleared\n"

	got := Parse([]byte(body))
	if len(got) != 1 || got[0].Text != "coloured and cleared" {
		t.Fatalf("got %q", texts(got))
	}
}

func TestParseKeepsLinesWithoutTimestamp(t *testing.T) {
	got := Parse([]byte("no timestamp here\n2026-10-04T22:00:18.1Z  indented\n"))

	if len(got) != 2 || got[0].Text != "no timestamp here" || got[1].Text != " indented" {
		t.Fatalf("got %q", texts(got))
	}
}

func TestParseExpandsTabs(t *testing.T) {
	got := Parse([]byte("2026-10-04T22:00:18.1Z a\tb\n"))

	if len(got) != 1 || got[0].Text != "a       b" {
		t.Fatalf("got %q", texts(got))
	}
}

func TestParseDropsTrailingEmptyLine(t *testing.T) {
	got := Parse([]byte("2026-10-04T22:00:18.1Z one\n\n"))

	if len(got) != 2 || got[1].Text != "" {
		t.Fatalf("got %q, want the blank line kept and nothing after the final newline", texts(got))
	}
}

func texts(ls []Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func TestFetchFollowsRedirectToLogText(t *testing.T) {
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization sent to the blob host")
		}
		w.Write([]byte("2026-10-04T22:00:18.1Z hello\n"))
	}))
	t.Cleanup(blob.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/jobs/70/logs" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, blob.URL+"/log.txt", http.StatusFound)
	}))
	t.Cleanup(api.Close)

	body, err := Fetch(context.Background(), api.Client(), api.URL, "o/r", 70)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "2026-10-04T22:00:18.1Z hello\n" {
		t.Errorf("body %q", body)
	}
}

func TestFetchErrorCarriesStatus(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(api.Close)

	_, err := Fetch(context.Background(), api.Client(), api.URL, "o/r", 70)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusNotFound {
		t.Fatalf("got %v, want *StatusError 404", err)
	}
}

func TestParseDropsStrayControlCharacters(t *testing.T) {
	got := Parse([]byte("2026-10-04T22:00:18.1Z a\x08b\x00c\x07d\x7fe\n"))

	if len(got) != 1 || got[0].Text != "abcde" {
		t.Fatalf("got %q", texts(got))
	}
}
