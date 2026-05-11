package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/selfupdate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type selfupdateInfoForTest struct {
	CurrentVersion string
	LatestVersion  string
}

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "version指定あり",
			input:    "github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
			expected: "golangci-lint",
		},
		{
			name:     "version指定なし",
			input:    "golang.org/x/tools/gopls",
			expected: "gopls",
		},
		{
			name:     "単一セグメント",
			input:    "dlv@latest",
			expected: "dlv",
		},
		{
			name:     "単一セグメントでversion指定なし",
			input:    "gotests",
			expected: "gotests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractToolName(tt.input))
		})
	}
}

func TestDefaultGoTargets(t *testing.T) {
	targets := DefaultGoTargets()
	require.NotEmpty(t, targets)

	for _, target := range targets {
		assert.Contains(t, target, "@", "デフォルトターゲットは@version指定を含む想定")
	}
}

func TestGoUpdater_Configure(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.ManagerConfig
		initialTargets []string
		expected       []string
	}{
		{
			name:           "nil設定は何もしない",
			cfg:            nil,
			initialTargets: []string{"a@latest"},
			expected:       []string{"a@latest"},
		},
		{
			name:           "targetsが[]interface{}",
			cfg:            config.ManagerConfig{"targets": []interface{}{"a@latest", 123, "b"}},
			initialTargets: nil,
			expected:       []string{"a@latest", "b"},
		},
		{
			name:           "targetsが[]string",
			cfg:            config.ManagerConfig{"targets": []string{"a@latest", "b@latest"}},
			initialTargets: nil,
			expected:       []string{"a@latest", "b@latest"},
		},
		{
			name:           "targetsの型が不正なら無視",
			cfg:            config.ManagerConfig{"targets": "a@latest"},
			initialTargets: []string{"keep"},
			expected:       []string{"keep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GoUpdater{targets: tt.initialTargets}
			require.NoError(t, g.Configure(tt.cfg))
			assert.Equal(t, tt.expected, g.targets)
		})
	}
}

func TestGoUpdater_Check(t *testing.T) {
	t.Run("更新対象なし", func(t *testing.T) {
		g := &GoUpdater{}

		got, err := g.Check(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Equal(t, 0, got.AvailableUpdates)
		assert.Contains(t, got.Message, "targets は未設定")
		assert.Empty(t, got.Packages)
	})

	tests := []struct {
		name              string
		targets           []string
		detected          []GoBinaryInfo
		latestVersions    map[string]string
		latestErr         error
		currentVersion    string
		selfUpdateInfo    *selfupdateInfoForTest
		selfUpdateErr     error
		wantErrContains   string
		wantUpdates       int
		wantPackageNames  []string
		wantNewVersions   []string
		wantMessageParts  []string
		wantLatestCallMap map[string]int
	}{
		{
			name:    "installedとlatestが一致する場合はスキップ",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				ModulePath:       "github.com/foo",
				InstalledVersion: "v1.0.0",
			}},
			latestVersions:   map[string]string{"github.com/foo": "v1.0.0"},
			wantUpdates:      0,
			wantMessageParts: []string{"更新対象はありません", "スキップ: 1 件"},
		},
		{
			name:    "installedとlatestが異なる場合は更新対象",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				ModulePath:       "github.com/foo",
				InstalledVersion: "v1.0.0",
			}},
			latestVersions:   map[string]string{"github.com/foo": "v1.1.0"},
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"v1.1.0"},
			wantMessageParts: []string{"更新予定: 1 件", "スキップ: 0 件"},
		},
		{
			name:             "対応バイナリなしは判定不能として更新対象",
			targets:          []string{"github.com/foo/missing"},
			latestVersions:   map[string]string{},
			wantUpdates:      1,
			wantPackageNames: []string{"missing"},
			wantNewVersions:  []string{"@latest"},
			wantMessageParts: []string{"判定不能または固定バージョン: 1 件"},
		},
		{
			name:    "ModulePath空は判定不能として更新対象",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				InstalledVersion: "v1.0.0",
			}},
			latestVersions:   map[string]string{},
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"@latest"},
		},
		{
			name:    "InstalledVersionがdevelなら判定不能として更新対象",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				ModulePath:       "github.com/foo",
				InstalledVersion: "(devel)",
			}},
			latestVersions:   map[string]string{},
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"@latest"},
		},
		{
			name:    "go list失敗は判定不能として更新対象",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				ModulePath:       "github.com/foo",
				InstalledVersion: "v1.0.0",
			}},
			latestErr:        errors.New("network error"),
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"@latest"},
		},
		{
			name:    "latest version空は判定不能として更新対象",
			targets: []string{"github.com/foo/bar@latest"},
			detected: []GoBinaryInfo{{
				PackagePath:      "github.com/foo/bar",
				ModulePath:       "github.com/foo",
				InstalledVersion: "v1.0.0",
			}},
			latestVersions:   map[string]string{"github.com/foo": ""},
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"@latest"},
		},
		{
			name:             "固定バージョンtargetは従来通り更新対象",
			targets:          []string{"github.com/foo/bar@v1.2.3"},
			latestVersions:   map[string]string{},
			wantUpdates:      1,
			wantPackageNames: []string{"bar"},
			wantNewVersions:  []string{"v1.2.3"},
			wantMessageParts: []string{"判定不能または固定バージョン: 1 件"},
		},
		{
			name:    "同一ModulePathはlatest取得をキャッシュする",
			targets: []string{"github.com/foo/cmd/a@latest", "github.com/foo/cmd/b@latest"},
			detected: []GoBinaryInfo{
				{PackagePath: "github.com/foo/cmd/a", ModulePath: "github.com/foo", InstalledVersion: "v1.0.0"},
				{PackagePath: "github.com/foo/cmd/b", ModulePath: "github.com/foo", InstalledVersion: "v1.0.0"},
			},
			latestVersions:    map[string]string{"github.com/foo": "v1.1.0"},
			wantUpdates:       2,
			wantPackageNames:  []string{"a", "b"},
			wantNewVersions:   []string{"v1.1.0", "v1.1.0"},
			wantLatestCallMap: map[string]int{"github.com/foo": 1},
		},
		{
			name:           "dsx本体が最新版なら非エラースキップ",
			targets:        []string{"github.com/scottlz0310/dsx/cmd/dsx@latest"},
			currentVersion: "v0.4.1",
			selfUpdateInfo: nil,
			wantUpdates:    0,
			wantMessageParts: []string{
				"更新対象はありません",
				"スキップ: 1 件",
			},
		},
		{
			name:            "dsx本体の更新ありはself-update誘導エラー",
			targets:         []string{"github.com/scottlz0310/dsx/cmd/dsx@latest"},
			currentVersion:  "v0.4.1",
			selfUpdateInfo:  &selfupdateInfoForTest{CurrentVersion: "v0.4.1", LatestVersion: "v0.4.2"},
			wantErrContains: "dsx self-update",
		},
		{
			name:           "dsx本体の更新判定失敗は非エラースキップ",
			targets:        []string{"github.com/scottlz0310/dsx/cmd/dsx@latest"},
			currentVersion: "v0.4.1",
			selfUpdateErr:  errors.New("api error"),
			wantUpdates:    0,
			wantMessageParts: []string{
				"更新対象はありません",
				"スキップ: 1 件",
			},
		},
		{
			name:            "空targetは設定エラー",
			targets:         []string{"  "},
			wantErrContains: "go.targets に空の target",
		},
		{
			name:            "package空targetは設定エラー",
			targets:         []string{"@latest"},
			wantErrContains: "go.targets に不正な target",
		},
		{
			name:            "version空targetは設定エラー",
			targets:         []string{"github.com/foo/bar@"},
			wantErrContains: "go.targets に不正な target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			latestCalls := make(map[string]int)
			g := newTestGoUpdater(tt.targets, tt.detected)
			g.currentVersion = tt.currentVersion
			g.latestModuleVersion = func(_ context.Context, modulePath string) (string, error) {
				latestCalls[modulePath]++

				if tt.latestErr != nil {
					return "", tt.latestErr
				}

				return tt.latestVersions[modulePath], nil
			}

			g.selfUpdateCheck = func(_ context.Context, currentVersion string) (*selfupdate.Info, error) {
				if tt.selfUpdateErr != nil {
					return nil, tt.selfUpdateErr
				}

				if tt.selfUpdateInfo == nil {
					return nil, nil
				}

				return &selfupdate.Info{
					CurrentVersion: tt.selfUpdateInfo.CurrentVersion,
					LatestVersion:  tt.selfUpdateInfo.LatestVersion,
				}, nil
			}

			got, err := g.Check(context.Background())
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantUpdates, got.AvailableUpdates)
			assertPackageNames(t, got.Packages, tt.wantPackageNames)
			assertPackageNewVersions(t, got.Packages, tt.wantNewVersions)

			for _, part := range tt.wantMessageParts {
				assert.Contains(t, got.Message, part)
			}

			for modulePath, wantCalls := range tt.wantLatestCallMap {
				assert.Equal(t, wantCalls, latestCalls[modulePath], "modulePath=%s", modulePath)
			}
		})
	}
}

func TestGoUpdater_Check_MalformedTargetDoesNotDiscover(t *testing.T) {
	g := &GoUpdater{
		targets: []string{"@latest"},
		discoverGoBinaries: func(context.Context) (*DiscoverResult, error) {
			t.Fatal("不正な target は Go バイナリ検出前にエラーにする")
			return nil, nil
		},
	}

	got, err := g.Check(context.Background())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "go.targets に不正な target")
}

func TestGoUpdater_Check_DiscoveryWarning(t *testing.T) {
	g := &GoUpdater{
		targets: []string{
			"github.com/foo/bar@latest",
			"github.com/scottlz0310/dsx/cmd/dsx@latest",
		},
		discoverGoBinaries: func(context.Context) (*DiscoverResult, error) {
			return nil, errors.New("discover failed")
		},
		selfUpdateCheck: func(context.Context, string) (*selfupdate.Info, error) {
			return nil, nil
		},
	}

	got, err := g.Check(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Contains(t, got.Message, "dsx 本体を除く @latest target は判定不能")
	assert.NotContains(t, got.Message, "全 target")
	assert.Equal(t, 1, got.AvailableUpdates)
	assertPackageNames(t, got.Packages, []string{"bar"})
}

func TestGoUpdater_Update_DryRun(t *testing.T) {
	t.Run("更新対象なし", func(t *testing.T) {
		g := &GoUpdater{}

		got, err := g.Update(context.Background(), UpdateOptions{DryRun: true})
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Contains(t, got.Message, "targets は未設定")
		assert.Contains(t, got.Message, "dsx sys discover")
		assert.Contains(t, got.Message, "go.targets")
		assert.Empty(t, got.Packages)
		assert.Equal(t, 0, got.UpdatedCount)
	})

	t.Run("更新対象あり", func(t *testing.T) {
		g := newTestGoUpdater([]string{
			"golang.org/x/tools/gopls@latest",
			"github.com/fatih/gomodifytags",
		}, nil)

		got, err := g.Update(context.Background(), UpdateOptions{DryRun: true})
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Contains(t, got.Message, "DryRun")
		require.Len(t, got.Packages, 2)

		assert.Equal(t, "gopls", got.Packages[0].Name)
		assert.Equal(t, "@latest", got.Packages[0].NewVersion)
		assert.Equal(t, "gomodifytags", got.Packages[1].Name)
		assert.Equal(t, "@latest", got.Packages[1].NewVersion)
	})
}

func TestGoUpdater_Update_InstallsOnlyPlannedTargets(t *testing.T) {
	g := newTestGoUpdater([]string{
		"github.com/foo/latest@latest",
		"github.com/foo/update@latest",
		"github.com/foo/missing",
		"github.com/foo/pinned@v1.2.3",
		"github.com/scottlz0310/dsx/cmd/dsx@latest",
	}, []GoBinaryInfo{
		{PackagePath: "github.com/foo/latest", ModulePath: "github.com/foo/latestmod", InstalledVersion: "v1.0.0"},
		{PackagePath: "github.com/foo/update", ModulePath: "github.com/foo/updatemod", InstalledVersion: "v1.0.0"},
	})

	g.currentVersion = "v0.4.1"
	g.latestModuleVersion = func(_ context.Context, modulePath string) (string, error) {
		switch modulePath {
		case "github.com/foo/latestmod":
			return "v1.0.0", nil
		case "github.com/foo/updatemod":
			return "v1.1.0", nil
		default:
			return "", nil
		}
	}
	g.selfUpdateCheck = func(context.Context, string) (*selfupdate.Info, error) {
		return nil, nil
	}

	var installed []string

	g.installGoTarget = func(_ context.Context, target string) error {
		installed = append(installed, target)
		return nil
	}

	got, err := g.Update(context.Background(), UpdateOptions{})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, []string{
		"github.com/foo/update@latest",
		"github.com/foo/missing@latest",
		"github.com/foo/pinned@v1.2.3",
	}, installed)
	assert.Equal(t, 3, got.UpdatedCount)
	assert.Equal(t, 0, got.FailedCount)
	assertPackageNames(t, got.Packages, []string{"update", "missing", "pinned"})
	assertPackageNewVersions(t, got.Packages, []string{"v1.1.0", "@latest", "v1.2.3"})
	assert.Contains(t, got.Message, "更新: 3 件")
	assert.Contains(t, got.Message, "スキップ: 2 件")
}

func TestGoUpdater_Update_DSXSelfTargetUpdateAvailable(t *testing.T) {
	g := newTestGoUpdater([]string{"github.com/scottlz0310/dsx/cmd/dsx@latest"}, nil)
	g.selfUpdateCheck = func(context.Context, string) (*selfupdate.Info, error) {
		return &selfupdate.Info{
			CurrentVersion: "v0.4.1",
			LatestVersion:  "v0.4.2",
		}, nil
	}

	installed := false
	g.installGoTarget = func(context.Context, string) error {
		installed = true
		return nil
	}

	got, err := g.Update(context.Background(), UpdateOptions{CurrentVersion: "v0.4.1"})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "dsx self-update")
	assert.False(t, installed)
}

func TestGoUpdater_Update_InstallFailure(t *testing.T) {
	g := newTestGoUpdater([]string{"github.com/foo/bar"}, nil)
	g.installGoTarget = func(context.Context, string) error {
		return errors.New("install failed")
	}

	got, err := g.Update(context.Background(), UpdateOptions{})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, 0, got.UpdatedCount)
	assert.Equal(t, 1, got.FailedCount)
	require.Len(t, got.Errors, 1)
	assert.Contains(t, got.Errors[0].Error(), "install failed")
	assert.Contains(t, got.Message, "1 件失敗")
}

func TestParseGoTarget(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		want          parsedGoTarget
		wantErrSubstr string
	}{
		{
			name: "version指定なしはlatest比較target",
			raw:  " github.com/foo/bar ",
			want: parsedGoTarget{
				Raw:           "github.com/foo/bar",
				PackagePath:   "github.com/foo/bar",
				InstallTarget: "github.com/foo/bar@latest",
				CompareLatest: true,
			},
		},
		{
			name: "latest指定あり",
			raw:  "github.com/foo/bar@latest",
			want: parsedGoTarget{
				Raw:           "github.com/foo/bar@latest",
				PackagePath:   "github.com/foo/bar",
				Version:       "latest",
				InstallTarget: "github.com/foo/bar@latest",
				CompareLatest: true,
			},
		},
		{
			name: "固定バージョン指定あり",
			raw:  "github.com/foo/bar@v1.2.3",
			want: parsedGoTarget{
				Raw:           "github.com/foo/bar@v1.2.3",
				PackagePath:   "github.com/foo/bar",
				Version:       "v1.2.3",
				InstallTarget: "github.com/foo/bar@v1.2.3",
				CompareLatest: false,
			},
		},
		{
			name:          "空文字列はエラー",
			raw:           "",
			wantErrSubstr: "go.targets に空の target",
		},
		{
			name:          "空白のみはエラー",
			raw:           " \t ",
			wantErrSubstr: "go.targets に空の target",
		},
		{
			name:          "package空はエラー",
			raw:           "@latest",
			wantErrSubstr: "go.targets に不正な target",
		},
		{
			name:          "version空はエラー",
			raw:           "github.com/foo/bar@",
			wantErrSubstr: "go.targets に不正な target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGoTarget(tt.raw)
			if tt.wantErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubstr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseGoListModuleVersion(t *testing.T) {
	tests := []struct {
		name            string
		output          []byte
		wantVersion     string
		wantErrContains []string
	}{
		{
			name:        "Versionをtrimして返す",
			output:      []byte(`{"Version":" v1.2.3 "}`),
			wantVersion: "v1.2.3",
		},
		{
			name: "解析エラーは出力内容を含める",
			output: []byte(`{
  "Version":`),
			wantErrContains: []string{
				"go list -m -json の解析に失敗",
				`"Version":`,
			},
		},
		{
			name:   "空JSONなら空文字列",
			output: []byte(`{}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGoListModuleVersion(tt.output)
			if len(tt.wantErrContains) > 0 {
				require.Error(t, err)

				for _, part := range tt.wantErrContains {
					assert.Contains(t, err.Error(), part)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, got)
		})
	}
}

func newTestGoUpdater(targets []string, detected []GoBinaryInfo) *GoUpdater {
	return &GoUpdater{
		targets: targets,
		discoverGoBinaries: func(context.Context) (*DiscoverResult, error) {
			return &DiscoverResult{Detected: detected}, nil
		},
		latestModuleVersion: func(context.Context, string) (string, error) {
			return "", nil
		},
		installGoTarget: func(context.Context, string) error {
			return nil
		},
		selfUpdateCheck: func(context.Context, string) (*selfupdate.Info, error) {
			return nil, nil
		},
	}
}

func assertPackageNames(t *testing.T, packages []PackageInfo, want []string) {
	t.Helper()

	if want == nil {
		assert.Empty(t, packages)
		return
	}

	require.Len(t, packages, len(want))

	for i, name := range want {
		assert.Equal(t, name, packages[i].Name)
	}
}

func assertPackageNewVersions(t *testing.T, packages []PackageInfo, want []string) {
	t.Helper()

	if want == nil {
		assert.Empty(t, packages)
		return
	}

	require.Len(t, packages, len(want))

	for i, version := range want {
		assert.Equal(t, version, packages[i].NewVersion)
	}
}

func TestListInstalledGoTools(t *testing.T) {
	t.Run("GOBIN配下のファイルを列挙する", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "tool1"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tool2.exe"), []byte(""), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "somedir"), 0o755))

		t.Setenv("GOBIN", dir)

		got, err := ListInstalledGoTools()
		require.NoError(t, err)

		sort.Strings(got)
		assert.Equal(t, []string{"tool1", "tool2.exe"}, got)
	})

	t.Run("GOBINが存在しない場合はエラー", func(t *testing.T) {
		t.Setenv("GOBIN", filepath.Join(t.TempDir(), "missing"))

		_, err := ListInstalledGoTools()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "$GOBIN")
	})
}

func TestGoBinaryInfo_UpdateTarget(t *testing.T) {
	tests := []struct {
		name     string
		info     *GoBinaryInfo
		expected string
	}{
		{
			name:     "PackagePathあり",
			info:     &GoBinaryInfo{PackagePath: "github.com/foo/bar"},
			expected: "github.com/foo/bar@latest",
		},
		{
			name:     "PackagePathが空",
			info:     &GoBinaryInfo{PackagePath: ""},
			expected: "",
		},
		{
			name:     "nilレシーバ",
			info:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.info.UpdateTarget())
		})
	}
}

func TestParseGoBinaryInfo(t *testing.T) {
	tests := []struct {
		name            string
		binaryPath      string
		output          string
		wantPackagePath string
		wantModulePath  string
		wantVersion     string
		wantBinaryName  string
		wantErr         bool
	}{
		{
			name:            "path行のみ",
			binaryPath:      "/usr/local/bin/bar",
			output:          "path github.com/foo/bar\n",
			wantPackagePath: "github.com/foo/bar",
			wantBinaryName:  "bar",
		},
		{
			name:            "mod行あり",
			binaryPath:      "/usr/local/bin/bar",
			output:          "path github.com/foo/bar\nmod github.com/foo v1.2.3 h1:dummy\n",
			wantPackagePath: "github.com/foo/bar",
			wantModulePath:  "github.com/foo",
			wantVersion:     "v1.2.3",
			wantBinaryName:  "bar",
		},
		{
			name:       "path行なし",
			binaryPath: "/usr/local/bin/bar",
			output:     "mod github.com/foo v1.2.3 h1:dummy\n",
			wantErr:    true,
		},
		{
			name:       "空出力",
			binaryPath: "/usr/local/bin/bar",
			output:     "",
			wantErr:    true,
		},
		{
			name:       "改行のみ",
			binaryPath: "/usr/local/bin/bar",
			output:     "\n\n",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGoBinaryInfo(tt.binaryPath, tt.output)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.binaryPath, got.BinaryPath)
			assert.Equal(t, tt.wantBinaryName, got.BinaryName)
			assert.Equal(t, tt.wantPackagePath, got.PackagePath)
			assert.Equal(t, tt.wantModulePath, got.ModulePath)
			assert.Equal(t, tt.wantVersion, got.InstalledVersion)
		})
	}
}

func TestDiscoverGoBinariesInDir(t *testing.T) {
	t.Parallel()

	fakeOutput := "path github.com/foo/mytool\nmod github.com/foo v1.2.3 h1:dummy\n"

	tests := []struct {
		name         string
		setup        func(t *testing.T, dir string)
		fakeRunCmd   func(ctx context.Context, path string) ([]byte, error)
		wantDetected []string // BinaryName の一覧
		wantSkipped  []string // SkippedBinary.Name の一覧
	}{
		{
			name: "バックアップファイル（*.exe~）はSkippedに入る",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "tool.exe~"), []byte{}, 0o644))
			},
			fakeRunCmd: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("呼ばれてはいけない")
			},
			wantDetected: nil,
			wantSkipped:  []string{"tool.exe~"},
		},
		{
			name: "ディレクトリは除外される（DetectedにもSkippedにも入らない）",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
			},
			fakeRunCmd: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("呼ばれてはいけない")
			},
			wantDetected: nil,
			wantSkipped:  nil,
		},
		{
			name: "go version -m 失敗バイナリはSkippedに入る",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "notgobin"), []byte{}, 0o644))
			},
			fakeRunCmd: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("exit status 1")
			},
			wantDetected: nil,
			wantSkipped:  []string{"notgobin"},
		},
		{
			name: "path行なしバイナリはSkippedに入る",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "nopath"), []byte{}, 0o644))
			},
			fakeRunCmd: func(_ context.Context, _ string) ([]byte, error) {
				return []byte("mod github.com/foo v1.0.0 h1:dummy\n"), nil
			},
			wantDetected: nil,
			wantSkipped:  []string{"nopath"},
		},
		{
			name: "正常バイナリはDetectedに入る",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "mytool"), []byte{}, 0o644))
			},
			fakeRunCmd: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(fakeOutput), nil
			},
			wantDetected: []string{"mytool"},
			wantSkipped:  nil,
		},
		{
			name: "バックアップ・失敗・正常が混在する場合",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "backup~"), []byte{}, 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "broken"), []byte{}, 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "good"), []byte{}, 0o644))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "dir"), 0o755))
			},
			fakeRunCmd: func(_ context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "broken" {
					return nil, errors.New("not a go binary")
				}
				return []byte("path github.com/foo/" + filepath.Base(path) + "\nmod github.com/foo v1.0.0 h1:dummy\n"), nil
			},
			wantDetected: []string{"good"},
			wantSkipped:  []string{"backup~", "broken"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			tt.setup(t, dir)

			result, err := discoverInDir(context.Background(), dir, tt.fakeRunCmd)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Detected の BinaryName 一覧を検証
			if tt.wantDetected == nil {
				assert.Empty(t, result.Detected)
			} else {
				require.Len(t, result.Detected, len(tt.wantDetected))

				names := make([]string, len(result.Detected))
				for i, d := range result.Detected {
					names[i] = d.BinaryName
				}

				assert.ElementsMatch(t, tt.wantDetected, names)
			}

			// Skipped の Name 一覧を検証
			if tt.wantSkipped == nil {
				assert.Empty(t, result.Skipped)
			} else {
				require.Len(t, result.Skipped, len(tt.wantSkipped))

				names := make([]string, len(result.Skipped))
				for i, s := range result.Skipped {
					names[i] = s.Name
				}

				assert.ElementsMatch(t, tt.wantSkipped, names)
			}
		})
	}
}

func TestDiscoverGoBinariesInDir_MissingDir(t *testing.T) {
	t.Parallel()

	_, err := DiscoverGoBinariesInDir(context.Background(), filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "読み取りに失敗")
}

func TestDiscoverGoBinaries_UsesGOBIN(t *testing.T) {
	// t.Setenv は t.Parallel() と併用不可
	dir := t.TempDir()
	// *~ ファイルはバックアップファイルとして go version -m を起動せずに Skipped 処理されるため、
	// 外部プロセスを発生させずに GOBIN 優先を検証できる
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool~"), []byte{}, 0o644))
	t.Setenv("GOBIN", dir)
	t.Setenv("GOPATH", "")

	result, err := DiscoverGoBinaries(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Skipped, 1)
	assert.Equal(t, "tool~", result.Skipped[0].Name)
	assert.Equal(t, "バックアップファイル", result.Skipped[0].Reason)
}

func TestDiscoverInDir_ContextCanceled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool"), []byte{}, 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	fakeRunCmd := func(c context.Context, _ string) ([]byte, error) {
		return nil, c.Err()
	}

	_, err := discoverInDir(ctx, dir, fakeRunCmd)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestGoUpdater_Check_EmptyTargets は targets 未設定時に dsx sys discover へのヒントを含む
// メッセージが返ることを検証します。
func TestGoUpdater_Check_EmptyTargets(t *testing.T) {
	tests := []struct {
		name         string
		targets      []string
		wantMsgParts []string
		wantUpdates  int
	}{
		{
			name:    "targets未設定でdiscover案内メッセージを返す",
			targets: nil,
			wantMsgParts: []string{
				"targets は未設定",
				"dsx sys discover",
				"go.targets",
			},
			wantUpdates: 0,
		},
		{
			name:    "空スライスでもdiscover案内メッセージを返す",
			targets: []string{},
			wantMsgParts: []string{
				"targets は未設定",
				"dsx sys discover",
				"go.targets",
			},
			wantUpdates: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GoUpdater{targets: tt.targets}

			got, err := g.Check(context.Background())
			require.NoError(t, err)
			require.NotNil(t, got)

			assert.Equal(t, tt.wantUpdates, got.AvailableUpdates)
			assert.Empty(t, got.Packages)

			for _, part := range tt.wantMsgParts {
				assert.Contains(t, got.Message, part, "メッセージに %q が含まれていない", part)
			}
		})
	}
}
