package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseSemverCore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		input  string
		want   semverCore
		wantOK bool
	}{
		{
			name:   "通常のv付きsemver",
			input:  "v1.2.3",
			want:   semverCore{Major: 1, Minor: 2, Patch: 3},
			wantOK: true,
		},
		{
			name:   "vなしsemver",
			input:  "2.10.5",
			want:   semverCore{Major: 2, Minor: 10, Patch: 5},
			wantOK: true,
		},
		{
			name:   "プレリリース付き",
			input:  "v0.3.0-alpha.1",
			want:   semverCore{Major: 0, Minor: 3, Patch: 0},
			wantOK: true,
		},
		{
			name:   "ビルドメタ付き",
			input:  "v1.0.0+build.5",
			want:   semverCore{Major: 1, Minor: 0, Patch: 0},
			wantOK: true,
		},
		{
			name:   "要素不足は失敗",
			input:  "v1.2",
			wantOK: false,
		},
		{
			name:   "文字列要素は失敗",
			input:  "v1.x.3",
			wantOK: false,
		},
		{
			name:   "空文字は失敗",
			input:  "",
			wantOK: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseSemverCore(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseSemverCore(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}

			if !tc.wantOK {
				return
			}

			if got != tc.want {
				t.Fatalf("parseSemverCore(%q) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompareSemverCore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		left  semverCore
		right semverCore
		want  int
	}{
		{
			name:  "majorが大きい",
			left:  semverCore{Major: 2, Minor: 0, Patch: 0},
			right: semverCore{Major: 1, Minor: 9, Patch: 9},
			want:  1,
		},
		{
			name:  "minorが小さい",
			left:  semverCore{Major: 1, Minor: 2, Patch: 9},
			right: semverCore{Major: 1, Minor: 3, Patch: 0},
			want:  -1,
		},
		{
			name:  "patchが同一",
			left:  semverCore{Major: 0, Minor: 8, Patch: 1},
			right: semverCore{Major: 0, Minor: 8, Patch: 1},
			want:  0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := compareSemverCore(tc.left, tc.right)
			if got != tc.want {
				t.Fatalf("compareSemverCore(%#v, %#v) = %d, want %d", tc.left, tc.right, got, tc.want)
			}
		})
	}
}

func TestCheckSelfUpdateAvailable(t *testing.T) {
	originalFetch := selfUpdateFetchReleaseStep
	t.Cleanup(func() {
		selfUpdateFetchReleaseStep = originalFetch
	})

	testCases := []struct {
		name           string
		currentVersion string
		fetchVersion   string
		fetchURL       string
		fetchErr       error
		wantNotice     bool
		wantErr        bool
		wantFetchCall  bool
	}{
		{
			name:           "dev版は通知しない",
			currentVersion: "dev",
			wantNotice:     false,
			wantErr:        false,
			wantFetchCall:  false,
		},
		{
			name:           "新バージョンあり",
			currentVersion: "v0.2.0",
			fetchVersion:   "v0.3.0",
			fetchURL:       "https://example.com/release",
			wantNotice:     true,
			wantErr:        false,
			wantFetchCall:  true,
		},
		{
			name:           "同一バージョンは通知しない",
			currentVersion: "v0.3.0",
			fetchVersion:   "v0.3.0",
			wantNotice:     false,
			wantErr:        false,
			wantFetchCall:  true,
		},
		{
			name:           "取得失敗はエラー",
			currentVersion: "v0.2.0",
			fetchErr:       errors.New("network error"),
			wantNotice:     false,
			wantErr:        true,
			wantFetchCall:  true,
		},
		{
			name:           "現在バージョン不正は通知しない",
			currentVersion: "invalid",
			fetchVersion:   "v0.3.0",
			wantNotice:     false,
			wantErr:        false,
			wantFetchCall:  false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			called := false
			selfUpdateFetchReleaseStep = func(context.Context) (string, string, error) {
				called = true
				return tc.fetchVersion, tc.fetchURL, tc.fetchErr
			}

			got, err := checkSelfUpdateAvailable(context.Background(), tc.currentVersion)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkSelfUpdateAvailable() error = %v, wantErr %v", err, tc.wantErr)
			}

			if called != tc.wantFetchCall {
				t.Fatalf("fetch called = %v, want %v", called, tc.wantFetchCall)
			}

			if (got != nil) != tc.wantNotice {
				t.Fatalf("notice exists = %v, want %v", got != nil, tc.wantNotice)
			}
		})
	}
}

func TestRunSelfUpdate(t *testing.T) {
	originalVersion := version
	originalCheck := selfUpdateCheckStep
	originalApply := selfUpdateApplyStep
	originalCheckOnly := selfUpdateCheckOnly

	t.Cleanup(func() {
		version = originalVersion
		selfUpdateCheckStep = originalCheck
		selfUpdateApplyStep = originalApply
		selfUpdateCheckOnly = originalCheckOnly
	})

	testCases := []struct {
		name          string
		checkOnly     bool
		checkResult   *selfUpdateInfo
		checkErr      error
		applyErr      error
		wantErr       bool
		wantApplyCall bool
	}{
		{
			name:          "更新ありで適用実行",
			checkOnly:     false,
			checkResult:   &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"},
			wantErr:       false,
			wantApplyCall: true,
		},
		{
			name:          "check指定時は適用しない",
			checkOnly:     true,
			checkResult:   &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"},
			wantErr:       false,
			wantApplyCall: false,
		},
		{
			name:          "更新なし",
			checkOnly:     false,
			checkResult:   nil,
			wantErr:       false,
			wantApplyCall: false,
		},
		{
			name:          "確認失敗",
			checkOnly:     false,
			checkErr:      errors.New("check failed"),
			wantErr:       true,
			wantApplyCall: false,
		},
		{
			name:          "適用失敗",
			checkOnly:     false,
			checkResult:   &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"},
			applyErr:      errors.New("apply failed"),
			wantErr:       true,
			wantApplyCall: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			version = "v0.2.0"
			selfUpdateCheckOnly = tc.checkOnly

			applyCalled := false
			selfUpdateCheckStep = func(context.Context, string) (*selfUpdateInfo, error) {
				return tc.checkResult, tc.checkErr
			}
			selfUpdateApplyStep = func(context.Context) error {
				applyCalled = true
				return tc.applyErr
			}

			err := runSelfUpdate(&cobra.Command{Use: "self-update"}, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("runSelfUpdate() error = %v, wantErr %v", err, tc.wantErr)
			}

			if applyCalled != tc.wantApplyCall {
				t.Fatalf("apply called = %v, want %v", applyCalled, tc.wantApplyCall)
			}
		})
	}
}

func TestPrintSelfUpdateNoticeAtEnd(t *testing.T) {
	originalCheck := selfUpdateCheckStep
	t.Cleanup(func() {
		selfUpdateCheckStep = originalCheck
	})

	testCases := []struct {
		name      string
		checkInfo *selfUpdateInfo
		checkErr  error
		wantText  string
	}{
		{
			name:      "通知あり",
			checkInfo: &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"},
			wantText:  "新しいバージョン",
		},
		{
			name:      "通知なし",
			checkInfo: nil,
			wantText:  "",
		},
		{
			name:     "確認エラー時は表示なし",
			checkErr: errors.New("check failed"),
			wantText: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			selfUpdateCheckStep = func(context.Context, string) (*selfUpdateInfo, error) {
				return tc.checkInfo, tc.checkErr
			}

			got := captureStdout(t, func() {
				printSelfUpdateNoticeAtEnd()
			})

			if tc.wantText == "" {
				if strings.TrimSpace(got) != "" {
					t.Fatalf("stdout = %q, want empty", got)
				}

				return
			}

			if !strings.Contains(got, tc.wantText) {
				t.Fatalf("stdout = %q, want contains %q", got, tc.wantText)
			}
		})
	}
}
