package updater

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
)

// ScoopUpdater は Scoop パッケージマネージャの実装です。
type ScoopUpdater struct{}

// 起動時にレジストリに登録
func init() {
	Register(&ScoopUpdater{})
}

func (s *ScoopUpdater) Name() string {
	return "scoop"
}

func (s *ScoopUpdater) DisplayName() string {
	return "Scoop (Windows)"
}

func (s *ScoopUpdater) IsAvailable() bool {
	_, err := exec.LookPath("scoop")
	return err == nil
}

func (s *ScoopUpdater) Configure(cfg config.ManagerConfig) error {
	return nil
}

func (s *ScoopUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// まず scoop のバケット情報を更新
	updateCmd := exec.CommandContext(ctx, "scoop", "update")

	if output, err := updateCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scoop update の実行に失敗: %w", buildCommandOutputErr(err, output))
	}

	// scoop status で更新可能なパッケージを確認
	statusCmd := exec.CommandContext(ctx, "scoop", "status")

	output, err := statusCmd.CombinedOutput()
	if err != nil {
		// scoop status は更新可能なパッケージがあっても exit code 0 以外を返すことがある
		if len(output) == 0 {
			return nil, fmt.Errorf("scoop status の実行に失敗: %w", buildCommandOutputErr(err, output))
		}
	}

	packages := s.parseStatusOutput(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (s *ScoopUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := s.Check(ctx)
	if err != nil {
		return nil, err
	}

	return runCountBasedUpdate(
		ctx, opts, checkResult,
		"すべての Scoop パッケージは最新です",
		func(count int) string {
			return fmt.Sprintf("%d 件の Scoop パッケージが更新可能です（DryRunモード）", count)
		},
		"scoop",
		[]string{"update", "--all"},
		"scoop update --all に失敗: %w",
		func(count int) string {
			return fmt.Sprintf("%d 件の Scoop パッケージを更新しました", count)
		},
	)
}

// parseStatusOutput は "scoop status" の出力をパースします。
//
// 出力形式:
//
//	Name              Installed Version   Latest Version   Missing Dependencies   Info
//	----              -----------------   --------------   --------------------   ----
//	git               2.34.1              2.38.0
//	nodejs            16.13.0             18.9.0
//
// "Everything is ok!" のメッセージが含まれる場合は更新なしと判定します。
func (s *ScoopUpdater) parseStatusOutput(output string) []PackageInfo {
	lines := strings.Split(output, "\n")

	// "Everything is ok!" が含まれる場合は更新なし
	for _, line := range lines {
		if strings.Contains(line, "Everything is ok!") {
			return nil
		}
	}

	// ダッシュ区切り行を検出
	headerIdx, separatorIdx := findScoopTableHeader(lines)
	if separatorIdx < 0 || headerIdx < 0 {
		return nil
	}

	// ヘッダー行からカラム位置を算出
	colPositions := detectScoopColumnPositions(lines[headerIdx])
	if len(colPositions) < 3 {
		return nil
	}

	nameStart := colPositions[0]
	installedStart := colPositions[1]
	latestStart := colPositions[2]

	packages := make([]PackageInfo, 0)

	for i := separatorIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// 情報メッセージのスキップ
		if strings.HasPrefix(trimmed, "Scoop ") || strings.HasPrefix(trimmed, "WARN") || strings.HasPrefix(trimmed, "INFO") {
			continue
		}

		name := strings.TrimSpace(safeSubstring(line, nameStart, installedStart))
		currentVersion := strings.TrimSpace(safeSubstring(line, installedStart, latestStart))
		newVersion := strings.TrimSpace(safeSubstringToEnd(line, latestStart))

		// 追加カラム（Missing Dependencies, Info）がある場合は除去
		// スペース区切りの最初のフィールドのみをバージョンとして扱う
		if spaceIdx := strings.Index(newVersion, "  "); spaceIdx > 0 {
			newVersion = strings.TrimSpace(newVersion[:spaceIdx])
		}

		if name == "" {
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

// findScoopTableHeader は scoop status 出力のテーブルヘッダーを検出します。
func findScoopTableHeader(lines []string) (headerIdx, separatorIdx int) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// ダッシュとスペースで構成された行を検出（scoop は "----  -----" 形式）
		if len(trimmed) >= 4 && isScoopSeparator(trimmed) {
			if i > 0 {
				return i - 1, i
			}

			return -1, i
		}
	}

	return -1, -1
}

// isScoopSeparator は scoop のセパレータ行を判定します。
// ダッシュとスペースのみで構成されている行を対象とします。
func isScoopSeparator(s string) bool {
	hasDash := false

	for _, r := range s {
		switch r {
		case '-':
			hasDash = true
		case ' ':
			// スペースは許容
		default:
			return false
		}
	}

	return hasDash
}

// detectScoopColumnPositions は scoop のヘッダー行からカラム位置を検出します。
// scoop は複数スペースでカラムを区切るため、2つ以上のスペースを区切りとして扱います。
func detectScoopColumnPositions(header string) []int {
	positions := []int{}

	i := 0
	for i < len(header) {
		// スペースをスキップ
		if header[i] == ' ' {
			i++

			continue
		}

		// 非スペース文字の開始位置を記録
		positions = append(positions, i)

		// 次のスペースまでスキップ
		for i < len(header) && header[i] != ' ' {
			i++
		}

		// 連続スペースをスキップ
		for i < len(header) && header[i] == ' ' {
			i++
		}
	}

	return positions
}

// インターフェース準拠の確認（推奨）
var _ Updater = (*ScoopUpdater)(nil)
