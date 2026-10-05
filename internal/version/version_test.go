package version

import "testing"

func TestShort(t *testing.T) {
	cases := map[string]string{
		"v0.1.0-rc.1":                        "v0.1.0-rc.1",
		"v0.1.0":                             "v0.1.0",
		"v0.1.0+dirty":                       "v0.1.0 modified",
		"v0.0.0-20261005092227-5a52c4614c7e": "dev 5a52c46",
		"v0.0.0-20261005092227-5a52c4614c7e+dirty":  "dev 5a52c46 modified",
		"v0.1.1-0.20261005092227-5a52c4614c7e":      "dev 5a52c46",
		"v0.2.0-rc.1.0.20261005092227-5a52c4614c7e": "dev 5a52c46",
		"(devel)": "dev",
		"":        "dev",
	}
	for in, want := range cases {
		if got := Short(in); got != want {
			t.Errorf("Short(%q) = %q, want %q", in, got, want)
		}
	}
}
