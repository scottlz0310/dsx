package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottlz0310/devsync/internal/testutil"
	"github.com/spf13/cobra"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() unexpected error: %v", err)
	}

	os.Stdout = writeEnd

	var (
		buf      bytes.Buffer
		copyErr  error
		copyDone = make(chan struct{})
	)

	defer func() {
		os.Stdout = original

		if writeEnd != nil {
			if err := writeEnd.Close(); err != nil {
				t.Errorf("stdout writer close error: %v", err)
			}
		}

		if readEnd != nil {
			if err := readEnd.Close(); err != nil {
				t.Errorf("stdout reader close error: %v", err)
			}
		}
	}()

	go func() {
		_, copyErr = io.Copy(&buf, readEnd)

		close(copyDone)
	}()

	fn()

	// EOF を通知して reader 側のコピーを完了させる。
	if closeErr := writeEnd.Close(); closeErr != nil {
		t.Fatalf("stdout writer close error: %v", closeErr)
	}

	writeEnd = nil
	os.Stdout = original

	<-copyDone

	if copyErr != nil {
		t.Fatalf("failed to copy stdout: %v", copyErr)
	}

	if closeErr := readEnd.Close(); closeErr != nil {
		t.Fatalf("stdout reader close error: %v", closeErr)
	}

	readEnd = nil

	return buf.String()
}

func TestRunConfigShow_ConfigMissing(t *testing.T) {
	home := t.TempDir()
	testutil.SetTestHome(t, home)

	out := captureStdout(t, func() {
		if err := runConfigShow(&cobra.Command{Use: "show"}, nil); err != nil {
			t.Fatalf("runConfigShow() error = %v", err)
		}
	})

	if !strings.Contains(out, "設定ファイル") {
		t.Fatalf("output does not contain config path info: %q", out)
	}

	if !strings.Contains(out, "未作成") {
		t.Fatalf("output does not contain missing config notice: %q", out)
	}

	if !strings.Contains(out, "version: 1") {
		t.Fatalf("output does not contain YAML config: %q", out)
	}
}

func TestRunConfigValidate_InvalidRepoRoot(t *testing.T) {
	home := t.TempDir()
	testutil.SetTestHome(t, home)

	configDir := filepath.Join(home, ".config", "devsync")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// repo.root に ~ を入れて、validate がエラーになることを確認する。
	configBody := `
version: 1
control:
  concurrency: 8
  timeout: "10m"
repo:
  root: "~/src"
  github:
    protocol: https
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configBody), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	var gotErr error

	out := captureStdout(t, func() {
		gotErr = runConfigValidate(&cobra.Command{Use: "validate"}, nil)
	})

	if gotErr == nil {
		t.Fatalf("runConfigValidate() error = nil, want error")
	}

	if !strings.Contains(out, "❌ エラー") {
		t.Fatalf("output does not contain error header: %q", out)
	}

	if !strings.Contains(out, "repo.root") {
		t.Fatalf("output does not mention repo.root: %q", out)
	}
}
