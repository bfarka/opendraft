package version

import (
	"runtime/debug"
	"strings"
)

// Version can be overridden at build time via:
// -ldflags "-X github.com/bfarka/opendraft/internal/version.Version=v1.2.3"
var Version = "dev"

// String returns the resolved application version.
func String() string {
	info, ok := debug.ReadBuildInfo()
	return resolve(Version, info, ok)
}

func resolve(override string, info *debug.BuildInfo, ok bool) string {
	override = strings.TrimSpace(override)
	if override != "" && override != "dev" {
		return override
	}
	if ok && info != nil {
		v := strings.TrimSpace(info.Main.Version)
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
