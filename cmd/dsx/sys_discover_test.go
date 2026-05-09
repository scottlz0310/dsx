package main

import (
	"context"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/updater"
)

func TestIsSupportedDiscoverManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "go は対応している",
			input: "go",
			want:  true,
		},
		{
			name:  "apt は未対応",
			input: "apt",
			want:  false,
		},
		{
			name:  "空文字列は未対応",
			input: "",
			want:  false,
		},
		{
			name:  "大文字 GO は未対応（大文字小文字区別）",
			input: "GO",
			want:  false,
		},
		{
			name:  "brew は未対応",
			input: "brew",
			want:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isSupportedDiscoverManager(tc.input)
			if got != tc.want {
				t.Fatalf("isSupportedDiscoverManager(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolveDiscoverManagers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		manager string
		want    []string
		wantErr bool
	}{
		{
			name:    "空文字列は全対応マネージャを返す",
			manager: "",
			want:    supportedDiscoverManagers,
			wantErr: false,
		},
		{
			name:    "go を指定すると go のみ返す",
			manager: "go",
			want:    []string{"go"},
			wantErr: false,
		},
		{
			name:    "未対応マネージャはエラー",
			manager: "apt",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "未知の文字列はエラー",
			manager: "invalid",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveDiscoverManagers(tc.manager)

			if tc.wantErr {
				if err == nil {
					t.Fatal("エラーを期待したが nil が返った")
				}
				return
			}

			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("resolveDiscoverManagers(%q) len = %d, want %d", tc.manager, len(got), len(tc.want))
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("resolveDiscoverManagers(%q)[%d] = %q, want %q", tc.manager, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPrintGoDiscoverResult_NoneDetected(t *testing.T) {
	result := &updater.DiscoverResult{}
	out := captureStdout(t, func() {
		printGoDiscoverResult(result)
	})

	if !strings.Contains(out, "ありませんでした") {
		t.Fatalf("出力に「ありませんでした」が含まれていない: %q", out)
	}
}

func TestPrintGoDiscoverResult_WithDetected(t *testing.T) {
	result := &updater.DiscoverResult{
		Detected: []updater.GoBinaryInfo{
			{
				BinaryName:       "gopls",
				PackagePath:      "golang.org/x/tools/gopls",
				InstalledVersion: "v0.17.1",
			},
			{
				BinaryName:  "dlv",
				PackagePath: "github.com/go-delve/delve/cmd/dlv",
				// InstalledVersion 未設定
			},
		},
	}

	out := captureStdout(t, func() {
		printGoDiscoverResult(result)
	})

	if !strings.Contains(out, "[go] 検出されたバイナリ:") {
		t.Fatalf("ヘッダーが出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "gopls") {
		t.Fatalf("gopls が出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "v0.17.1") {
		t.Fatalf("バージョンが出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "@latest") {
		t.Fatalf("@latest が出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "go.targets") {
		t.Fatalf("ヒントメッセージが出力に含まれていない: %q", out)
	}
}

func TestPrintGoDiscoverResult_WithSkipped(t *testing.T) {
	result := &updater.DiscoverResult{
		Detected: []updater.GoBinaryInfo{
			{
				BinaryName:       "gopls",
				PackagePath:      "golang.org/x/tools/gopls",
				InstalledVersion: "v0.17.1",
			},
		},
		Skipped: []updater.SkippedBinary{
			{Name: "unknown.exe", Reason: "Go モジュール情報なし"},
			{Name: "backup~", Reason: "バックアップファイル"},
		},
	}

	out := captureStdout(t, func() {
		printGoDiscoverResult(result)
	})

	if !strings.Contains(out, "スキップ:") {
		t.Fatalf("スキップセクションが出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "unknown.exe") {
		t.Fatalf("unknown.exe が出力に含まれていない: %q", out)
	}

	if !strings.Contains(out, "backup~") {
		t.Fatalf("backup~ が出力に含まれていない: %q", out)
	}
}

func TestPrintGoDiscoverResult_SkippedSectionOmittedWhenEmpty(t *testing.T) {
	result := &updater.DiscoverResult{
		Detected: []updater.GoBinaryInfo{
			{
				BinaryName:  "mytool",
				PackagePath: "github.com/foo/mytool",
			},
		},
		// Skipped は空
	}

	out := captureStdout(t, func() {
		printGoDiscoverResult(result)
	})

	if strings.Contains(out, "スキップ:") {
		t.Fatalf("スキップが 0 件なのにスキップセクションが表示されている: %q", out)
	}
}

func TestResolveDiscoverManagers_ErrorMessage(t *testing.T) {
	t.Parallel()

	// エラーメッセージに未対応マネージャ名が含まれることを確認
	_, err := resolveDiscoverManagers("brew")
	if err == nil {
		t.Fatal("未対応マネージャでエラーが返らなかった")
	}

	if !strings.Contains(err.Error(), "brew") {
		t.Fatalf("エラーメッセージにマネージャ名「brew」が含まれていない: %q", err.Error())
	}
}

func TestDiscoverManager_UnsupportedManagerReturnsError(t *testing.T) {
	t.Parallel()

	err := discoverManager(context.Background(), "apt")
	if err == nil {
		t.Fatal("未対応マネージャでエラーが返らなかった")
	}

	if !strings.Contains(err.Error(), "apt") {
		t.Fatalf("エラーメッセージに「apt」が含まれていない: %q", err.Error())
	}
}

// TestPrintGoDiscoverResult_InstalledVersionEmpty は InstalledVersion が空の場合に
// バージョン表記が省略されることを確認します。
func TestPrintGoDiscoverResult_InstalledVersionEmpty(t *testing.T) {
	result := &updater.DiscoverResult{
		Detected: []updater.GoBinaryInfo{
			{
				BinaryName:  "mytool",
				PackagePath: "github.com/foo/mytool",
				// InstalledVersion は空
			},
		},
	}

	out := captureStdout(t, func() {
		printGoDiscoverResult(result)
	})

	if strings.Contains(out, "インストール済み:") {
		t.Fatalf("バージョン未設定なのに「インストール済み:」が表示されている: %q", out)
	}

	if !strings.Contains(out, "github.com/foo/mytool@latest") {
		t.Fatalf("UpdateTarget の出力 %q が含まれていない: %q", "github.com/foo/mytool@latest", out)
	}
}
