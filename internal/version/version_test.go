package version

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		override string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{
			name:     "explicit ldflags override wins over build info",
			override: "v0.4.2",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}},
			ok:       true,
			want:     "v0.4.2",
		},
		{
			name:     "build info used when override is dev",
			override: "dev",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}},
			ok:       true,
			want:     "v0.4.0",
		},
		{
			name:     "build info used when override is empty",
			override: "  ",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}},
			ok:       true,
			want:     "v1.0.0",
		},
		{
			name:     "devel build info falls back to dev",
			override: "dev",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:       true,
			want:     "dev",
		},
		{
			name:     "missing build info falls back to dev",
			override: "dev",
			info:     nil,
			ok:       false,
			want:     "dev",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolve(tc.override, tc.info, tc.ok)
			if got != tc.want {
				t.Fatalf("resolve(%q, %+v, %v) = %q, want %q", tc.override, tc.info, tc.ok, got, tc.want)
			}
		})
	}
}
