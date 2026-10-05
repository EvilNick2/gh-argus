package runners

import (
	"slices"
	"testing"
)

func TestDecode(t *testing.T) {
	// Shape from EvilNick2/infra.
	body := []byte(`{"total_count":1,"runners":[{"id":21,"name":"dockhand-relay","os":"Linux",
		"status":"online","busy":false,"labels":[{"id":1,"name":"self-hosted","type":"read-only"},
		{"id":2,"name":"Linux","type":"read-only"},{"id":3,"name":"X64","type":"read-only"},
		{"id":4,"name":"deploy","type":"custom"}]}]}`)

	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d runners", len(got))
	}
	r := got[0]
	if r.ID != 21 || r.Name != "dockhand-relay" || r.OS != "Linux" || r.Status != "online" || r.Busy {
		t.Errorf("decoded %+v", r)
	}
	if !slices.Equal(r.LabelNames(), []string{"self-hosted", "Linux", "X64", "deploy"}) {
		t.Errorf("labels %v", r.LabelNames())
	}
}
