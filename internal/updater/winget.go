package updater

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"

	"github.com/scottlz0310/dsx/internal/config"
)

// WingetUpdater は Windows Package Manager (winget) の実装です。
type WingetUpdater struct {
	snapshotRun snapshotCommandRunner
}

// 起動時にレジストリに登録
func init() {
	Register(&WingetUpdater{})
}

func (w *WingetUpdater) Name() string {
	return "winget"
}

func (w *WingetUpdater) DisplayName() string {
	return "winget (Windows Package Manager)"
}

func (w *WingetUpdater) IsAvailable() bool {
	_, err := exec.LookPath("winget")
	return err == nil
}

func (w *WingetUpdater) Configure(cfg config.ManagerConfig) error {
	return nil
}

func (w *WingetUpdater) Check(ctx context.Context) (*CheckResult, error) {
	cmd := exec.CommandContext(ctx, "winget", upgradeCommand, "--include-unknown", "--disable-interactivity", "--accept-source-agreements")

	// stderr も含めた出力を取得することで、エラー時の診断情報を失わないようにする。
	// winget はアップグレード可能なパッケージがある場合も exit code 0 以外を返すことがあるため、
	// まずは出力をパースしてみて、アップグレード候補が取得できるかを確認する。
	output, err := cmd.CombinedOutput()

	packages, tableFound := parseWingetUpgradeTable(string(output))

	// コマンドがエラー終了しており、かつパース結果が 0 件の場合は、
	// 実際には失敗している可能性が高いためエラーとして扱う。
	if err != nil && len(packages) == 0 {
		return nil, fmt.Errorf("winget upgrade の実行に失敗: %w", buildCommandOutputErr(err, output))
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
		Suspect:          wingetSuspect(packages, tableFound),
	}, nil
}

func (w *WingetUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := w.Check(ctx)
	if err != nil {
		return nil, err
	}

	return runCountBasedUpdate(
		ctx, opts, checkResult,
		"すべての winget パッケージは最新です",
		func(count int) string {
			return fmt.Sprintf("%d 件の winget パッケージが更新可能です（DryRunモード）", count)
		},
		"winget",
		[]string{upgradeCommand, "--all", "--include-unknown", "--disable-interactivity", "--accept-source-agreements", "--accept-package-agreements"},
		"winget upgrade --all に失敗: %w",
		func(count int) string {
			return fmt.Sprintf("%d 件の winget パッケージを更新しました", count)
		},
	)
}

// wingetCondition は winget が表の桁を揃えるときの表示幅（全角=2、半角=1）を数えます。
// 曖昧幅の文字（"…" など）は 1 として数えます。ヘッダーの桁位置は表示幅で決まるため、
// バイト位置や文字数で切ると日本語ロケールの出力で列がずれます。
var wingetCondition = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false

	return c
}()

// parseUpgradeOutput は "winget upgrade --include-unknown" の出力をパースします。
//
// 出力形式（固定幅テーブル。桁は表示幅で揃えられる）:
//
//	Name                                   ID                               Version              Available            Source
//	-----------------------------------------------------------------------------------------------------------------------
//	Docker Desktop                         Docker.DockerDesktop             4.59.0               4.60.0               winget
//	GitHub CLI                             GitHub.cli                       2.83.2               2.86.0               winget
//	2 upgrades available.
//
// 先頭の表だけを更新対象とします。"upgrade --all" が更新しない「明示的対象化が必要」な
// パッケージの表（"…:" で終わる導入文の後ろ）は数えません。
// 列位置はヘッダー行から表示幅で算出します。
func (w *WingetUpdater) parseUpgradeOutput(output string) []PackageInfo {
	packages, _ := parseWingetUpgradeTable(output)

	return packages
}

// wingetSeparatorToken は表の区切り行（ダッシュの連続）を見分ける目印です。
const wingetSeparatorToken = "----------"

// parseWingetUpgradeTable は先頭の表を読み取ります。
// tableFound は更新可能パッケージの表（のような出力）を検出したかどうかで、
// 検出したのに読み取れなかった場合を「判定不能の疑い」として検知するために返します。
func parseWingetUpgradeTable(output string) (packages []PackageInfo, tableFound bool) {
	lines := splitLines(output)

	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.Contains(trimmed, wingetSeparatorToken) {
			continue
		}

		// 導入文（"…が必要です:" など）を伴う表は "upgrade --all" の対象ではない
		if i >= 2 && isWingetSubListHeading(lines[i-2]) {
			continue
		}

		// 区切り行らしいが、ダッシュだけではない（想定外の装飾が付いている等）
		if !isAllDashes(trimmed) {
			return nil, true
		}

		cols := wingetColumnStarts(lines[i-1])
		if len(cols) < 4 {
			return nil, true
		}

		return parseWingetRows(lines[i+1:], cols), true
	}

	return nil, false
}

// wingetSuspect は winget の出力を正しく解釈できなかった疑いの理由を返します（疑いがなければ空文字列）。
func wingetSuspect(packages []PackageInfo, tableFound bool) string {
	if reason := corruptedPackageReason(packages); reason != "" {
		return reason
	}

	if tableFound && len(packages) == 0 {
		return "winget の更新可能パッケージの表を検出しましたが、行を 1 件も解釈できませんでした（出力形式が想定と異なる可能性）"
	}

	return ""
}

// parseWingetRows は表の行を列位置（表示幅）で切り出します。
// 空行・サマリー行・ID 列に空白を含む行（表の後ろの案内文）で表の終わりと判断します。
func parseWingetRows(rows []string, cols []int) []PackageInfo {
	availableEnd := -1
	if len(cols) >= 5 {
		availableEnd = cols[4]
	}

	packages := make([]PackageInfo, 0)

	for _, line := range rows {
		if strings.TrimSpace(line) == "" || isSummaryLine(line) {
			break
		}

		id := strings.TrimSpace(sliceByDisplayWidth(line, cols[1], cols[2]))
		if strings.ContainsAny(id, " \t") {
			break
		}

		if id == "" {
			continue
		}

		packages = append(packages, PackageInfo{
			Name:           strings.TrimSpace(sliceByDisplayWidth(line, cols[0], cols[1])),
			CurrentVersion: strings.TrimSpace(sliceByDisplayWidth(line, cols[2], cols[3])),
			NewVersion:     strings.TrimSpace(sliceByDisplayWidth(line, cols[3], availableEnd)),
		})
	}

	return packages
}

// isWingetSubListHeading は表の導入文（"…:" または "…：" で終わる行）かどうかを判定します。
func isWingetSubListHeading(line string) bool {
	trimmed := strings.TrimSpace(line)

	return strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "：")
}

// wingetColumnStarts はヘッダー行から各列の開始位置（表示幅）を返します。
// 列名は空白を含まないため、空白で区切られた語の開始位置を列の開始位置とします。
func wingetColumnStarts(header string) []int {
	var starts []int

	col := 0
	inSpace := true

	for _, r := range header {
		if unicode.IsSpace(r) {
			inSpace = true
		} else if inSpace {
			starts = append(starts, col)
			inSpace = false
		}

		col += wingetCondition.RuneWidth(r)
	}

	return starts
}

// sliceByDisplayWidth は表示幅で [start, end) に開始位置が入る文字を切り出します。
// end が負のときは行末までを返します。
func sliceByDisplayWidth(s string, start, end int) string {
	var b strings.Builder

	col := 0

	for _, r := range s {
		if col >= start && (end < 0 || col < end) {
			b.WriteRune(r)
		}

		col += wingetCondition.RuneWidth(r)
	}

	return b.String()
}

// splitLines は出力を行ごとに分割し、プログレスバー等のゴミを除去します。
func splitLines(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))

	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		// プログレスバー文字（█▓░━）を含む行を除去
		if containsProgressChars(line) {
			continue
		}

		lines = append(lines, line)
	}

	return lines
}

// containsProgressChars はプログレスバー関連の文字を含むか判定します。
func containsProgressChars(s string) bool {
	for _, r := range s {
		switch r {
		case '█', '▓', '░', '━', '╸', '╺':
			return true
		}
	}

	return false
}

// isAllDashes は文字列がダッシュのみで構成されているか判定します。
func isAllDashes(s string) bool {
	for _, r := range s {
		if r != '-' {
			return false
		}
	}

	return true
}

// isSummaryLine はサマリー行（末尾の集計行）かどうかを判定します。
func isSummaryLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// 数字で始まるサマリー行を検出
	if unicode.IsDigit(rune(trimmed[0])) {
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "upgrade") || strings.Contains(lower, "アップグレード") {
			return true
		}
	}

	// "No applicable upgrade found." のようなメッセージ
	if strings.Contains(strings.ToLower(trimmed), "no applicable") {
		return true
	}

	return false
}

// safeSubstring は文字列の部分文字列を安全に取得します。
func safeSubstring(s string, start, end int) string {
	if start >= len(s) {
		return ""
	}

	if end > len(s) {
		end = len(s)
	}

	return s[start:end]
}

// safeSubstringToEnd は文字列の指定位置から末尾までを安全に取得します。
func safeSubstringToEnd(s string, start int) string {
	if start >= len(s) {
		return ""
	}

	return s[start:]
}

// インターフェース準拠の確認（推奨）
var _ Updater = (*WingetUpdater)(nil)
