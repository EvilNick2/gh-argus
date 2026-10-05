// Package caches decodes a repository's Actions caches.
package caches

import (
	"encoding/json"
	"strings"
	"time"
)

type Cache struct {
	ID             int64     `json:"id"`
	Ref            string    `json:"ref"`
	Key            string    `json:"key"`
	SizeInBytes    int64     `json:"size_in_bytes"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// Decode parses a GET /repos/{owner}/{repo}/actions/caches response.
func Decode(body []byte) ([]Cache, error) {
	var page struct {
		Caches []Cache `json:"actions_caches"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Caches, nil
}

// Branch is the ref without refs/heads/, or the whole ref for anything else
// such as a pull request merge ref.
func (c Cache) Branch() string {
	return strings.TrimPrefix(c.Ref, "refs/heads/")
}
