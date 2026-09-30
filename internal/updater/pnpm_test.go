package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPnpmUpdater_Name(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	assert.Equal(t, "pnpm", p.Name())
}

func TestPnpmUpdater_DisplayName(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	assert.Equal(t, "pnpm (Node.js グローバルパッケージ)", p.DisplayName())
}

func TestPnpmUpdater_Configure(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	err := p.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestPnpmUpdater_IsAvailable(t *testing.T) {
	tests := []struct {
		name      string
		setupPath func(t *testing.T) string
		expected  bool
	}{
		{
			name: "pnpm コマンドが存在する場合は true",
			setupPath: func(t *testing.T) string {
				t.Helper()
				fakeDir := t.TempDir()
				writeFakePnpmCommand(t, fakeDir)

				return fakeDir
			},
			expected: true,
		},
		{
			name: "pnpm コマンドが存在しない場合は false",
			setupPath: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", tt.setupPath(t))

			p := &PnpmUpdater{}
			assert.Equal(t, tt.expected, p.IsAvailable())
		})
	}
}

func TestPnpmUpdater_parseOutdatedJSON(t *testing.T) {
	tests := []struct {
		name        string
		output      []byte
		want        map[string]PackageInfo
		expectErr   bool
		errContains string
	}{
		{
			name:      "空出力",
			output:    nil,
			want:      map[string]PackageInfo{},
			expectErr: false,
		},
		{
			name:        "不正なJSONはエラー",
			output:      []byte("{invalid"),
			expectErr:   true,
			errContains: "JSON の解析に失敗",
		},
		{
			name: "配列形式の出力",
			output: []byte(`[
  {"name":"typescript","current":"5.1.0","latest":"5.2.0"},
  {"packageName":"@scope/pkg","current":"1.0.0","wanted":"1.1.0"}
]`),
			want: map[string]PackageInfo{
				"typescript": {
					Name:           "typescript",
					CurrentVersion: "5.1.0",
					NewVersion:     "5.2.0",
				},
				"@scope/pkg": {
					Name:           "@scope/pkg",
					CurrentVersion: "1.0.0",
					NewVersion:     "1.1.0",
				},
			},
		},
		{
			name: "オブジェクト形式の出力",
			output: []byte(`{
  "eslint": {"current":"8.0.0","latest":"9.0.0"},
  "pnpm": {"current":"9.0.0","wanted":"9.1.0"}
}`),
			want: map[string]PackageInfo{
				"eslint": {
					Name:           "eslint",
					CurrentVersion: "8.0.0",
					NewVersion:     "9.0.0",
				},
				"pnpm": {
					Name:           "pnpm",
					CurrentVersion: "9.0.0",
					NewVersion:     "9.1.0",
				},
			},
		},
		{
			name: "配列形式で名前が空の要素はスキップ",
			output: []byte(`[
  {"name":"", "packageName":"", "current":"1.0.0", "latest":"2.0.0"}
]`),
			want: map[string]PackageInfo{},
		},
		{
			name: "pnpm v11 の [WARN] 行が JSON の前に混入しても解析できる",
			output: []byte("[WARN] Using --global skips the package manager check for this project\n" +
				`{"eslint": {"current":"8.0.0","latest":"9.0.0"}}`),
			want: map[string]PackageInfo{
				"eslint": {
					Name:           "eslint",
					CurrentVersion: "8.0.0",
					NewVersion:     "9.0.0",
				},
			},
		},
		{
			name:        "JSON が存在しない出力はエラー",
			output:      []byte("[WARN] something went wrong"),
			expectErr:   true,
			errContains: "JSON が見つかりません",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &PnpmUpdater{}
			got, err := p.parseOutdatedJSON(tt.output)

			if tt.expectErr {
				assert.Error(t, err)

				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Len(t, got, len(tt.want))

			gotMap := make(map[string]PackageInfo, len(got))
			for _, pkg := range got {
				gotMap[pkg.Name] = pkg
			}

			for name, wantPkg := range tt.want {
				gotPkg, ok := gotMap[name]
				assert.True(t, ok, "package %q が見つかりません", name)
				assert.Equal(t, wantPkg.CurrentVersion, gotPkg.CurrentVersion)
				assert.Equal(t, wantPkg.NewVersion, gotPkg.NewVersion)
			}
		})
	}
}

func TestPnpmUpdater_Check(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		wantCount     int
		expectErr     bool
		errContains   string
		wantFirstName string
	}{
		{
			name:          "更新候補あり（exit code 1）",
			mode:          "outdated_updates",
			wantCount:     1,
			wantFirstName: "typescript",
		},
		{
			name:      "更新候補なし",
			mode:      "outdated_none",
			wantCount: 0,
		},
		{
			name:        "不正JSONは解析エラー",
			mode:        "outdated_invalid_json",
			expectErr:   true,
			errContains: "出力解析に失敗",
		},
		{
			name:        "manifest不足はエラー",
			mode:        "missing_manifest",
			expectErr:   true,
			errContains: "ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PNPM_MODE", tc.mode)

			if tc.mode == "missing_manifest" {
				globalDir := filepath.Join(createASCIITempDir(t, "dsx-pnpm-check-"), "pnpm-global")
				t.Setenv("DSX_TEST_PNPM_GLOBAL_DIR", globalDir)
			}

			p := &PnpmUpdater{}
			got, err := p.Check(context.Background())

			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantCount, got.AvailableUpdates)

			if tc.wantFirstName != "" {
				assert.NotEmpty(t, got.Packages)
				assert.Equal(t, tc.wantFirstName, got.Packages[0].Name)
			}
		})
	}
}

func TestPnpmUpdater_resolveGlobalDir(t *testing.T) {
	tests := []struct {
		name           string
		rootMode       string
		globalDir      string
		expectErr      bool
		errContainsAny []string
		wantDir        string
	}{
		{
			name:      "node_modules 末尾は親ディレクトリを返す",
			rootMode:  "",
			globalDir: "pnpm-global",
			wantDir:   "pnpm-global",
		},
		{
			name:      "node_modules 末尾でない出力はそのまま返す",
			rootMode:  "plain_dir",
			globalDir: "pnpm-global-plain",
			wantDir:   "pnpm-global-plain",
		},
		{
			name:      "pnpm root 実行失敗はエラー",
			rootMode:  "error",
			globalDir: "pnpm-global-error",
			expectErr: true,
			errContainsAny: []string{
				"pnpm root -g の実行に失敗",
				"pnpm root -g の出力が空です",
			},
		},
		{
			name:      "空出力はエラー",
			rootMode:  "empty",
			globalDir: "pnpm-global-empty",
			expectErr: true,
			errContainsAny: []string{
				"pnpm root -g の出力が空です",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PNPM_ROOT_MODE", tt.rootMode)

			baseDir := createASCIITempDir(t, "dsx-pnpm-resolve-")
			actualGlobalDir := filepath.Join(baseDir, tt.globalDir)
			t.Setenv("DSX_TEST_PNPM_GLOBAL_DIR", actualGlobalDir)

			p := &PnpmUpdater{}
			got, err := p.resolveGlobalDir(context.Background())

			if tt.expectErr {
				if assert.Error(t, err) && len(tt.errContainsAny) > 0 {
					matched := false
					for _, expected := range tt.errContainsAny {
						if strings.Contains(err.Error(), expected) {
							matched = true
							break
						}
					}

					assert.True(t, matched, "想定エラー文字列が見つかりません: %v / got: %s", tt.errContainsAny, err.Error())
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, filepath.Clean(filepath.Join(baseDir, tt.wantDir)), got)
		})
	}
}

func TestPnpmUpdater_ensureGlobalManifest(t *testing.T) {
	tests := []struct {
		name             string
		rootMode         string
		prepareGlobalDir func(t *testing.T) string
		expectErr        bool
		errContainsAny   []string
		wantContent      string
	}{
		{
			name:     "manifest が無ければ作成する",
			rootMode: "",
			prepareGlobalDir: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(createASCIITempDir(t, "dsx-pnpm-manifest-"), "pnpm-global")
			},
			wantContent: pnpmGlobalManifestContent,
		},
		{
			name:     "manifest が既に存在する場合は上書きしない",
			rootMode: "",
			prepareGlobalDir: func(t *testing.T) string {
				t.Helper()

				globalDir := filepath.Join(createASCIITempDir(t, "dsx-pnpm-existing-"), "pnpm-global-existing")
				if mkdirErr := os.MkdirAll(globalDir, 0o755); mkdirErr != nil {
					t.Fatalf("global dir 作成失敗: %v", mkdirErr)
				}

				manifestPath := filepath.Join(globalDir, "package.json")
				if writeErr := os.WriteFile(manifestPath, []byte("{\"name\":\"existing\"}\n"), 0o644); writeErr != nil {
					t.Fatalf("manifest 事前作成失敗: %v", writeErr)
				}

				return globalDir
			},
			wantContent: "{\"name\":\"existing\"}\n",
		},
		{
			name:     "manifest の状態確認失敗を返す",
			rootMode: "",
			prepareGlobalDir: func(t *testing.T) string {
				t.Helper()

				blocker := filepath.Join(createASCIITempDir(t, "dsx-pnpm-blocker-"), "blocker")
				if writeErr := os.WriteFile(blocker, []byte("x"), 0o644); writeErr != nil {
					t.Fatalf("blocker 作成失敗: %v", writeErr)
				}

				return filepath.Join(blocker, "pnpm-global")
			},
			expectErr: true,
			errContainsAny: []string{
				"状態確認に失敗",
				"グローバルディレクトリの作成に失敗",
			},
		},
		{
			name:     "pnpm root 実行失敗を返す",
			rootMode: "error",
			prepareGlobalDir: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(createASCIITempDir(t, "dsx-pnpm-error-"), "pnpm-global-error")
			},
			expectErr: true,
			errContainsAny: []string{
				"pnpm root -g の実行に失敗",
				"pnpm root -g の出力が空です",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PNPM_ROOT_MODE", tt.rootMode)

			globalDir := tt.prepareGlobalDir(t)
			t.Setenv("DSX_TEST_PNPM_GLOBAL_DIR", globalDir)

			p := &PnpmUpdater{}
			err := p.ensureGlobalManifest(context.Background())

			if tt.expectErr {
				if assert.Error(t, err) && len(tt.errContainsAny) > 0 {
					matched := false
					for _, expected := range tt.errContainsAny {
						if strings.Contains(err.Error(), expected) {
							matched = true
							break
						}
					}

					assert.True(t, matched, "想定エラー文字列が見つかりません: %v / got: %s", tt.errContainsAny, err.Error())
				}

				return
			}

			assert.NoError(t, err)

			content, readErr := os.ReadFile(filepath.Join(globalDir, "package.json"))
			if readErr != nil {
				t.Fatalf("manifest 読み込み失敗: %v", readErr)
			}

			assert.Equal(t, tt.wantContent, string(content))
		})
	}
}

func TestPnpmUpdater_Update(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  string
		opts                  UpdateOptions
		expectErr             bool
		errContains           string
		wantUpdated           int
		wantMessageContains   string
		expectManifestCreated bool
		expectManifestMissing bool
	}{
		{
			name:                "DryRunは更新せず計画のみ返す",
			mode:                "outdated_updates",
			opts:                UpdateOptions{DryRun: true},
			wantUpdated:         0,
			wantMessageContains: "DryRun",
		},
		{
			name:                "通常更新成功",
			mode:                "outdated_updates",
			opts:                UpdateOptions{},
			wantUpdated:         1,
			wantMessageContains: "更新しました",
		},
		{
			name:        "事前チェック失敗",
			mode:        "outdated_invalid_json",
			opts:        UpdateOptions{},
			expectErr:   true,
			errContains: "出力解析に失敗",
		},
		{
			name:                  "manifest不足は通常更新時に自動初期化して再試行",
			mode:                  "missing_manifest",
			opts:                  UpdateOptions{},
			wantUpdated:           0,
			wantMessageContains:   "最新です",
			expectManifestCreated: true,
		},
		{
			name:                  "manifest不足はDryRunでは自動初期化せず案内",
			mode:                  "missing_manifest",
			opts:                  UpdateOptions{DryRun: true},
			wantUpdated:           0,
			wantMessageContains:   "更新確認をスキップしました",
			expectManifestMissing: true,
		},
		{
			name:        "通常更新失敗",
			mode:        "update_fail",
			opts:        UpdateOptions{},
			expectErr:   true,
			errContains: "pnpm update -g --latest に失敗",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PNPM_MODE", tc.mode)

			manifestPath := ""
			if tc.mode == "missing_manifest" {
				globalDir := filepath.Join(createASCIITempDir(t, "dsx-pnpm-update-"), "pnpm-global")
				t.Setenv("DSX_TEST_PNPM_GLOBAL_DIR", globalDir)
				manifestPath = filepath.Join(globalDir, "package.json")
			}

			p := &PnpmUpdater{}
			got, err := p.Update(context.Background(), tc.opts)

			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdated, got.UpdatedCount)

			if tc.wantMessageContains != "" {
				assert.Contains(t, got.Message, tc.wantMessageContains)
			}

			if tc.expectManifestCreated {
				if _, statErr := os.Stat(manifestPath); statErr != nil {
					t.Fatalf("manifest が作成されていません: %v", statErr)
				}
			}

			if tc.expectManifestMissing {
				_, statErr := os.Stat(manifestPath)
				assert.Error(t, statErr)
				assert.True(t, os.IsNotExist(statErr), "manifest は作成されない想定です")
			}
		})
	}
}

func TestPnpmUpdater_CheckSelfUpdate(t *testing.T) {
	tests := []struct {
		name              string
		currentOutput     string
		latestOutput      string
		errOnCall         int
		wantUpdates       int
		wantPackage       PackageInfo
		wantMessage       string
		wantErrContains   string
		wantOutputCallNum int
	}{
		{
			name:              "更新候補を返す",
			currentOutput:     "12.7.0\n",
			latestOutput:      `"12.8.1"` + "\n",
			wantUpdates:       1,
			wantPackage:       PackageInfo{Name: "pnpm", CurrentVersion: "12.7.0", NewVersion: "12.8.1"},
			wantMessage:       "pnpm 本体の更新が可能です",
			wantOutputCallNum: 2,
		},
		{
			name:              "最新版なら候補を返さない",
			currentOutput:     "v12.8.1\n",
			latestOutput:      `"12.8.1"`,
			wantMessage:       "pnpm 本体は最新です",
			wantOutputCallNum: 2,
		},
		{
			name:              "registry の latest が現在版より古ければ更新しない",
			currentOutput:     "12.8.1\n",
			latestOutput:      `"12.7.0"`,
			wantMessage:       "pnpm 本体は最新です",
			wantOutputCallNum: 2,
		},
		{
			name:              "同じ core の prerelease から安定版へ更新する",
			currentOutput:     "12.8.1-rc.1\n",
			latestOutput:      `"12.8.1"`,
			wantUpdates:       1,
			wantPackage:       PackageInfo{Name: "pnpm", CurrentVersion: "12.8.1-rc.1", NewVersion: "12.8.1"},
			wantMessage:       "pnpm 本体の更新が可能です",
			wantOutputCallNum: 2,
		},
		{
			name:              "同じ core の新しい prerelease へ更新する",
			currentOutput:     "12.8.1-rc.1\n",
			latestOutput:      `"12.8.1-rc.2"`,
			wantUpdates:       1,
			wantPackage:       PackageInfo{Name: "pnpm", CurrentVersion: "12.8.1-rc.1", NewVersion: "12.8.1-rc.2"},
			wantMessage:       "pnpm 本体の更新が可能です",
			wantOutputCallNum: 2,
		},
		{
			name:              "WARN 行を除外してバージョンを読む",
			currentOutput:     "[WARN] 設定の警告\n12.7.0\n",
			latestOutput:      `"12.8.1"`,
			wantUpdates:       1,
			wantPackage:       PackageInfo{Name: "pnpm", CurrentVersion: "12.7.0", NewVersion: "12.8.1"},
			wantMessage:       "pnpm 本体の更新が可能です",
			wantOutputCallNum: 2,
		},
		{
			name:              "不正なバージョンはエラー",
			currentOutput:     "latest\n",
			wantErrContains:   "pnpm --version の出力解析に失敗",
			wantOutputCallNum: 1,
		},
		{
			name:              "空のバージョン出力はエラー",
			currentOutput:     "",
			wantErrContains:   "pnpm --version の出力解析に失敗",
			wantOutputCallNum: 1,
		},
		{
			name:              "現在バージョンの取得失敗はエラー",
			errOnCall:         1,
			wantErrContains:   "pnpm --version の実行に失敗",
			wantOutputCallNum: 1,
		},
		{
			name:              "最新バージョンの取得失敗はエラー",
			currentOutput:     "12.7.0\n",
			errOnCall:         2,
			wantErrContains:   "pnpm view pnpm version --json の実行に失敗",
			wantOutputCallNum: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			updater := &PnpmUpdater{
				runSelfUpdateOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					calls++

					wantArgs := pnpmSelfUpdateArgs("--version")

					if calls == 2 {
						wantArgs = pnpmSelfUpdateArgs("view", "pnpm", "version", "--json")
					}

					assert.Equal(t, wantArgs, args)

					if calls == tc.errOnCall {
						return nil, errors.New("registry unavailable")
					}

					if calls == 1 {
						return []byte(tc.currentOutput), nil
					}

					return []byte(tc.latestOutput), nil
				},
			}

			got, err := updater.CheckSelfUpdate(context.Background())

			assert.Equal(t, tc.wantOutputCallNum, calls)

			if tc.wantErrContains != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdates, got.AvailableUpdates)
			assert.Equal(t, tc.wantMessage, got.Message)

			if tc.wantUpdates > 0 {
				assert.Equal(t, []PackageInfo{tc.wantPackage}, got.Packages)
			} else {
				assert.Empty(t, got.Packages)
			}
		})
	}
}

func TestPnpmUpdater_SelfUpdate(t *testing.T) {
	tests := []struct {
		name                string
		currentVersion      string
		latestVersion       string
		opts                UpdateOptions
		updateErr           error
		wantUpdates         int
		wantMessageContains string
		wantErrContains     string
		wantUpdateCalls     int
	}{
		{
			name:                "更新なし",
			currentVersion:      "12.8.1",
			latestVersion:       "12.8.1",
			wantMessageContains: "最新です",
		},
		{
			name:                "DryRun は自己更新コマンドを実行しない",
			currentVersion:      "12.7.0",
			latestVersion:       "12.8.1",
			opts:                UpdateOptions{DryRun: true},
			wantMessageContains: "DryRun",
		},
		{
			name:                "pnpm self-update で本体を更新する",
			currentVersion:      "12.7.0",
			latestVersion:       "12.8.1",
			wantUpdates:         1,
			wantMessageContains: "pnpm 本体を更新しました",
			wantUpdateCalls:     1,
		},
		{
			name:                "自己更新失敗をコマンドの文脈付きで返す",
			currentVersion:      "12.7.0",
			latestVersion:       "12.8.1",
			updateErr:           errors.New("permission denied"),
			wantUpdates:         0,
			wantMessageContains: "pnpm 本体の更新が可能です",
			wantErrContains:     "pnpm self-update の実行に失敗",
			wantUpdateCalls:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateCalls := 0
			updater := &PnpmUpdater{
				runSelfUpdateOutputStep: func(_ context.Context, args ...string) ([]byte, error) {
					if strings.HasSuffix(strings.Join(args, " "), "--version") {
						return []byte(tc.currentVersion), nil
					}

					assert.Equal(t, pnpmSelfUpdateArgs("view", "pnpm", "version", "--json"), args)

					return []byte(`"` + tc.latestVersion + `"`), nil
				},
				runSelfUpdateStep: func(_ context.Context, args ...string) error {
					updateCalls++

					assert.Equal(t, pnpmSelfUpdateArgs("self-update"), args)

					return tc.updateErr
				},
			}

			got, err := updater.SelfUpdate(context.Background(), tc.opts)
			assert.Equal(t, tc.wantUpdateCalls, updateCalls)

			if tc.wantErrContains != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}

				assert.NotNil(t, got)

				assert.Len(t, got.Errors, 1)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, ContinueNormalUpdate, got.Continuation)
			assert.Equal(t, tc.wantUpdates, got.UpdatedCount)
			assert.Contains(t, got.Message, tc.wantMessageContains)
		})
	}
}

func TestComparePnpmVersions(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{
			name:  "安定版は同じ core の prerelease より新しい",
			left:  "12.8.1",
			right: "12.8.1-rc.2",
			want:  1,
		},
		{
			name:  "prerelease の数値識別子を数値として比較する",
			left:  "12.8.1-rc.10",
			right: "12.8.1-rc.2",
			want:  1,
		},
		{
			name:  "数値識別子は英数字識別子より古い",
			left:  "12.8.1-rc.10",
			right: "12.8.1-rc.beta",
			want:  -1,
		},
		{
			name:  "同じ識別子なら長い prerelease の方が新しい",
			left:  "12.8.1-rc.1.1",
			right: "12.8.1-rc.1",
			want:  1,
		},
		{
			name:  "build metadata は比較に影響しない",
			left:  "12.8.1-rc.1+build.2",
			right: "12.8.1-rc.1+build.1",
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, comparePnpmVersions(tc.left, tc.right))
		})
	}
}

func TestPnpmUpdater_SelfUpdate_DisablesCorepackProjectSpec(t *testing.T) {
	commandDir := createASCIITempDir(t, "dsx-pnpm-corepack-")
	writeCorepackAwareFakePnpmCommand(t, commandDir)
	t.Setenv("PATH", commandDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("COREPACK_ENABLE_PROJECT_SPEC", "1")

	result, err := (&PnpmUpdater{}).SelfUpdate(context.Background(), UpdateOptions{})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, 1, result.UpdatedCount)

	if assert.Len(t, result.Packages, 1) {
		assert.Equal(t, "12.7.0", result.Packages[0].CurrentVersion)
		assert.Equal(t, "12.8.1", result.Packages[0].NewVersion)
	}
}

func createASCIITempDir(t *testing.T, pattern string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("temp dir 作成失敗: %v", err)
	}

	t.Cleanup(func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			t.Errorf("temp dir 削除失敗: %v", removeErr)
		}
	})

	return dir
}

func writeCorepackAwareFakePnpmCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)
	if runtime.GOOS == "windows" {
		fileName = "pnpm.cmd"
		content = `@echo off
if not "%COREPACK_ENABLE_PROJECT_SPEC%"=="0" exit /b 90
set "arguments=%*"
if not "%arguments:--version=%"=="%arguments%" goto current
if not "%arguments:view=%"=="%arguments%" goto latest
if not "%arguments:self-update=%"=="%arguments%" exit /b 0
exit /b 91

:current
echo 12.7.0
exit /b 0

:latest
echo "12.8.1"
exit /b 0
`
	} else {
		fileName = "pnpm"
		content = `#!/bin/sh
if [ "${COREPACK_ENABLE_PROJECT_SPEC}" != "0" ]; then
  exit 90
fi

case "$3" in
  --version)
    echo '12.7.0'
    ;;
  view)
    echo '"12.8.1"'
    ;;
  self-update)
    exit 0
    ;;
  *)
    exit 91
    ;;
esac
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake Corepack shim の作成に失敗: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake Corepack shim の実行権限設定に失敗: %v", err)
		}
	}
}

func writeFakePnpmCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "pnpm.cmd"
		content = `@echo off
set subcmd=%1

if "%subcmd%"=="root" (
  if "%2"=="-g" (
    if "%DSX_TEST_PNPM_ROOT_MODE%"=="error" (
      >&2 echo root failed
      exit /b 2
    )
    if "%DSX_TEST_PNPM_ROOT_MODE%"=="empty" (
      exit /b 0
    )
    if not "%DSX_TEST_PNPM_GLOBAL_DIR%"=="" (
      if "%DSX_TEST_PNPM_ROOT_MODE%"=="plain_dir" (
        echo %DSX_TEST_PNPM_GLOBAL_DIR%
        exit /b 0
      )
      echo %DSX_TEST_PNPM_GLOBAL_DIR%\node_modules
      exit /b 0
    )
    echo C:\pnpm-global\node_modules
    exit /b 0
  )
)

if "%subcmd%"=="outdated" (
  if "%DSX_TEST_PNPM_MODE%"=="missing_manifest" (
    if exist "%DSX_TEST_PNPM_GLOBAL_DIR%\package.json" (
      echo []
      exit /b 0
    )
    echo ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND No package.json was found in "%DSX_TEST_PNPM_GLOBAL_DIR%".
    exit /b 1
  )
  if "%DSX_TEST_PNPM_MODE%"=="outdated_updates" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
  if "%DSX_TEST_PNPM_MODE%"=="outdated_none" (
    echo []
    exit /b 0
  )
  if "%DSX_TEST_PNPM_MODE%"=="outdated_invalid_json" (
    echo {invalid
    exit /b 1
  )
  if "%DSX_TEST_PNPM_MODE%"=="outdated_command_error" (
    >&2 echo fatal error
    exit /b 2
  )
  if "%DSX_TEST_PNPM_MODE%"=="update_fail" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
)

if "%subcmd%"=="update" (
  if not "%2"=="-g" (
    >&2 echo missing -g
    exit /b 1
  )
  if not "%3"=="--latest" (
    >&2 echo missing --latest
    exit /b 1
  )
  if not "%4"=="--no-interactive" (
    >&2 echo missing --no-interactive
    exit /b 1
  )
  if not "%CI%"=="true" (
    >&2 echo missing CI=true
    exit /b 1
  )
  if "%DSX_TEST_PNPM_MODE%"=="update_fail" (
    goto updatefail
  )
  echo updated
  exit /b 0
)

echo []
exit /b 0

:updatefail
>&2 echo update failed
exit /b 1
`
	} else {
		fileName = "pnpm"
		content = `#!/bin/sh
subcmd="$1"
mode="${DSX_TEST_PNPM_MODE}"

if [ "${subcmd}" = "root" ]; then
  if [ "$2" = "-g" ]; then
    if [ "${DSX_TEST_PNPM_ROOT_MODE}" = "error" ]; then
      echo "root failed" 1>&2
      exit 2
    fi
    if [ "${DSX_TEST_PNPM_ROOT_MODE}" = "empty" ]; then
      exit 0
    fi
    if [ -n "${DSX_TEST_PNPM_GLOBAL_DIR}" ]; then
      if [ "${DSX_TEST_PNPM_ROOT_MODE}" = "plain_dir" ]; then
        echo "${DSX_TEST_PNPM_GLOBAL_DIR}"
        exit 0
      fi
      echo "${DSX_TEST_PNPM_GLOBAL_DIR}/node_modules"
      exit 0
    fi
    echo "/tmp/pnpm-global/node_modules"
    exit 0
  fi
fi

if [ "${subcmd}" = "outdated" ]; then
  if [ "${mode}" = "missing_manifest" ]; then
    if [ -f "${DSX_TEST_PNPM_GLOBAL_DIR}/package.json" ]; then
      echo '[]'
      exit 0
    fi
    echo 'ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND No package.json was found in "'"${DSX_TEST_PNPM_GLOBAL_DIR}"'".'
    exit 1
  fi
  if [ "${mode}" = "outdated_updates" ]; then
    echo '[{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]'
    exit 1
  fi
  if [ "${mode}" = "outdated_none" ]; then
    echo '[]'
    exit 0
  fi
  if [ "${mode}" = "outdated_invalid_json" ]; then
    echo '{invalid'
    exit 1
  fi
  if [ "${mode}" = "outdated_command_error" ]; then
    echo 'fatal error' 1>&2
    exit 2
  fi
  if [ "${mode}" = "update_fail" ]; then
    echo '[{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]'
    exit 1
  fi
fi

if [ "${subcmd}" = "update" ]; then
  if [ "$2" != "-g" ]; then
    echo 'missing -g' 1>&2
    exit 1
  fi
  if [ "$3" != "--latest" ]; then
    echo 'missing --latest' 1>&2
    exit 1
  fi
  if [ "$4" != "--no-interactive" ]; then
    echo 'missing --no-interactive' 1>&2
    exit 1
  fi
  if [ "${CI}" != "true" ]; then
    echo 'missing CI=true' 1>&2
    exit 1
  fi
  if [ "${mode}" = "update_fail" ]; then
    echo 'update failed' 1>&2
    exit 1
  fi
  echo 'updated'
  exit 0
fi

echo '[]'
exit 0
`
	}

	fullPath := filepath.Join(dir, fileName)
	if writeErr := os.WriteFile(fullPath, []byte(content), 0o755); writeErr != nil {
		t.Fatalf("fake command write failed: %v", writeErr)
	}

	if runtime.GOOS != "windows" {
		if chmodErr := os.Chmod(fullPath, 0o755); chmodErr != nil {
			t.Fatalf("fake command chmod failed: %v", chmodErr)
		}
	}
}
