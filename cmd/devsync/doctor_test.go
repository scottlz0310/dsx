package main

import (
	"strings"
	"testing"
)

func TestBuildDoctorConfigStatusMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		exists     bool
		path       string
		wantPhrase string
	}{
		{
			name:       "設定ファイルあり",
			exists:     true,
			path:       "/tmp/config.yaml",
			wantPhrase: "設定ファイルを読み込みました",
		},
		{
			name:       "設定ファイルなし",
			exists:     false,
			path:       "/tmp/config.yaml",
			wantPhrase: "設定ファイルは未作成です",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildDoctorConfigStatusMessage(tc.exists, tc.path)
			if !strings.Contains(got, tc.wantPhrase) {
				t.Fatalf("buildDoctorConfigStatusMessage() = %q, want contains %q", got, tc.wantPhrase)
			}

			if !strings.Contains(got, tc.path) {
				t.Fatalf("buildDoctorConfigStatusMessage() = %q, want contains path %q", got, tc.path)
			}
		})
	}
}
