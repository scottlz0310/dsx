package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/scottlz0310/devsync/internal/config"
)

// SnapUpdater は snap (Ubuntu Snap パッケージ) の実装です。
type SnapUpdater struct {
	useSudo bool
}

// 起動時にレジストリに登録
func init() {
	Register(&SnapUpdater{useSudo: true})
}

func (s *SnapUpdater) Name() string {
	return "snap"
}

func (s *SnapUpdater) DisplayName() string {
	return "snap (Ubuntu Snap パッケージ)"
}

func (s *SnapUpdater) IsAvailable() bool {
	_, err := exec.LookPath("snap")
	if err != nil {
		return false
	}

	// snapd が利用できない環境では snap コマンドがハング/失敗しやすいため除外する。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "snap", "version")

	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return !isSnapdUnavailable(string(output))
}

func (s *SnapUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}

	if useSudo, ok := cfg["use_sudo"].(bool); ok {
		s.useSudo = useSudo
		return nil
	}

	// 旧キー `sudo` との後方互換
	if useSudo, ok := cfg["sudo"].(bool); ok {
		s.useSudo = useSudo
	}

	return nil
}

func (s *SnapUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// snap refresh --list で更新可能なスナップを取得
	// LANG=C でロケールを英語に固定
	cmd := exec.CommandContext(ctx, "snap", "refresh", "--list")

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	output, err := cmd.Output()

	// 更新がない場合は特定のメッセージが返る
	if err != nil {
		// exit code が 0 でない場合もあるが、output を確認
		outputStr := string(output)
		if strings.Contains(outputStr, "All snaps up to date") {
			return &CheckResult{
				AvailableUpdates: 0,
				Packages:         []PackageInfo{},
			}, nil
		}
		// パース可能な出力があれば続行、そうでなければエラー
		if len(output) > 0 {
			packages := s.parseRefreshList(string(output))
			if len(packages) > 0 {
				return &CheckResult{
					AvailableUpdates: len(packages),
					Packages:         packages,
				}, nil
			}
		}

		return nil, fmt.Errorf("snap refresh --list の実行に失敗: %w", err)
	}

	packages := s.parseRefreshList(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (s *SnapUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := s.Check(ctx)
	if err != nil {
		return nil, err
	}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "すべてのスナップは最新です"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件のスナップが更新可能です（DryRunモード）", checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages

		return result, nil
	}

	// 実際の更新を実行
	if err := s.runCommand(ctx); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("snap refresh に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のスナップを更新しました", result.UpdatedCount)

	return result, nil
}

// runCommand は snap refresh コマンドを実行します（必要に応じて sudo を使用）
func (s *SnapUpdater) runCommand(ctx context.Context) error {
	var cmd *exec.Cmd
	if s.useSudo {
		cmd = exec.CommandContext(ctx, "sudo", "snap", "refresh")
	} else {
		cmd = exec.CommandContext(ctx, "snap", "refresh")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// parseRefreshList は "snap refresh --list" の出力をパースします
// 形式:
// Name     Version    Rev   Size   Publisher   Notes
// package  1.0.0      123   10MB   publisher   -
func (s *SnapUpdater) parseRefreshList(output string) []PackageInfo {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	// ヘッダー行をスキップ（最初の行）
	headerSkipped := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !headerSkipped {
			// ヘッダー行をスキップ
			if strings.HasPrefix(line, "Name") {
				headerSkipped = true
			}

			continue
		}

		// データ行をパース
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pkg := PackageInfo{
			Name:           fields[0],
			NewVersion:     fields[1],
			CurrentVersion: "", // snap refresh --list では現在のバージョンは表示されない
		}
		packages = append(packages, pkg)
	}

	return packages
}

func isSnapdUnavailable(output string) bool {
	normalized := strings.ToLower(output)

	for _, line := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "snapd") && strings.Contains(trimmed, "unavailable") {
			return true
		}
	}

	return false
}
