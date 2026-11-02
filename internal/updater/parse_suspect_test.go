package updater

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorruptedPackageReason(t *testing.T) {
	tests := []struct {
		name     string
		packages []PackageInfo
		wantSet  bool
	}{
		{"空", nil, false},
		{"正常", []PackageInfo{{Name: "Git", CurrentVersion: "1.0", NewVersion: "2.0"}}, false},
		{"日本語の名前は正常", []PackageInfo{{Name: "メモ帳", CurrentVersion: "1.0", NewVersion: "2.0"}}, false},
		{"名前に不正な UTF-8", []PackageInfo{{Name: "ok"}, {Name: "bad\xff", NewVersion: "2.0"}}, true},
		{"バージョンに不正な UTF-8", []PackageInfo{{Name: "a", CurrentVersion: "1.0\xff"}}, true},
		{"置換文字を含む", []PackageInfo{{Name: "a�"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := corruptedPackageReason(tt.packages)
			assert.Equal(t, tt.wantSet, got != "", "理由: %q", got)
		})
	}
}

func TestWingetSuspect(t *testing.T) {
	w := &WingetUpdater{}

	tests := []struct {
		name    string
		output  string
		wantSet bool
	}{
		{
			// 実機（ja-JP）の出力。更新可能 2 件に加え、明示的対象化が必要な表がある
			name: "実機の日本語出力は疑いなし",
			output: `名前                                            ID                   バージョン 利用可能     ソース
---------------------------------------------------------------------------------------------------
Google Cloud SDK                                Google.CloudSDK      Unknown    591.0.0      winget
Microsoft Build of OpenJDK  17.0.21.0.2+1 (x64) Microsoft.OpenJDK.17 17.0.21.21 17.0.21.1021 winget
3 アップグレードを利用できます。

次のパッケージにはアップグレードを適用可能ですが、アップグレードには明示的対象化が必要です:
名前           ID                   バージョン 利用可能   ソース
----------------------------------------------------------------
Android Studio Google.AndroidStudio 2026.1     2026.2.2.8 winget
`,
			wantSet: false,
		},
		{"更新なしのメッセージ（表なし）は疑いなし", "No applicable upgrade found.\n", false},
		{"空出力は疑いなし", "", false},
		{
			// 英語ロケールの実出力は未採取のため想定の入力
			name: "明示的対象化が必要な表だけなら疑いなし",
			output: `The following packages have an upgrade available, but require explicit targeting for upgrade:
Name           ID                   Version Available  Source
--------------------------------------------------------------
Android Studio Google.AndroidStudio 2026.1  2026.2.2.8 winget
`,
			wantSet: false,
		},
		{
			name: "ヘッダーの列が足りず表を読めない",
			output: `Name   ID
------------------
App    App.One
`,
			wantSet: true,
		},
		{
			name: "区切り行に想定外の装飾が付いて表を読めない",
			output: "Name       ID        Version Available Source\n" +
				"\x1b[2K------------------------------------------------\n" +
				"App One    App.One   1.0.0   2.0.0     winget\n",
			wantSet: true,
		},
		{
			name: "表はあるが行を 1 件も読めない",
			output: `Name       ID        Version Available Source
------------------------------------------------
`,
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, tableFound := parseWingetUpgradeTable(tt.output)
			got := wingetSuspect(packages, tableFound)

			assert.Equal(t, tt.wantSet, got != "", "理由: %q", got)
			assert.Equal(t, w.parseUpgradeOutput(tt.output), packages, "parseUpgradeOutput と一致する")
		})
	}
}

func TestWingetSuspect_CorruptedPackages(t *testing.T) {
	assert.NotEmpty(t, wingetSuspect([]PackageInfo{{Name: "bad\xff"}}, true))
}

func TestScoopSuspect(t *testing.T) {
	s := &ScoopUpdater{}

	tests := []struct {
		name    string
		output  string
		wantSet bool
	}{
		{"更新なし", "Scoop is up to date.\nEverything is ok!\n", false},
		{"空出力", "", false},
		{
			name: "更新可能な表（解析できる）",
			output: `Name Installed Version Latest Version Missing Dependencies Info
---- ----------------- -------------- -------------------- ----
fd   10.2.0            10.5.0
`,
			wantSet: false,
		},
		{
			// v0.14.0 までの不具合の条件: pwsh 7 の色付き出力（現在は除去して解析できる）
			name: "pwsh 7 の色付き出力は解析できて疑いなし",
			output: "\x1b[32;1mName Installed Version Latest Version Missing Dependencies Info\x1b[0m\n" +
				"\x1b[32;1m---- ----------------- -------------- -------------------- ----\x1b[0m\n" +
				"fd   10.2.0            10.5.0\n",
			wantSet: false,
		},
		{
			name: "区切り行に除去できない装飾が付いて表を読めない",
			output: "Name Installed Version Latest Version Missing Dependencies Info\n" +
				"---- ----------------- -------------- -------------------- ----\x1b[K\n" +
				"fd   10.2.0            10.5.0\n",
			wantSet: true,
		},
		{
			name:    "見出しだけで区切り行がない",
			output:  "Name Installed Version Latest Version\nfd 10.2.0 10.5.0\n",
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages := s.parseStatusOutput(tt.output)
			got := scoopSuspect(tt.output, packages)

			assert.Equal(t, tt.wantSet, got != "", "理由: %q", got)
		})
	}
}

func TestAptSuspect(t *testing.T) {
	a := &AptUpdater{}

	tests := []struct {
		name    string
		output  string
		wantSet bool
	}{
		{"空出力", "", false},
		{"更新なし", "Listing... Done\n", false},
		{
			name:    "英語の出力は疑いなし",
			output:  "Listing... Done\nvim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]\n",
			wantSet: false,
		},
		{
			// 実機（Ubuntu 24.04、ja_JP.UTF-8）の出力。旧バージョンが全件で取れない
			name: "日本語に翻訳された出力は疑い",
			output: `一覧表示...
base-files/noble-updates 13ubuntu10.5 amd64 [13ubuntu10.4 からアップグレード可]
libssl3t64/noble-updates,noble-security 3.0.13-0ubuntu3.16 amd64 [3.0.13-0ubuntu3.15 からアップグレード可]
`,
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages := a.parseUpgradableList(tt.output)
			got := aptSuspect(packages)

			assert.Equal(t, tt.wantSet, got != "", "理由: %q", got)
		})
	}
}

func TestRustupSuspect(t *testing.T) {
	r := &RustupUpdater{}

	tests := []struct {
		name    string
		output  string
		wantSet bool
	}{
		{"空出力", "", false},
		{"更新なし", "stable-x86_64-unknown-linux-gnu - Up to date : 1.80.0\nrustup - Up to date : 1.27.1\n", false},
		{
			// 実機（Windows、rustup 1.29.2）の出力
			name:    "実機の更新あり出力は疑いなし",
			output:  "1.99-x86_64-pc-windows-msvc - update available: 1.99.0 (b940084d7 2026-09-28) -> 1.99.1 (58cf8f9c3 2026-10-13)\nrustup - up to date : 1.29.2\n",
			wantSet: false,
		},
		{
			name:    "更新ありの行の書式が変わって読めない",
			output:  "stable-x86_64-unknown-linux-gnu - Update available (1.80.0 to 1.81.0)\n",
			wantSet: true,
		},
		{
			name:    "一部の行だけ読めない",
			output:  "stable-x86_64-unknown-linux-gnu - Update available : 1.80.0 -> 1.81.0\nnightly - Update available (a to b)\n",
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages := r.parseCheckOutput(tt.output)
			got := rustupSuspect(tt.output, packages)

			assert.Equal(t, tt.wantSet, got != "", "理由: %q", got)
		})
	}
}

func TestRunCountBasedUpdate_PropagatesSuspect(t *testing.T) {
	const reason = "判定不能の疑い"

	tests := []struct {
		name   string
		check  *CheckResult
		dryRun bool
	}{
		{"更新なし（0 件）でも疑いを引き継ぐ", &CheckResult{Suspect: reason}, false},
		{"DryRun でも疑いを引き継ぐ", &CheckResult{AvailableUpdates: 1, Packages: []PackageInfo{{Name: "a"}}, Suspect: reason}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runCountBasedUpdate(
				t.Context(), UpdateOptions{DryRun: tt.dryRun}, tt.check,
				"最新です",
				func(int) string { return "dry" },
				"unused", nil, "%w",
				func(int) string { return "done" },
			)

			assert.NoError(t, err)
			assert.Equal(t, reason, result.Suspect)
		})
	}
}
