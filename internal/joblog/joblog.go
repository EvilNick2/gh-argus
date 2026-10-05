// Package joblog fetches and parses the plain text log of an Actions job.
package joblog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

type Kind int

const (
	Plain Kind = iota
	Group
	Error
	Warning
	Command
)

type Line struct {
	Kind Kind
	Text string
}

var timestamp = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z `)

var markers = []struct {
	prefix string
	kind   Kind
}{
	{"##[group]", Group},
	{"##[error]", Error},
	{"##[warning]", Warning},
	{"##[notice]", Warning},
	{"[command]", Command},
	{"##[debug]", Plain},
}

// Parse splits a job log into display lines. It drops the byte order mark,
// timestamps, ##[endgroup] lines and the start-action and end-action lines
// around a composite action's steps, which repeat the group line after them
// or only record its outcome. It keeps only the text after the last
// carriage return of a line, and strips every escape sequence, since logs
// carry whatever the job's tools printed.
func Parse(body []byte) []Line {
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	raw := strings.Split(string(body), "\n")
	if n := len(raw); n > 0 && raw[n-1] == "" {
		raw = raw[:n-1]
	}

	out := make([]Line, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSuffix(s, "\r")
		s = timestamp.ReplaceAllString(s, "")
		if i := strings.LastIndexByte(s, '\r'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.Map(dropControl, expandTabs(ansi.Strip(s)))

		if s == "##[endgroup]" || strings.HasPrefix(s, "##[start-action ") || strings.HasPrefix(s, "##[end-action ") {
			continue
		}
		line := Line{Kind: Plain, Text: s}
		for _, m := range markers {
			if rest, ok := strings.CutPrefix(s, m.prefix); ok {
				line = Line{Kind: m.kind, Text: rest}
				break
			}
		}
		out = append(out, line)
	}
	return out
}

// dropControl removes control characters left after escape sequences are
// stripped. Tabs are expanded before it runs.
func dropControl(r rune) rune {
	if unicode.IsControl(r) {
		return -1
	}
	return r
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

type StatusError struct {
	StatusCode int
	Body       string
}

// Error gives GitHub's message when the body carries one, and otherwise the
// status text, since the log host answers errors with a page of XML.
func (e *StatusError) Error() string {
	var body struct{ Message string }
	text := http.StatusText(e.StatusCode)
	if json.Unmarshal([]byte(e.Body), &body) == nil && body.Message != "" {
		text = body.Message
	}
	return fmt.Sprintf("fetching log: %d %s", e.StatusCode, text)
}

// Fetch downloads the log of job id. The API answers with a redirect to the
// log text, which client follows.
func Fetch(ctx context.Context, client *http.Client, baseURL, repo string, id int64) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/jobs/%d/logs", strings.TrimSuffix(baseURL, "/"), repo, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}
