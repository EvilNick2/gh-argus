package repos

import (
	"context"
	"encoding/json"
	"fmt"
)

// Getter fetches an API path, such as a Fetcher's Get returning the body.
type Getter func(ctx context.Context, path string) ([]byte, error)

const (
	perPage  = 100
	maxPages = 10
)

// names follows the pages of a list endpoint, decoding each page into names
// with decode, until a short page or maxPages.
func names(ctx context.Context, get Getter, base string, decode func([]byte) ([]string, error)) ([]string, error) {
	var out []string
	for page := 1; page <= maxPages; page++ {
		body, err := get(ctx, fmt.Sprintf("%s?per_page=%d&page=%d", base, perPage, page))
		if err != nil {
			return nil, err
		}
		got, err := decode(body)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
		if len(got) < perPage {
			break
		}
	}
	return out, nil
}

// Branches lists the branch names of repo, up to 1000.
func Branches(ctx context.Context, get Getter, repo string) ([]string, error) {
	return names(ctx, get, "/repos/"+repo+"/branches", func(body []byte) ([]string, error) {
		var page []struct{ Name string }
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		out := make([]string, len(page))
		for i, b := range page {
			out[i] = b.Name
		}
		return out, nil
	})
}

// Environments lists the deployment environment names of repo, up to 1000.
func Environments(ctx context.Context, get Getter, repo string) ([]string, error) {
	return names(ctx, get, "/repos/"+repo+"/environments", func(body []byte) ([]string, error) {
		var page struct {
			Environments []struct{ Name string }
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		out := make([]string, len(page.Environments))
		for i, e := range page.Environments {
			out[i] = e.Name
		}
		return out, nil
	})
}
