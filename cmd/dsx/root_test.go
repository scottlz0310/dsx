package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	originalReadBuildInfo := readBuildInfoStep
	t.Cleanup(func() {
		readBuildInfoStep = originalReadBuildInfo
	})

	testCases := []struct {
		name           string
		currentVersion string
		buildInfo      *debug.BuildInfo
		buildInfoOK    bool
		wantVersion    string
		wantReadCall   bool
	}{
		{
			name:           "ldflags注入済みはそのまま利用",
			currentVersion: "v0.2.2-alpha",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}},
			buildInfoOK:    true,
			wantVersion:    "v0.2.2-alpha",
			wantReadCall:   false,
		},
		{
			name:           "dev版はbuildinfoのバージョンへフォールバック",
			currentVersion: "dev",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: "v0.2.2-alpha"}},
			buildInfoOK:    true,
			wantVersion:    "v0.2.2-alpha",
			wantReadCall:   true,
		},
		{
			name:           "devel版はbuildinfoが取れなければそのまま",
			currentVersion: "(devel)",
			buildInfoOK:    false,
			wantVersion:    "(devel)",
			wantReadCall:   true,
		},
		{
			name:           "buildinfoのバージョン空文字はそのまま",
			currentVersion: "dev",
			buildInfo:      &debug.BuildInfo{Main: debug.Module{Version: ""}},
			buildInfoOK:    true,
			wantVersion:    "dev",
			wantReadCall:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			readCalled := false
			readBuildInfoStep = func() (*debug.BuildInfo, bool) {
				readCalled = true
				return tc.buildInfo, tc.buildInfoOK
			}

			got := resolveVersion(tc.currentVersion)
			if got != tc.wantVersion {
				t.Fatalf("resolveVersion(%q) = %q, want %q", tc.currentVersion, got, tc.wantVersion)
			}

			if readCalled != tc.wantReadCall {
				t.Fatalf("readBuildInfoStep called = %v, want %v", readCalled, tc.wantReadCall)
			}
		})
	}
}
