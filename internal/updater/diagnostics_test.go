package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scottlz0310/dsx/internal/diag"
)

// useDiagnostics は診断ログを一時ディレクトリへ向け、テストの後始末で無効に戻します。
func useDiagnostics(t *testing.T, capture diag.Capture) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "diag")

	recorder, err := diag.New(diag.Options{Dir: dir, Capture: capture, MaxFiles: 10, MaxTotalBytes: 10 << 20}, diag.Meta{DsxVersion: "test", Command: "sys update"})
	require.NoError(t, err)

	SetDiagnostics(recorder)
	t.Cleanup(func() { SetDiagnostics(nil) })

	return dir
}

// readCaptures は診断ログ（JSON Lines）から、記録（type=capture）の行だけを読みます。
func readCaptures(t *testing.T, dir string) []map[string]any {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "diag-*.jsonl"))
	require.NoError(t, err)

	var captures []map[string]any

	for _, path := range files {
		file, openErr := os.Open(path)
		require.NoError(t, openErr)

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1<<20), 8<<20)

		for scanner.Scan() {
			var line map[string]any

			require.NoError(t, json.Unmarshal(scanner.Bytes(), &line))

			if line["type"] == "capture" {
				captures = append(captures, line)
			}
		}

		require.NoError(t, scanner.Err())
		require.NoError(t, file.Close())
	}

	return captures
}

func TestRecordDiagnostic_Modes(t *testing.T) {
	tests := []struct {
		name       string
		capture    diag.Capture
		failed     bool
		suspect    string
		wantReason string
	}{
		{"anomaly: 異常なしは保存しない", diag.CaptureAnomaly, false, "", ""},
		{"anomaly: 疑いあり", diag.CaptureAnomaly, false, "疑い", "suspect"},
		{"anomaly: コマンド失敗", diag.CaptureAnomaly, true, "", "command_failed"},
		{"all: 異常なしでも保存", diag.CaptureAll, false, "", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := useDiagnostics(t, tt.capture)

			recordDiagnostic(context.Background(), &diagCapture{
				Manager: "winget", Command: []string{"winget", "upgrade"}, Output: []byte("out"),
				Failed: tt.failed, Suspect: tt.suspect,
			})

			captures := readCaptures(t, dir)
			if tt.wantReason == "" {
				assert.Empty(t, captures)

				_, statErr := os.Stat(dir)
				assert.True(t, os.IsNotExist(statErr), "保存しないときはディレクトリも作らない")

				return
			}

			require.Len(t, captures, 1)
			assert.Equal(t, tt.wantReason, captures[0]["reason"])
			assert.Equal(t, "winget", captures[0]["manager"])
			assert.Equal(t, "winget upgrade", captures[0]["command"])
			assert.Equal(t, "out", captures[0]["output"])
		})
	}
}

func TestRecordDiagnostic_DisabledDoesNothing(t *testing.T) {
	SetDiagnostics(nil)

	assert.False(t, DiagnosticsEnabled())

	// 無効（nil）のとき、疑いや失敗があっても何もしない（パニックもしない）
	recordDiagnostic(context.Background(), &diagCapture{Manager: "m", Command: []string{"m"}, Failed: true, Suspect: "疑い"})
}

func TestRecordDiagnostic_ErrorDoesNotStopCaller(t *testing.T) {
	// 保存先がファイルで、ディレクトリを作れない
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	recorder, err := diag.New(diag.Options{Dir: filepath.Join(blocker, "diag"), Capture: diag.CaptureAll, MaxFiles: 1, MaxTotalBytes: 1 << 20}, diag.Meta{})
	require.NoError(t, err)

	SetDiagnostics(recorder)
	t.Cleanup(func() { SetDiagnostics(nil) })

	// 失敗しても呼び出し元へは何も返さず（更新を止めず）、2 回目も同様に続行できる
	recordDiagnostic(context.Background(), &diagCapture{Manager: "m", Command: []string{"m"}})
	recordDiagnostic(context.Background(), &diagCapture{Manager: "m", Command: []string{"m"}})
}

func TestToolVersion(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		args       []string
		wantPrefix string
	}{
		{"go version の 1 行目", "go", []string{"version"}, "go version"},
		{"存在しないコマンドは空", "dsx-no-such-command-for-test", []string{"--version"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolVersion(context.Background(), tt.command, tt.args)

			if tt.wantPrefix == "" {
				assert.Empty(t, got)

				return
			}

			assert.True(t, strings.HasPrefix(got, tt.wantPrefix), "got=%q", got)
			assert.NotContains(t, got, "\n")
		})
	}
}

// writeFakeDiagScoop は、確認系の出力を DSX_TEST_DIAG_MODE で切り替える偽の scoop を置きます。
func writeFakeDiagScoop(t *testing.T, dir string) {
	t.Helper()

	var (
		name    string
		content string
	)

	if runtime.GOOS == "windows" {
		name = "scoop.cmd"
		content = `@echo off
if "%1"=="--version" goto ver
if "%1"=="update" exit /b 0
if "%1"=="status" goto st
exit /b 0
:ver
echo Current Scoop version: vTEST
exit /b 0
:st
if "%DSX_TEST_DIAG_MODE%"=="fail" exit /b 1
if "%DSX_TEST_DIAG_MODE%"=="ok" goto ok
echo Name Installed Version Latest Version
echo fd 10.2.0 10.5.0
exit /b 0
:ok
echo Everything is ok!
exit /b 0
`
	} else {
		name = "scoop"
		content = `#!/bin/sh
case "$1" in
  --version) echo "Current Scoop version: vTEST"; exit 0 ;;
  update) exit 0 ;;
  status)
    case "${DSX_TEST_DIAG_MODE}" in
      fail) exit 1 ;;
      ok) echo "Everything is ok!"; exit 0 ;;
    esac
    echo "Name Installed Version Latest Version"
    echo "fd 10.2.0 10.5.0"
    exit 0 ;;
esac
exit 0
`
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755))
}

func TestScoopCheck_RecordsDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		capture    diag.Capture
		mode       string
		wantReason string
		wantErr    bool
	}{
		{"anomaly: 疑いのある出力を保存する", diag.CaptureAnomaly, "suspect", "suspect", false},
		{"anomaly: 更新なしは保存しない", diag.CaptureAnomaly, "ok", "", false},
		{"anomaly: 出力なしの失敗を保存する", diag.CaptureAnomaly, "fail", "command_failed", true},
		{"all: 更新なしも保存する", diag.CaptureAll, "ok", "all", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := useDiagnostics(t, tt.capture)

			fakeDir := t.TempDir()
			writeFakeDiagScoop(t, fakeDir)
			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_DIAG_MODE", tt.mode)

			_, err := (&ScoopUpdater{}).Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			captures := readCaptures(t, dir)
			if tt.wantReason == "" {
				assert.Empty(t, captures)

				return
			}

			require.Len(t, captures, 1)
			assert.Equal(t, "scoop", captures[0]["manager"])
			assert.Equal(t, tt.wantReason, captures[0]["reason"])
			assert.Equal(t, "scoop status", captures[0]["command"])
			assert.Equal(t, "Current Scoop version: vTEST", captures[0]["tool_version"])

			if tt.mode == "suspect" {
				assert.Contains(t, captures[0]["output"], "Latest Version", "実際の出力を保存する")
				assert.NotEmpty(t, captures[0]["suspect"])
			}
		})
	}
}

func TestAptCheck_RecordsCommandFailure(t *testing.T) {
	dir := useDiagnostics(t, diag.CaptureAnomaly)

	fakeDir := t.TempDir()
	writeFakeAptCommand(t, fakeDir)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DSX_TEST_APT_MODE", "list_error")

	_, err := (&AptUpdater{useSudo: false}).Check(context.Background())
	require.Error(t, err)

	captures := readCaptures(t, dir)
	require.Len(t, captures, 1)
	assert.Equal(t, "apt", captures[0]["manager"])
	assert.Equal(t, "command_failed", captures[0]["reason"])
	assert.Equal(t, "apt list --upgradable", captures[0]["command"])
	assert.Contains(t, captures[0]["error"], "更新可能パッケージの取得に失敗")
}
