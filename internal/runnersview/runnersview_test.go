package runnersview

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/EvilNick2/gh-argus/internal/fetch"
	"github.com/EvilNick2/gh-argus/internal/runners"
)

var relay = runners.Runner{ID: 21, Name: "dockhand-relay", OS: "Linux", Status: "online",
	Labels: []runners.Label{{Name: "self-hosted"}, {Name: "deploy"}}}

func view(m Model) string { return ansi.Strip(m.View()) }

func line(t *testing.T, v, substr string) string {
	t.Helper()
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, substr) {
			return l
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substr, v)
	return ""
}

func TestRunnerRows(t *testing.T) {
	busy := runners.Runner{ID: 22, Name: "builder", OS: "Windows", Status: "online", Busy: true}
	off := runners.Runner{ID: 23, Name: "old-box", OS: "macOS", Status: "offline"}
	m := New([]string{"EvilNick2/infra"}).SetSize(100, 20)
	m, _ = m.Update(LoadedMsg{Repo: "EvilNick2/infra", Runners: []runners.Runner{relay, busy, off}})

	v := view(m)
	if l := line(t, v, "dockhand-relay"); !strings.Contains(l, "online") || !strings.Contains(l, "Linux") ||
		!strings.Contains(l, "self-hosted, deploy") {
		t.Errorf("relay row %q", l)
	}
	if l := line(t, v, "builder"); !strings.Contains(l, "busy") {
		t.Errorf("busy row %q", l)
	}
	if l := line(t, v, "old-box"); !strings.Contains(l, "offline") {
		t.Errorf("offline row %q", l)
	}
}

func TestNotPermittedFor403And404(t *testing.T) {
	for _, code := range []int{403, 404} {
		m := New([]string{"psyeo2/homelab-gitops"}).SetSize(100, 20)
		err := fmt.Errorf("listing runners: %w", &fetch.StatusError{StatusCode: code, Path: "/x", Body: "Not Found"})
		m, _ = m.Update(LoadedMsg{Repo: "psyeo2/homelab-gitops", Err: err})

		v := view(m)
		if !strings.Contains(v, "not permitted") || strings.Contains(v, "Not Found") {
			t.Errorf("%d: view:\n%s", code, v)
		}
		if raw := m.View(); strings.Contains(raw, "\x1b[31m") {
			t.Errorf("%d: not permitted drawn in red: %q", code, raw)
		}
	}
}

func TestOtherErrorsShown(t *testing.T) {
	m := New([]string{"o/r"}).SetSize(100, 20)

	m, _ = m.Update(LoadedMsg{Repo: "o/r", Err: errors.New("GET runners: 502 Bad Gateway")})
	if v := view(m); !strings.Contains(v, "502 Bad Gateway") || strings.Contains(v, "not permitted") {
		t.Errorf("view:\n%s", v)
	}
}

func TestNoRunners(t *testing.T) {
	m := New([]string{"The-Psychward/music"}).SetSize(100, 20)

	m, _ = m.Update(LoadedMsg{Repo: "The-Psychward/music", Runners: []runners.Runner{}})
	if v := view(m); !strings.Contains(v, "no self-hosted runners") {
		t.Errorf("view:\n%s", v)
	}
}
