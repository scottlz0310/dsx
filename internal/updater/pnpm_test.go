package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
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
			t.Setenv("DEVSYNC_TEST_PNPM_MODE", tc.mode)

			if tc.mode == "missing_manifest" {
				globalDir := filepath.Join(createASCIITempDir(t, "devsync-pnpm-check-"), "pnpm-global")
				t.Setenv("DEVSYNC_TEST_PNPM_GLOBAL_DIR", globalDir)
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
			t.Setenv("DEVSYNC_TEST_PNPM_ROOT_MODE", tt.rootMode)

			baseDir := createASCIITempDir(t, "devsync-pnpm-resolve-")
			actualGlobalDir := filepath.Join(baseDir, tt.globalDir)
			t.Setenv("DEVSYNC_TEST_PNPM_GLOBAL_DIR", actualGlobalDir)

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

				return filepath.Join(createASCIITempDir(t, "devsync-pnpm-manifest-"), "pnpm-global")
			},
			wantContent: pnpmGlobalManifestContent,
		},
		{
			name:     "manifest が既に存在する場合は上書きしない",
			rootMode: "",
			prepareGlobalDir: func(t *testing.T) string {
				t.Helper()

				globalDir := filepath.Join(createASCIITempDir(t, "devsync-pnpm-existing-"), "pnpm-global-existing")
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

				blocker := filepath.Join(createASCIITempDir(t, "devsync-pnpm-blocker-"), "blocker")
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

				return filepath.Join(createASCIITempDir(t, "devsync-pnpm-error-"), "pnpm-global-error")
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
			t.Setenv("DEVSYNC_TEST_PNPM_ROOT_MODE", tt.rootMode)

			globalDir := tt.prepareGlobalDir(t)
			t.Setenv("DEVSYNC_TEST_PNPM_GLOBAL_DIR", globalDir)

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
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_PNPM_MODE", tc.mode)

			manifestPath := ""
			if tc.mode == "missing_manifest" {
				globalDir := filepath.Join(createASCIITempDir(t, "devsync-pnpm-update-"), "pnpm-global")
				t.Setenv("DEVSYNC_TEST_PNPM_GLOBAL_DIR", globalDir)
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
    if "%DEVSYNC_TEST_PNPM_ROOT_MODE%"=="error" (
      >&2 echo root failed
      exit /b 2
    )
    if "%DEVSYNC_TEST_PNPM_ROOT_MODE%"=="empty" (
      exit /b 0
    )
    if not "%DEVSYNC_TEST_PNPM_GLOBAL_DIR%"=="" (
      if "%DEVSYNC_TEST_PNPM_ROOT_MODE%"=="plain_dir" (
        echo %DEVSYNC_TEST_PNPM_GLOBAL_DIR%
        exit /b 0
      )
      echo %DEVSYNC_TEST_PNPM_GLOBAL_DIR%\node_modules
      exit /b 0
    )
    echo C:\pnpm-global\node_modules
    exit /b 0
  )
)

if "%subcmd%"=="outdated" (
  if "%DEVSYNC_TEST_PNPM_MODE%"=="missing_manifest" (
    if exist "%DEVSYNC_TEST_PNPM_GLOBAL_DIR%\package.json" (
      echo []
      exit /b 0
    )
    echo ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND No package.json was found in "%DEVSYNC_TEST_PNPM_GLOBAL_DIR%".
    exit /b 1
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_updates" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_none" (
    echo []
    exit /b 0
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_invalid_json" (
    echo {invalid
    exit /b 1
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_command_error" (
    >&2 echo fatal error
    exit /b 2
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="update_fail" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
)

if "%subcmd%"=="update" (
  if "%DEVSYNC_TEST_PNPM_MODE%"=="update_fail" (
    >&2 echo update failed
    exit /b 1
  )
  echo updated
  exit /b 0
)

echo []
exit /b 0
`
	} else {
		fileName = "pnpm"
		content = `#!/bin/sh
subcmd="$1"
mode="${DEVSYNC_TEST_PNPM_MODE}"

if [ "${subcmd}" = "root" ]; then
  if [ "$2" = "-g" ]; then
    if [ "${DEVSYNC_TEST_PNPM_ROOT_MODE}" = "error" ]; then
      echo "root failed" 1>&2
      exit 2
    fi
    if [ "${DEVSYNC_TEST_PNPM_ROOT_MODE}" = "empty" ]; then
      exit 0
    fi
    if [ -n "${DEVSYNC_TEST_PNPM_GLOBAL_DIR}" ]; then
      if [ "${DEVSYNC_TEST_PNPM_ROOT_MODE}" = "plain_dir" ]; then
        echo "${DEVSYNC_TEST_PNPM_GLOBAL_DIR}"
        exit 0
      fi
      echo "${DEVSYNC_TEST_PNPM_GLOBAL_DIR}/node_modules"
      exit 0
    fi
    echo "/tmp/pnpm-global/node_modules"
    exit 0
  fi
fi

if [ "${subcmd}" = "outdated" ]; then
  if [ "${mode}" = "missing_manifest" ]; then
    if [ -f "${DEVSYNC_TEST_PNPM_GLOBAL_DIR}/package.json" ]; then
      echo '[]'
      exit 0
    fi
    echo 'ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND No package.json was found in "'"${DEVSYNC_TEST_PNPM_GLOBAL_DIR}"'".'
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
