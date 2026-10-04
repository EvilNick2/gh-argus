package timefmt

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{4 * time.Second, "4s"},
		{62 * time.Second, "1m02s"},
		{63 * time.Minute, "1h03m"},
	}
	for _, c := range cases {
		if got := Duration(c.d); got != c.want {
			t.Errorf("Duration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{59 * time.Second, "59s"},
		{90 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := Age(c.d); got != c.want {
			t.Errorf("Age(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
