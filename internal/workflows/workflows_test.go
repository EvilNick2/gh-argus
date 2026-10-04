package workflows

import "testing"

func TestDecode(t *testing.T) {
	body := []byte(`{"total_count":2,"workflows":[
		{"id":324332571,"name":"Build and publish","path":".github/workflows/deploy-pages.yml","state":"active"},
		{"id":255355271,"name":"pages-build-deployment","path":"dynamic/pages/pages-build-deployment","state":"disabled_manually"}]}`)

	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 324332571 || got[0].Name != "Build and publish" ||
		got[0].Path != ".github/workflows/deploy-pages.yml" || got[1].State != "disabled_manually" {
		t.Errorf("decoded %+v", got)
	}
}

func TestDynamic(t *testing.T) {
	if !(Workflow{Path: "dynamic/pages/pages-build-deployment"}).Dynamic() {
		t.Error("dynamic path not detected")
	}
	if (Workflow{Path: ".github/workflows/ci.yml"}).Dynamic() {
		t.Error("file workflow reported dynamic")
	}
}

func TestEnabled(t *testing.T) {
	cases := map[string]bool{
		"active":              true,
		"disabled_manually":   false,
		"disabled_inactivity": false,
		"disabled_fork":       false,
		"deleted":             false,
	}
	for state, want := range cases {
		if got := (Workflow{State: state}).Enabled(); got != want {
			t.Errorf("Enabled() for %s = %v, want %v", state, got, want)
		}
	}
}
