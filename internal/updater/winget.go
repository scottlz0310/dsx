package updater

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"github.com/scottlz0310/dsx/internal/config"
)

// WingetUpdater は Windows Package Manager (winget) の実装です。
type WingetUpdater struct{}

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
	cmd := exec.CommandContext(ctx, "winget", "upgrade", "--include-unknown", "--disable-interactivity", "--accept-source-agreements")

	// stderr も含めた出力を取得することで、エラー時の診断情報を失わないようにする。
	// winget はアップグレード可能なパッケージがある場合も exit code 0 以外を返すことがあるため、
	// まずは出力をパースしてみて、アップグレード候補が取得できるかを確認する。
	output, err := cmd.CombinedOutput()

	packages := w.parseUpgradeOutput(string(output))

	// コマンドがエラー終了しており、かつパース結果が 0 件の場合は、
	// 実際には失敗している可能性が高いためエラーとして扱う。
	if err != nil && len(packages) == 0 {
		return nil, fmt.Errorf("winget upgrade の実行に失敗: %w", buildCommandOutputErr(err, output))
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
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
		[]string{"upgrade", "--all", "--include-unknown", "--disable-interactivity", "--accept-source-agreements", "--accept-package-agreements"},
		"winget upgrade --all に失敗: %w",
		func(count int) string {
			return fmt.Sprintf("%d 件の winget パッケージを更新しました", count)
		},
	)
}

// parseUpgradeOutput は "winget upgrade --include-unknown" の出力をパースします。
//
// 出力形式（固定幅テーブル）:
//
//	Name                                   ID                               Version              Available            Source
//	-----------------------------------------------------------------------------------------------------------------------
//	Docker Desktop                         Docker.DockerDesktop             4.59.0               4.60.0              winget
//	GitHub CLI                             GitHub.cli                       2.83.2               2.86.0              winget
//	9 upgrades available.
//
// ヘッダー行のダッシュ区切り行でカラム位置を検出し、各行をパースします。
func (w *WingetUpdater) parseUpgradeOutput(output string) []PackageInfo {
	lines := splitLines(output)

	// ダッシュ区切り行を検出してカラム位置を特定
	_, separatorIdx := findTableHeader(lines)
	if separatorIdx < 0 {
		return nil
	}

	// セパレータ行の直前のデータ行からカラム位置を算出
	// セパレータ行は ASCII のみのためバイト位置が正確に使える
	// ただし winget のセパレータ行は1本の連続ダッシュなので、
	// データ行のパターンから位置を検出する必要がある
	// → separatorIdx の2行後（最初のデータ行）からカラム位置を推定するか、
	//   ヘッダー行の代わりにデータ行のパターンを使う
	// 最も信頼性の高い方法: データ行を走査し、ID カラム（ドット区切り）の位置を検出
	colPositions := detectWingetColumnsFromData(lines, separatorIdx+1)
	if colPositions == nil {
		return nil
	}

	idStart := colPositions.idStart
	verStart := colPositions.versionStart
	availStart := colPositions.availableStart

	packages := make([]PackageInfo, 0)

	for i := separatorIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}

		// サマリー行のスキップ（数字で始まり "upgrade" や "アップグレード" を含む）
		if isSummaryLine(line) {
			continue
		}

		// 行の長さが ID 位置に満たない場合はスキップ
		if len(line) < idStart {
			continue
		}

		name := strings.TrimSpace(safeSubstring(line, 0, idStart))
		id := strings.TrimSpace(safeSubstring(line, idStart, verStart))
		currentVersion := strings.TrimSpace(safeSubstring(line, verStart, availStart))
		newVersion := strings.TrimSpace(safeSubstringToEnd(line, availStart))

		// Source カラムがある場合は除去（newVersion から末尾の "winget" 等を除去）
		if spaceIdx := strings.LastIndex(newVersion, " "); spaceIdx > 0 {
			potentialSource := strings.TrimSpace(newVersion[spaceIdx:])
			if isSourceName(potentialSource) {
				newVersion = strings.TrimSpace(newVersion[:spaceIdx])
			}
		}

		if id == "" {
			continue
		}

		packages = append(packages, PackageInfo{
			Name:           name,
			CurrentVersion: currentVersion,
			NewVersion:     newVersion,
		})
	}

	return packages
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

// findTableHeader はダッシュ区切り行を検出し、ヘッダー行とセパレータ行のインデックスを返します。
func findTableHeader(lines []string) (headerIdx, separatorIdx int) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// ダッシュのみで構成された行を検出（最低10文字）
		if len(trimmed) >= 10 && isAllDashes(trimmed) {
			if i > 0 {
				return i - 1, i
			}

			return -1, i
		}
	}

	return -1, -1
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

// wingetColumnPositions は winget 出力のカラム位置を保持します。
type wingetColumnPositions struct {
	idStart        int
	versionStart   int
	availableStart int
}

// detectWingetColumnsFromData はデータ行を走査して各カラムの開始位置を推定します。
// winget の ID カラムはドット区切り（例: Docker.DockerDesktop）のため、
// 複数行のデータから共通する位置パターンを検出します。
func detectWingetColumnsFromData(lines []string, startIdx int) *wingetColumnPositions {
	// データ行から ID（ドット区切り文字列）を探す
	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		if line == "" || isSummaryLine(line) {
			continue
		}

		// ID パターン（英数字.英数字）を行内で探索
		pos := findIDColumnPosition(line)
		if pos == nil {
			continue
		}

		return pos
	}

	return nil
}

// findIDColumnPosition は1行からカラム位置を検出します。
// winget の出力では各カラムが複数スペースで区切られています。
// ID カラムの特徴（ドット区切り）を利用してカラム境界を検出します。
func findIDColumnPosition(line string) *wingetColumnPositions {
	// スペースで区切られた「ワード群」を右から走査し、
	// "winget" / "msstore" (Source)、バージョン（Available）、バージョン（Version）、ID の順を検出
	fields := splitFieldsWithPositions(line)
	if len(fields) < 4 {
		return nil
	}

	// 末尾から Source, Available, Version, ID を検出
	// 末尾が Source 名なら除外
	endIdx := len(fields)
	if isSourceName(fields[endIdx-1].value) {
		endIdx--
	}

	if endIdx < 4 {
		return nil
	}

	// 末尾から: Available (endIdx-1), Version (endIdx-2)
	// IDはドット区切りを含むフィールド
	availField := fields[endIdx-1]
	verField := fields[endIdx-2]

	// ID フィールド: Version の左隣でドットを含むもの
	idField := fields[endIdx-3]

	if !strings.Contains(idField.value, ".") {
		return nil
	}

	return &wingetColumnPositions{
		idStart:        idField.start,
		versionStart:   verField.start,
		availableStart: availField.start,
	}
}

// fieldWithPosition はフィールドとその開始位置を保持します。
type fieldWithPosition struct {
	value string
	start int
}

// splitFieldsWithPositions は文字列をスペース区切りでフィールドに分割し、各フィールドの開始位置を返します。
func splitFieldsWithPositions(s string) []fieldWithPosition {
	var fields []fieldWithPosition

	i := 0
	for i < len(s) {
		// スペースをスキップ
		for i < len(s) && s[i] == ' ' {
			i++
		}

		if i >= len(s) {
			break
		}

		// フィールドの開始
		start := i
		for i < len(s) && s[i] != ' ' {
			i++
		}

		fields = append(fields, fieldWithPosition{
			value: s[start:i],
			start: start,
		})
	}

	return fields
}

// detectColumnPositions はヘッダー行からカラムの開始位置を検出します。
// "Name   ID   Version   Available   Source" のような行を解析し、
// 各カラムの開始位置（バイト位置）を返します。
func detectColumnPositions(header string) []int {
	positions := []int{0}
	inSpace := false

	for i, r := range header {
		if unicode.IsSpace(r) {
			inSpace = true
		} else if inSpace {
			positions = append(positions, i)
			inSpace = false
		}
	}

	return positions
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

// isSourceName はソース名（winget, msstore 等）かどうかを判定します。
func isSourceName(s string) bool {
	lower := strings.ToLower(s)
	return lower == "winget" || lower == "msstore"
}

// インターフェース準拠の確認（推奨）
var _ Updater = (*WingetUpdater)(nil)
