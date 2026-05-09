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

	err := discoverManager(context.Background(), "apt", false, false)
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

func TestPackagePathFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "@latest あり",
			input: "golang.org/x/tools/gopls@latest",
			want:  "golang.org/x/tools/gopls",
		},
		{
			name:  "@バージョン固定あり",
			input: "golang.org/x/tools/gopls@v0.16.0",
			want:  "golang.org/x/tools/gopls",
		},
		{
			name:  "@ なし",
			input: "golang.org/x/tools/gopls",
			want:  "golang.org/x/tools/gopls",
		},
		{
			name:  "空文字列",
			input: "",
			want:  "",
		},
		{
			name:  "@ のみ",
			input: "@latest",
			want:  "",
		},
		{
			name:  "複数 @ は最後の @ で分割",
			input: "example.com/user@host/pkg@v1.0.0",
			want:  "example.com/user@host/pkg",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := packagePathFrom(tc.input)
			if got != tc.want {
				t.Fatalf("packagePathFrom(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMergeGoTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		existing    []string
		detected    []updater.GoBinaryInfo
		wantToAdd   []string
		wantSkipped int
	}{
		{
			name:        "既存なし・検出なし",
			existing:    nil,
			detected:    nil,
			wantToAdd:   nil,
			wantSkipped: 0,
		},
		{
			name:     "既存なし・検出あり",
			existing: nil,
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
				{PackagePath: "github.com/go-delve/delve/cmd/dlv"},
			},
			wantToAdd:   []string{"golang.org/x/tools/gopls@latest", "github.com/go-delve/delve/cmd/dlv@latest"},
			wantSkipped: 0,
		},
		{
			name:     "既存あり・重複なし",
			existing: []string{"github.com/fatih/gomodifytags@latest"},
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
			},
			wantToAdd:   []string{"golang.org/x/tools/gopls@latest"},
			wantSkipped: 0,
		},
		{
			name:     "既存あり・全重複",
			existing: []string{"golang.org/x/tools/gopls@latest"},
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
			},
			wantToAdd:   nil,
			wantSkipped: 1,
		},
		{
			name:     "既存が固定バージョン・検出は @latest → 重複として skip",
			existing: []string{"golang.org/x/tools/gopls@v0.16.0"},
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
			},
			wantToAdd:   nil,
			wantSkipped: 1,
		},
		{
			name:     "部分重複",
			existing: []string{"golang.org/x/tools/gopls@latest"},
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
				{PackagePath: "github.com/go-delve/delve/cmd/dlv"},
			},
			wantToAdd:   []string{"github.com/go-delve/delve/cmd/dlv@latest"},
			wantSkipped: 1,
		},
		{
			name:     "PackagePath 空の検出はスキップ",
			existing: nil,
			detected: []updater.GoBinaryInfo{
				{PackagePath: ""},
			},
			wantToAdd:   nil,
			wantSkipped: 0,
		},
		{
			name:     "detected 側に同一 PackagePath が複数 → 重複を除外して 1 件のみ追加",
			existing: nil,
			detected: []updater.GoBinaryInfo{
				{PackagePath: "golang.org/x/tools/gopls"},
				{PackagePath: "golang.org/x/tools/gopls"},
			},
			wantToAdd:   []string{"golang.org/x/tools/gopls@latest"},
			wantSkipped: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotToAdd, gotSkipped := mergeGoTargets(tc.existing, tc.detected)

			if gotSkipped != tc.wantSkipped {
				t.Fatalf("skippedCount = %d, want %d", gotSkipped, tc.wantSkipped)
			}

			if len(gotToAdd) != len(tc.wantToAdd) {
				t.Fatalf("toAdd len = %d, want %d (got=%v, want=%v)", len(gotToAdd), len(tc.wantToAdd), gotToAdd, tc.wantToAdd)
			}

			for i := range gotToAdd {
				if gotToAdd[i] != tc.wantToAdd[i] {
					t.Fatalf("toAdd[%d] = %q, want %q", i, gotToAdd[i], tc.wantToAdd[i])
				}
			}
		})
	}
}

func TestPrintGoApplyDryRun_NoAdditions(t *testing.T) {
	out := captureStdout(t, func() {
		printGoApplyDryRun(nil, 2, "/home/user/.config/dsx/config.yaml", true)
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Fatalf("[dry-run] ラベルが含まれていない: %q", out)
	}

	if !strings.Contains(out, "追加対象なし") {
		t.Fatalf("「追加対象なし」が含まれていない: %q", out)
	}

	if !strings.Contains(out, "スキップ: 2 件") {
		t.Fatalf("スキップ件数が含まれていない: %q", out)
	}
}

func TestPrintGoApplyDryRun_WithAdditions(t *testing.T) {
	toAdd := []string{
		"golang.org/x/tools/gopls@latest",
		"github.com/go-delve/delve/cmd/dlv@latest",
	}

	out := captureStdout(t, func() {
		printGoApplyDryRun(toAdd, 1, "/home/user/.config/dsx/config.yaml", true)
	})

	if !strings.Contains(out, "+ golang.org/x/tools/gopls@latest") {
		t.Fatalf("追加エントリが含まれていない: %q", out)
	}

	if !strings.Contains(out, "+ github.com/go-delve/delve/cmd/dlv@latest") {
		t.Fatalf("追加エントリが含まれていない: %q", out)
	}

	if !strings.Contains(out, "スキップ: 1 件") {
		t.Fatalf("スキップ件数が含まれていない: %q", out)
	}
}

func TestPrintGoApplyDryRun_ConfigNotExist(t *testing.T) {
	out := captureStdout(t, func() {
		printGoApplyDryRun([]string{"golang.org/x/tools/gopls@latest"}, 0, "/home/user/.config/dsx/config.yaml", false)
	})

	if !strings.Contains(out, "新規作成") {
		t.Fatalf("新規作成メッセージが含まれていない: %q", out)
	}
}

func TestPrintGoApplyDryRun_CommentLossWarning(t *testing.T) {
	out := captureStdout(t, func() {
		printGoApplyDryRun(nil, 0, "/home/user/.config/dsx/config.yaml", true)
	})

	if !strings.Contains(out, "コメントは書き込み時に保持されません") {
		t.Fatalf("コメント喪失警告が含まれていない: %q", out)
	}
}
