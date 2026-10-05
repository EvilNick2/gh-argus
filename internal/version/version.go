// Package version names the build argus is running, from the version the Go
// toolchain stamps into the binary out of git.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// pseudo matches the end of a Go pseudo-version, which a build from an
// untagged commit gets: a timestamp and the commit hash.
var pseudo = regexp.MustCompile(`[-.]\d{14}-([0-9a-f]{12})$`)

// String is the running build's version, shortened by Short.
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Short("")
	}
	return Short(info.Main.Version)
}

// Short turns a module version into a label for the header. A tagged build
// shows its tag, any other build "dev" and its commit, and a build from a
// tree with uncommitted changes is marked modified.
func Short(v string) string {
	v, dirty := strings.CutSuffix(v, "+dirty")
	switch m := pseudo.FindStringSubmatch(v); {
	case m != nil:
		v = "dev " + m[1][:7]
	case v == "" || v == "(devel)":
		v = "dev"
	}
	if dirty {
		v += " modified"
	}
	return v
}
