package caches

import "testing"

func TestDecode(t *testing.T) {
	body := []byte(`{"total_count":1,"actions_caches":[{"id":505,"ref":"refs/heads/main",
		"key":"Linux-node-208b2f","version":"73885106","last_accessed_at":"2026-10-04T10:00:00Z",
		"created_at":"2026-10-01T10:00:00Z","size_in_bytes":1048576}]}`)

	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d caches", len(got))
	}
	c := got[0]
	if c.ID != 505 || c.Key != "Linux-node-208b2f" || c.Ref != "refs/heads/main" || c.SizeInBytes != 1048576 ||
		c.LastAccessedAt.Day() != 4 {
		t.Errorf("decoded %+v", c)
	}
}

func TestBranch(t *testing.T) {
	cases := map[string]string{
		"refs/heads/main":    "main",
		"refs/heads/feat/x":  "feat/x",
		"refs/pull/12/merge": "refs/pull/12/merge",
	}
	for ref, want := range cases {
		if got := (Cache{Ref: ref}).Branch(); got != want {
			t.Errorf("Branch(%q) = %q, want %q", ref, got, want)
		}
	}
}
