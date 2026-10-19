package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// BW_SESSION の未設定は、日常の更新（run / sys / repo）では正常なため、doctor の失敗にしない（#144）。
// 失敗なら runDoctor は os.Exit(1) するので、このテストプロセスごと落ちる。
func TestRunDoctor_BWSessionUnsetIsNotFailure(t *testing.T) {
	setupSecretsEnabledConfig(t)

	// git と bw が見つかる状態にする（bw は中身を実行しない。PATH 上の存在だけが検査される）。
	binDir := t.TempDir()
	fakeBW := "bw"
	script := "#!/bin/sh\nexit 0\n"

	if runtime.GOOS == "windows" {
		fakeBW = "bw.cmd"
		script = "@exit /b 0\r\n"
	}

	if err := os.WriteFile(filepath.Join(binDir, fakeBW), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git が PATH に無いためスキップします")
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitPath))
	t.Setenv("BW_SESSION", "")

	stdout := captureStdout(t, runDoctor)

	if !strings.Contains(stdout, "シークレット管理 (Bitwarden)") {
		t.Fatalf("runDoctor() の出力にシークレット管理の節がありません: %q", stdout)
	}
}
