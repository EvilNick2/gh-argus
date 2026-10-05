// Package fetch makes conditional GET requests against the GitHub REST API.
//
// It needs a plain *http.Client such as api.DefaultHTTPClient() from go-gh.
// go-gh's RESTClient treats a 304 as an error and drops the response headers.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Fetcher struct {
	client  *http.Client
	baseURL string

	mu    sync.Mutex
	cache map[string]entry

	remaining atomic.Int64
}

type entry struct {
	etag string
	body []byte
}

type Result struct {
	Body []byte
	// NotModified reports a 304, in which case Body is the cached copy, or
	// nil when the ETag came from Seed.
	NotModified bool
	// RateRemaining is X-RateLimit-Remaining, or -1 when absent.
	RateRemaining int
	// ETag validates Body, for callers that store it between sessions.
	ETag string
}

// Seed gives path an ETag from an earlier session, so the first request is
// conditional. A 304 then has no body, so the caller must hold the data the
// ETag validates. Paths already fetched in this session are left alone.
func (f *Fetcher) Seed(path, etag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.cache[path]; !ok {
		f.cache[path] = entry{etag: etag}
	}
}

type StatusError struct {
	StatusCode int
	Path       string
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: %d %s", e.Path, e.StatusCode, strings.TrimSpace(e.Body))
}

func New(client *http.Client, baseURL string) *Fetcher {
	f := &Fetcher{
		client:  client,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		cache:   map[string]entry{},
	}
	f.remaining.Store(-1)
	return f
}

func (f *Fetcher) Get(ctx context.Context, path string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+path, nil)
	if err != nil {
		return Result{}, err
	}
	f.mu.Lock()
	cached, ok := f.cache[path]
	f.mu.Unlock()
	if ok {
		req.Header.Set("If-None-Match", cached.etag)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	res := Result{RateRemaining: -1}
	if v, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining")); err == nil {
		res.RateRemaining = v
		f.remaining.Store(int64(v))
	}

	switch {
	case resp.StatusCode == http.StatusNotModified && ok:
		res.Body, res.NotModified, res.ETag = cached.body, true, cached.etag
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		res.Body, res.ETag = body, resp.Header.Get("ETag")
		if etag := res.ETag; etag != "" {
			f.mu.Lock()
			f.cache[path] = entry{etag: etag, body: body}
			f.mu.Unlock()
		}
	default:
		return Result{}, &StatusError{StatusCode: resp.StatusCode, Path: path, Body: string(body)}
	}
	return res, nil
}

// Remaining is X-RateLimit-Remaining from the latest response that carried
// it, or -1 before any did.
func (f *Fetcher) Remaining() int {
	return int(f.remaining.Load())
}
