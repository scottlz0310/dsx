//go:build windows

package updater

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helperEnv = "DSX_TEST_UV_PROCESS_HELPER"

// TestMain は、uvx.exe として起動された子プロセスのときだけ、標準入力が閉じるまで待機します。
func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		fmt.Println("ready")

		_, _ = io.Copy(io.Discard, os.Stdin) //nolint:errcheck // 親が標準入力を閉じるまで待機するだけ

		return
	}

	os.Exit(m.Run())
}

// startFakeUV はテストバイナリを dir に uvx.exe として複製して起動し、起動を確認してから返します。
func startFakeUV(t *testing.T, dir string) *exec.Cmd {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	data, err := os.ReadFile(self)
	require.NoError(t, err)

	fake := filepath.Join(dir, "uvx.exe")
	require.NoError(t, os.WriteFile(fake, data, 0o755))

	cmd := exec.CommandContext(t.Context(), fake)

	cmd.Env = append(os.Environ(), helperEnv+"=1")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close() //nolint:errcheck // 待機中の子プロセスを終了させるだけ
		_ = cmd.Wait()    //nolint:errcheck // 終了コードは見ない
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready", strings.TrimSpace(line))

	return cmd
}

func TestListRunningExecutables(t *testing.T) {
	runningDir := t.TempDir()
	otherDir := t.TempDir()
	cmd := startFakeUV(t, runningDir)
	wantProcess := fmt.Sprintf("uvx.exe (PID %d)", cmd.Process.Pid)

	tests := []struct {
		name     string
		dir      string
		names    []string
		wantSome bool
	}{
		{name: "同じディレクトリの実行中の uvx を検出する", dir: runningDir, names: uvExecutableNames, wantSome: true},
		{name: "名前の大文字小文字は区別しない", dir: runningDir, names: []string{"UVX.EXE"}, wantSome: true},
		{name: "ディレクトリの末尾の区切りや大文字小文字が違っても検出する", dir: strings.ToUpper(runningDir) + `\`, names: uvExecutableNames, wantSome: true},
		{name: "別のディレクトリの uv は対象外", dir: otherDir, names: uvExecutableNames},
		{name: "対象外の名前は検出しない", dir: runningDir, names: []string{"uv.exe", "uvw.exe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := listRunningExecutables(tt.dir, tt.names)
			require.NoError(t, err)

			if tt.wantSome {
				assert.Contains(t, got, wantProcess)
			} else {
				assert.NotContains(t, got, wantProcess)
				assert.Empty(t, got)
			}
		})
	}
}
