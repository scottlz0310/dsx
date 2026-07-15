package updater

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
)

func TestBunUpdaterMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "Name", got: (&BunUpdater{}).Name(), want: "bun"},
		{name: "DisplayName", got: (&BunUpdater{}).DisplayName(), want: "Bun (JavaScript/TypeScript グローバルパッケージ)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("got = %q, want %q", tt.got, tt.want)
			}
		})
	}

	if err := (&BunUpdater{}).Configure(config.ManagerConfig{"dummy": true}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
}

func TestBunUpdaterIsAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lookErr error
		want    bool
	}{
		{name: "bunが見つかる", want: true},
		{name: "bunが見つからない", lookErr: errors.New("not found"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			updater := &BunUpdater{
				lookPathStep: func(name string) (string, error) {
					if name != "bun" {
						t.Fatalf("lookPath name = %q, want bun", name)
					}

					return "/tools/bun", tt.lookErr
				},
			}

			if got := updater.IsAvailable(); got != tt.want {
				t.Fatalf("IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseBunOutdatedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		want        []PackageInfo
		wantErrText string
	}{
		{
			name: "通常出力",
			output: `bun outdated v1.3.14 (0d9b296a)
|---------------------------------------|
| Package  | Current | Update  | Latest |
|----------|---------|---------|--------|
| npm      | 11.7.0  | 11.18.0 | 12.0.1 |
|---------------------------------------|`,
			want: []PackageInfo{{Name: "npm", CurrentVersion: "11.7.0", NewVersion: "12.0.1"}},
		},
		{
			name: "scope付きパッケージと複数行",
			output: `| Package        | Current | Update | Latest |
|----------------|---------|--------|--------|
| @scope/tool    | 1.0.0   | 1.1.0  | 2.0.0  |
| plain-tool     | 3.0.0   | 3.0.1  | 3.0.1  |`,
			want: []PackageInfo{
				{Name: "@scope/tool", CurrentVersion: "1.0.0", NewVersion: "2.0.0"},
				{Name: "plain-tool", CurrentVersion: "3.0.0", NewVersion: "3.0.1"},
			},
		},
		{
			name: "Unicode罫線",
			output: `┌─────────┬─────────┬────────┬────────┐
│ Package │ Current │ Update │ Latest │
├─────────┼─────────┼────────┼────────┤
│ tool    │ 1.0.0   │ 1.1.0  │ 2.0.0  │
└─────────┴─────────┴────────┴────────┘`,
			want: []PackageInfo{{Name: "tool", CurrentVersion: "1.0.0", NewVersion: "2.0.0"}},
		},
		{name: "空出力", output: "", want: []PackageInfo{}},
		{name: "最新版はバナーのみ", output: "bun outdated v1.3.14 (0d9b296a)\n", want: []PackageInfo{}},
		{
			name: "列不足",
			output: `| Package | Current | Update | Latest |
| broken  | 1.0.0   | 2.0.0  |`,
			wantErrText: "列数が不正",
		},
		{
			name: "最新バージョンが空",
			output: `| Package | Current | Update | Latest |
| broken  | 1.0.0   | 1.1.0  |        |`,
			wantErrText: "必須列が空",
		},
		{name: "未知の形式", output: "unexpected output", wantErrText: "ヘッダーを認識できません"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBunOutdatedOutput(tt.output)
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("parseBunOutdatedOutput() error = %v, want contains %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseBunOutdatedOutput() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseBunOutdatedOutput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestClassifyBunInstallOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		bunPath       string
		resolvedPath  string
		bunInstallDir string
		homeDir       string
		scoopDir      string
		want          bunInstallOwner
	}{
		{
			name:          "BUN_INSTALL配下",
			bunPath:       `D:\tools\bun\bin\bun.exe`,
			resolvedPath:  `D:\tools\bun\bin\bun.exe`,
			bunInstallDir: `D:\tools\bun`,
			homeDir:       `C:\Users\dev`,
			want:          bunOwnerSelf,
		},
		{
			name:         "デフォルトのbun管理パス",
			bunPath:      "/home/dev/.bun/bin/bun",
			resolvedPath: "/home/dev/.bun/bin/bun",
			homeDir:      "/home/dev",
			want:         bunOwnerSelf,
		},
		{
			name:         "Homebrewの解決先",
			bunPath:      "/opt/homebrew/bin/bun",
			resolvedPath: "/opt/homebrew/Cellar/bun/1.3.14/bin/bun",
			homeDir:      "/Users/dev",
			want:         bunOwnerHomebrew,
		},
		{
			name:         "Linuxbrewのshim",
			bunPath:      "/home/linuxbrew/.linuxbrew/bin/bun",
			resolvedPath: "/home/linuxbrew/.linuxbrew/bin/bun",
			homeDir:      "/home/dev",
			want:         bunOwnerHomebrew,
		},
		{
			name:         "Scoopのデフォルトshim",
			bunPath:      `C:\Users\dev\scoop\shims\bun.exe`,
			resolvedPath: `C:\Users\dev\scoop\shims\bun.exe`,
			homeDir:      `C:\Users\dev`,
			want:         bunOwnerScoop,
		},
		{
			name:         "Scoopのカスタムルート",
			bunPath:      `D:\packages\shims\bun.exe`,
			resolvedPath: `D:\packages\shims\bun.exe`,
			homeDir:      `C:\Users\dev`,
			scoopDir:     `D:\packages`,
			want:         bunOwnerScoop,
		},
		{
			name:         "所有元不明",
			bunPath:      "/usr/local/bin/bun",
			resolvedPath: "/usr/local/bin/bun",
			homeDir:      "/home/dev",
			want:         bunOwnerUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyBunInstallOwner(tt.bunPath, tt.resolvedPath, tt.bunInstallDir, tt.homeDir, tt.scoopDir)
			if got != tt.want {
				t.Fatalf("classifyBunInstallOwner() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBunUpdaterCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		runErr      error
		wantCount   int
		wantErrText string
	}{
		{
			name:      "更新あり",
			output:    "| Package | Current | Update | Latest |\n| tool | 1.0.0 | 1.1.0 | 2.0.0 |\n",
			wantCount: 1,
		},
		{name: "更新なし", output: "bun outdated v1.3.14\n"},
		{name: "コマンド失敗", runErr: errors.New("network error"), wantErrText: "bun outdated -g の実行に失敗"},
		{name: "解析失敗", output: "invalid", wantErrText: "bun outdated -g の出力解析に失敗"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			updater := &BunUpdater{
				runOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					if strings.Join(args, " ") != "outdated -g" {
						t.Fatalf("args = %v, want [outdated -g]", args)
					}

					return []byte(tt.output), tt.runErr
				},
			}

			got, err := updater.Check(context.Background())
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("Check() error = %v, want contains %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}

			if got.AvailableUpdates != tt.wantCount {
				t.Fatalf("AvailableUpdates = %d, want %d", got.AvailableUpdates, tt.wantCount)
			}
		})
	}
}

func TestBunUpdaterUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		outdatedOutput  string
		dryRun          bool
		interactiveErr  error
		wantRun         bool
		wantUpdated     int
		wantMessageText string
		wantErrText     string
	}{
		{
			name:            "更新なし",
			outdatedOutput:  "bun outdated v1.3.14\n",
			wantMessageText: "最新です",
		},
		{
			name:            "dry-runでは更新しない",
			outdatedOutput:  bunOutdatedOnePackage,
			dryRun:          true,
			wantMessageText: "DryRunモード",
		},
		{
			name:            "latestまで更新",
			outdatedOutput:  bunOutdatedOnePackage,
			wantRun:         true,
			wantUpdated:     1,
			wantMessageText: "更新しました",
		},
		{
			name:           "更新コマンド失敗",
			outdatedOutput: bunOutdatedOnePackage,
			interactiveErr: errors.New("update failed"),
			wantRun:        true,
			wantErrText:    "bun update -g --latest に失敗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runCalled := false
			updater := &BunUpdater{
				runOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					return []byte(tt.outdatedOutput), nil
				},
				runInteractiveStep: func(_ context.Context, args ...string) error {
					runCalled = true

					if strings.Join(args, " ") != "update -g --latest" {
						t.Fatalf("args = %v, want [update -g --latest]", args)
					}

					return tt.interactiveErr
				},
			}

			got, err := updater.Update(context.Background(), UpdateOptions{DryRun: tt.dryRun})
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("Update() error = %v, want contains %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if runCalled != tt.wantRun {
				t.Fatalf("runCalled = %v, want %v", runCalled, tt.wantRun)
			}

			if got.UpdatedCount != tt.wantUpdated {
				t.Fatalf("UpdatedCount = %d, want %d", got.UpdatedCount, tt.wantUpdated)
			}

			if !strings.Contains(got.Message, tt.wantMessageText) {
				t.Fatalf("Message = %q, want contains %q", got.Message, tt.wantMessageText)
			}
		})
	}
}

func TestBunUpdaterCheckSelfUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		owner            bunInstallOwner
		currentVersion   string
		latestVersion    string
		versionErr       error
		fetchErr         error
		wantCount        int
		wantMessageText  string
		wantErrText      string
		wantVersionCalls int
		wantFetchCalls   int
	}{
		{name: "Homebrew管理", owner: bunOwnerHomebrew, wantMessageText: "Homebrew 管理"},
		{name: "Scoop管理", owner: bunOwnerScoop, wantMessageText: "Scoop 管理"},
		{name: "所有元不明", owner: bunOwnerUnknown, wantMessageText: "安全に判定できない"},
		{
			name:             "Bun管理で最新版",
			owner:            bunOwnerSelf,
			currentVersion:   "1.3.14\n",
			latestVersion:    "bun-v1.3.14",
			wantMessageText:  "最新です",
			wantVersionCalls: 1,
			wantFetchCalls:   1,
		},
		{
			name:             "Bun管理で更新あり",
			owner:            bunOwnerSelf,
			currentVersion:   "1.3.13\n",
			latestVersion:    "bun-v1.3.14",
			wantCount:        1,
			wantMessageText:  "更新が可能",
			wantVersionCalls: 1,
			wantFetchCalls:   1,
		},
		{
			name:             "現在バージョン取得失敗",
			owner:            bunOwnerSelf,
			versionErr:       errors.New("version failed"),
			wantErrText:      "bun --version の実行に失敗",
			wantVersionCalls: 1,
		},
		{
			name:             "現在バージョン不正",
			owner:            bunOwnerSelf,
			currentVersion:   "invalid",
			wantErrText:      "現在バージョンを解析できません",
			wantVersionCalls: 1,
		},
		{
			name:             "最新リリース取得失敗",
			owner:            bunOwnerSelf,
			currentVersion:   "1.3.13",
			fetchErr:         errors.New("api failed"),
			wantErrText:      "最新リリース取得に失敗",
			wantVersionCalls: 1,
			wantFetchCalls:   1,
		},
		{
			name:             "最新バージョン不正",
			owner:            bunOwnerSelf,
			currentVersion:   "1.3.13",
			latestVersion:    "invalid",
			wantErrText:      "最新バージョンを解析できません",
			wantVersionCalls: 1,
			wantFetchCalls:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			versionCalls := 0
			fetchCalls := 0
			updater := &BunUpdater{
				lookPathStep:    func(string) (string, error) { return "/tools/bun", nil },
				detectOwnerStep: func(string) bunInstallOwner { return tt.owner },
				runOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					versionCalls++

					if strings.Join(args, " ") != "--version" {
						t.Fatalf("args = %v, want [--version]", args)
					}

					return []byte(tt.currentVersion), tt.versionErr
				},
				fetchReleaseStep: func(_ context.Context, _ string) (string, string, error) {
					fetchCalls++

					return tt.latestVersion, "https://example.com", tt.fetchErr
				},
			}

			got, err := updater.CheckSelfUpdate(context.Background())
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("CheckSelfUpdate() error = %v, want contains %q", err, tt.wantErrText)
				}
			} else {
				if err != nil {
					t.Fatalf("CheckSelfUpdate() error = %v", err)
				}

				if got.AvailableUpdates != tt.wantCount {
					t.Fatalf("AvailableUpdates = %d, want %d", got.AvailableUpdates, tt.wantCount)
				}

				if !strings.Contains(got.Message, tt.wantMessageText) {
					t.Fatalf("Message = %q, want contains %q", got.Message, tt.wantMessageText)
				}
			}

			if versionCalls != tt.wantVersionCalls {
				t.Fatalf("versionCalls = %d, want %d", versionCalls, tt.wantVersionCalls)
			}

			if fetchCalls != tt.wantFetchCalls {
				t.Fatalf("fetchCalls = %d, want %d", fetchCalls, tt.wantFetchCalls)
			}
		})
	}
}

func TestBunUpdaterSelfUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		owner           bunInstallOwner
		dryRun          bool
		interactiveErr  error
		wantRun         bool
		wantUpdated     int
		wantMessageText string
		wantErrText     string
	}{
		{
			name:            "所有元不明はスキップして継続",
			owner:           bunOwnerUnknown,
			wantMessageText: "安全に判定できない",
		},
		{
			name:            "dry-runではupgradeしない",
			owner:           bunOwnerSelf,
			dryRun:          true,
			wantMessageText: "DryRunモード",
		},
		{
			name:            "bun upgradeを実行",
			owner:           bunOwnerSelf,
			wantRun:         true,
			wantUpdated:     1,
			wantMessageText: "更新しました",
		},
		{
			name:           "bun upgrade失敗",
			owner:          bunOwnerSelf,
			interactiveErr: errors.New("upgrade failed"),
			wantRun:        true,
			wantErrText:    "bun upgrade に失敗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runCalled := false
			updater := &BunUpdater{
				lookPathStep:    func(string) (string, error) { return "/tools/bun", nil },
				detectOwnerStep: func(string) bunInstallOwner { return tt.owner },
				runOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					if strings.Join(args, " ") != "--version" {
						return nil, fmt.Errorf("unexpected args: %v", args)
					}

					return []byte("1.3.13"), nil
				},
				fetchReleaseStep: func(context.Context, string) (string, string, error) {
					return "bun-v1.3.14", "https://example.com", nil
				},
				runInteractiveStep: func(_ context.Context, args ...string) error {
					runCalled = true

					if strings.Join(args, " ") != "upgrade" {
						t.Fatalf("args = %v, want [upgrade]", args)
					}

					return tt.interactiveErr
				},
			}

			got, err := updater.SelfUpdate(context.Background(), UpdateOptions{DryRun: tt.dryRun})
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("SelfUpdate() error = %v, want contains %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("SelfUpdate() error = %v", err)
			}

			if runCalled != tt.wantRun {
				t.Fatalf("runCalled = %v, want %v", runCalled, tt.wantRun)
			}

			if got.UpdatedCount != tt.wantUpdated {
				t.Fatalf("UpdatedCount = %d, want %d", got.UpdatedCount, tt.wantUpdated)
			}

			if got.Continuation != ContinueNormalUpdate {
				t.Fatalf("Continuation = %q, want %q", got.Continuation, ContinueNormalUpdate)
			}

			if !strings.Contains(got.Message, tt.wantMessageText) {
				t.Fatalf("Message = %q, want contains %q", got.Message, tt.wantMessageText)
			}
		})
	}
}

const bunOutdatedOnePackage = `| Package | Current | Update | Latest |
| tool    | 1.0.0   | 1.1.0 | 2.0.0  |
`
