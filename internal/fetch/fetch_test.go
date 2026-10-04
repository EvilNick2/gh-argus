package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// etagServer serves body with etag, answering 304 when If-None-Match matches.
// It records the If-None-Match header of each request it receives.
type etagServer struct {
	etag, body string
	seen       []string
}

func (s *etagServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	inm := r.Header.Get("If-None-Match")
	s.seen = append(s.seen, inm)
	w.Header().Set("X-RateLimit-Remaining", "4990")
	if inm == s.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", s.etag)
	w.Write([]byte(s.body))
}

func newFetcher(t *testing.T, h http.Handler) *Fetcher {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.Client(), srv.URL)
}

func TestFirstGetSendsNoETagAndReturnsBody(t *testing.T) {
	s := &etagServer{etag: `W/"a"`, body: `{"n":1}`}
	f := newFetcher(t, s)

	res, err := f.Get(context.Background(), "/repos/o/r/actions/runs")
	if err != nil {
		t.Fatal(err)
	}
	if s.seen[0] != "" {
		t.Errorf("first request sent If-None-Match %q", s.seen[0])
	}
	if res.NotModified || string(res.Body) != `{"n":1}` {
		t.Errorf("got NotModified=%v body=%q", res.NotModified, res.Body)
	}
}

func TestRepeatGetUsesETagAndReturnsCachedBodyOn304(t *testing.T) {
	s := &etagServer{etag: `W/"a"`, body: `{"n":1}`}
	f := newFetcher(t, s)
	ctx := context.Background()

	if _, err := f.Get(ctx, "/runs"); err != nil {
		t.Fatal(err)
	}
	res, err := f.Get(ctx, "/runs")
	if err != nil {
		t.Fatal(err)
	}
	if s.seen[1] != `W/"a"` {
		t.Errorf("second request sent If-None-Match %q", s.seen[1])
	}
	if !res.NotModified || string(res.Body) != `{"n":1}` {
		t.Errorf("got NotModified=%v body=%q", res.NotModified, res.Body)
	}
}

func TestChangedResourceReplacesCachedBody(t *testing.T) {
	s := &etagServer{etag: `W/"a"`, body: `{"n":1}`}
	f := newFetcher(t, s)
	ctx := context.Background()

	if _, err := f.Get(ctx, "/runs"); err != nil {
		t.Fatal(err)
	}
	s.etag, s.body = `W/"b"`, `{"n":2}`
	res, err := f.Get(ctx, "/runs")
	if err != nil {
		t.Fatal(err)
	}
	if res.NotModified || string(res.Body) != `{"n":2}` {
		t.Errorf("got NotModified=%v body=%q", res.NotModified, res.Body)
	}
	if _, err := f.Get(ctx, "/runs"); err != nil {
		t.Fatal(err)
	}
	if s.seen[2] != `W/"b"` {
		t.Errorf("third request sent If-None-Match %q, want the new etag", s.seen[2])
	}
}

func TestETagsAreTrackedPerPath(t *testing.T) {
	s := &etagServer{etag: `W/"a"`, body: `{}`}
	f := newFetcher(t, s)
	ctx := context.Background()

	if _, err := f.Get(ctx, "/one"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(ctx, "/two"); err != nil {
		t.Fatal(err)
	}
	if s.seen[1] != "" {
		t.Errorf("request for a new path sent If-None-Match %q", s.seen[1])
	}
}

func TestRateLimitRemainingIsReported(t *testing.T) {
	f := newFetcher(t, &etagServer{etag: `W/"a"`, body: `{}`})

	res, err := f.Get(context.Background(), "/runs")
	if err != nil {
		t.Fatal(err)
	}
	if res.RateRemaining != 4990 {
		t.Errorf("RateRemaining = %d, want 4990", res.RateRemaining)
	}
}

func TestErrorStatusReturnsStatusError(t *testing.T) {
	f := newFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))

	_, err := f.Get(context.Background(), "/repos/o/r/actions/runners")
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusNotFound {
		t.Fatalf("got err %v, want *StatusError with 404", err)
	}
}
