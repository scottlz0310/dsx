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

func TestIsDevelopmentBuildVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "devは開発版",
			input: "dev",
			want:  true,
		},
		{
			name:  "develは開発版",
			input: "(devel)",
			want:  true,
		},
		{
			name:  "空文字は開発版",
			input: "",
			want:  true,
		},
		{
			name:  "タグ版は開発版ではない",
			input: "v0.2.2-alpha",
			want:  false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isDevelopmentBuildVersion(tc.input)
			if got != tc.want {
				t.Fatalf("isDevelopmentBuildVersion(%q) = %v, want %v", tc.input, got, tc.want)
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
	originalGOOS := selfUpdateGOOS
	originalPackaged := selfUpdatePackagedStep

	t.Cleanup(func() {
		version = originalVersion
		selfUpdateCheckStep = originalCheck
		selfUpdateApplyStep = originalApply
		selfUpdateCheckOnly = originalCheckOnly
		selfUpdateGOOS = originalGOOS
		selfUpdatePackagedStep = originalPackaged
	})

	updateAvailable := &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"}

	testCases := []struct {
		name            string
		goos            string
		packaged        bool
		packagedErr     error
		checkOnly       bool
		checkResult     *selfUpdateInfo
		checkErr        error
		applyErr        error
		wantErr         bool
		wantErrContains string
		wantApplyCall   bool
		wantStdout      string
	}{
		{
			name:          "更新ありで適用実行",
			goos:          "linux",
			checkResult:   updateAvailable,
			wantApplyCall: true,
		},
		{
			name:        "check指定時は適用しない",
			goos:        "linux",
			checkOnly:   true,
			checkResult: updateAvailable,
		},
		{
			name: "更新なし",
			goos: "linux",
		},
		{
			name:     "確認失敗",
			goos:     "linux",
			checkErr: errors.New("check failed"),
			wantErr:  true,
		},
		{
			name:          "適用失敗",
			goos:          "linux",
			checkResult:   updateAvailable,
			applyErr:      errors.New("apply failed"),
			wantErr:       true,
			wantApplyCall: true,
		},
		{
			name:        "WindowsのMSIX版はgo installせず自動更新を案内する",
			goos:        "windows",
			packaged:    true,
			checkResult: updateAvailable,
			wantStdout:  "自動更新されます",
		},
		{
			name:            "WindowsのMSIX版以外はgo installせず移行手順付きでエラーにする",
			goos:            "windows",
			checkResult:     updateAvailable,
			wantErr:         true,
			wantErrContains: "MSIX 版へ移行",
		},
		{
			name:            "Windowsでパッケージ判定に失敗したらエラーを返す",
			goos:            "windows",
			packagedErr:     errors.New("api failed"),
			checkResult:     updateAvailable,
			wantErr:         true,
			wantErrContains: "api failed",
		},
		{
			name:        "Windowsでもcheck指定時はパッケージ判定せず終了する",
			goos:        "windows",
			packagedErr: errors.New("must not be called"),
			checkOnly:   true,
			checkResult: updateAvailable,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			version = "v0.2.0"
			selfUpdateCheckOnly = tc.checkOnly
			selfUpdateGOOS = tc.goos

			applyCalled := false
			selfUpdateCheckStep = func(context.Context, string) (*selfUpdateInfo, error) {
				return tc.checkResult, tc.checkErr
			}
			selfUpdateApplyStep = func(_ context.Context, ver string) error {
				applyCalled = true

				if tc.checkResult != nil && ver != tc.checkResult.LatestVersion {
					t.Errorf("apply called with version = %q, want %q", ver, tc.checkResult.LatestVersion)
				}

				return tc.applyErr
			}
			selfUpdatePackagedStep = func() (bool, error) {
				return tc.packaged, tc.packagedErr
			}

			var err error

			stdout := captureStdout(t, func() {
				err = runSelfUpdate(&cobra.Command{Use: "self-update"}, nil)
			})

			if (err != nil) != tc.wantErr {
				t.Fatalf("runSelfUpdate() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("runSelfUpdate() error = %q, want contains %q", err, tc.wantErrContains)
			}

			if applyCalled != tc.wantApplyCall {
				t.Fatalf("apply called = %v, want %v", applyCalled, tc.wantApplyCall)
			}

			if tc.wantStdout != "" && !strings.Contains(stdout, tc.wantStdout) {
				t.Fatalf("stdout = %q, want contains %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestResolveSelfUpdateMethod(t *testing.T) {
	originalGOOS := selfUpdateGOOS
	originalPackaged := selfUpdatePackagedStep

	t.Cleanup(func() {
		selfUpdateGOOS = originalGOOS
		selfUpdatePackagedStep = originalPackaged
	})

	testCases := []struct {
		name        string
		goos        string
		packaged    bool
		packagedErr error
		want        selfUpdateMethod
		wantErr     bool
	}{
		{
			name:        "Linuxはパッケージ判定せずgo install",
			goos:        "linux",
			packagedErr: errors.New("must not be called"),
			want:        selfUpdateByGoInstall,
		},
		{
			name:        "macOSはパッケージ判定せずgo install",
			goos:        "darwin",
			packagedErr: errors.New("must not be called"),
			want:        selfUpdateByGoInstall,
		},
		{
			name:     "WindowsのMSIX版はappinstaller",
			goos:     "windows",
			packaged: true,
			want:     selfUpdateByAppInstaller,
		},
		{
			name: "WindowsのMSIX版以外は非サポート",
			goos: "windows",
			want: selfUpdateUnsupported,
		},
		{
			name:        "Windowsでパッケージ判定失敗はエラー",
			goos:        "windows",
			packagedErr: errors.New("api failed"),
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			selfUpdateGOOS = tc.goos
			selfUpdatePackagedStep = func() (bool, error) {
				return tc.packaged, tc.packagedErr
			}

			got, err := resolveSelfUpdateMethod()
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveSelfUpdateMethod() error = %v, wantErr %v", err, tc.wantErr)
			}

			if !tc.wantErr && got != tc.want {
				t.Fatalf("resolveSelfUpdateMethod() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrintSelfUpdateNoticeAtEnd(t *testing.T) {
	originalCheck := selfUpdateCheckStep
	originalGOOS := selfUpdateGOOS
	originalPackaged := selfUpdatePackagedStep

	t.Cleanup(func() {
		selfUpdateCheckStep = originalCheck
		selfUpdateGOOS = originalGOOS
		selfUpdatePackagedStep = originalPackaged
	})

	updateAvailable := &selfUpdateInfo{CurrentVersion: "v0.2.0", LatestVersion: "v0.3.0"}

	testCases := []struct {
		name        string
		goos        string
		packaged    bool
		packagedErr error
		checkInfo   *selfUpdateInfo
		checkErr    error
		wantText    string
		wantAbsent  string
	}{
		{
			name:      "Windows以外はself-updateを案内",
			goos:      "linux",
			checkInfo: updateAvailable,
			wantText:  "更新コマンド: dsx self-update",
		},
		{
			name:       "WindowsのMSIX版は自動更新を案内",
			goos:       "windows",
			packaged:   true,
			checkInfo:  updateAvailable,
			wantText:   "MSIX 版は自動更新されます",
			wantAbsent: "dsx self-update",
		},
		{
			name:       "WindowsのMSIX版以外は移行を案内",
			goos:       "windows",
			checkInfo:  updateAvailable,
			wantText:   "MSIX 版への移行が必要です",
			wantAbsent: "dsx self-update",
		},
		{
			name:        "パッケージ判定失敗時は更新方法の案内だけ省略",
			goos:        "windows",
			packagedErr: errors.New("api failed"),
			checkInfo:   updateAvailable,
			wantText:    "新しいバージョン",
			wantAbsent:  "MSIX",
		},
		{
			name:     "通知なし",
			goos:     "linux",
			wantText: "",
		},
		{
			name:     "確認エラー時は表示なし",
			goos:     "linux",
			checkErr: errors.New("check failed"),
			wantText: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			selfUpdateGOOS = tc.goos
			selfUpdatePackagedStep = func() (bool, error) {
				return tc.packaged, tc.packagedErr
			}
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

func TestSelfUpdateInstallTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "通常バージョン",
			version: "v0.2.5",
			want:    "github.com/scottlz0310/dsx/cmd/dsx@v0.2.5",
		},
		{
			name:    "パッチバージョン",
			version: "v1.0.0",
			want:    "github.com/scottlz0310/dsx/cmd/dsx@v1.0.0",
		},
		{
			name:    "vプレフィックスなし",
			version: "0.2.5",
			want:    "github.com/scottlz0310/dsx/cmd/dsx@v0.2.5",
		},
		{
			name:    "前後スペースあり",
			version: " v0.2.5 ",
			want:    "github.com/scottlz0310/dsx/cmd/dsx@v0.2.5",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := selfUpdateInstallTarget(tc.version)
			if got != tc.want {
				t.Fatalf("selfUpdateInstallTarget(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}
