package updater

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
)

// CargoUpdater は cargo (Rust パッケージ) の実装です。
type CargoUpdater struct {
	snapshotRun snapshotCommandRunner
	// disableGit が true の場合、cargo-install-update へ -g を渡しません（既定は false = Git パッケージ更新を有効化）。
	// Go のゼロ値でも既定の「Git パッケージ更新有効」を保つため反転フラグで保持します。
	disableGit bool
	// locked が true の場合、cargo-install-update へ --locked を渡します（既定は false）。
	locked bool
	// listRunning は dir にある names の実行ファイルで動作中のプロセスを返します。
	// nil のときは実際のプロセスを調べます（テスト用に差し替え可能）。
	listRunning func(dir string, names []string) ([]string, error)
}

// minCargoUpdateLockedVersion は cargo-install-update の --locked フラグに対応する最低バージョンです。
const (
	minCargoUpdateLockedVersion = "14.1.0"
	minCargoUpdateLockedMajor   = 14
	minCargoUpdateLockedMinor   = 1
)

type cargoUpdateOptions struct {
	includeGit bool
	locked     bool
}

type cargoUpdateBinaryInfo struct {
	isLegacyCLI    bool
	version        string
	versionKnown   bool
	supportsLocked bool
}

// 起動時にレジストリに登録
func init() {
	Register(&CargoUpdater{})
}

func (c *CargoUpdater) Name() string {
	return config.SysManagerCargo
}

func (c *CargoUpdater) DisplayName() string {
	return "cargo (Rust パッケージ)"
}

func (c *CargoUpdater) IsAvailable() bool {
	_, err := exec.LookPath("cargo")
	return err == nil
}

func (c *CargoUpdater) includeGit() bool {
	return !c.disableGit
}

func (c *CargoUpdater) Configure(cfg config.ManagerConfig) error {
	// シングルトン再利用時に前回の設定が残らないよう毎回既定値へリセットする
	c.disableGit = false
	c.locked = false

	if cfg == nil {
		return nil
	}

	if rawGit, ok := cfg[config.CargoConfigKeyGit]; ok {
		gitEnabled, ok := rawGit.(bool)
		if !ok {
			return fmt.Errorf(
				"sys.managers.%s.%s は真偽値（true/false）で指定してください（指定値: %T）",
				config.SysManagerCargo,
				config.CargoConfigKeyGit,
				rawGit,
			)
		}

		c.disableGit = !gitEnabled
	}

	if rawLocked, ok := cfg[config.CargoConfigKeyLocked]; ok {
		lockedEnabled, ok := rawLocked.(bool)
		if !ok {
			return fmt.Errorf(
				"sys.managers.%s.%s は真偽値（true/false）で指定してください（指定値: %T）",
				config.SysManagerCargo,
				config.CargoConfigKeyLocked,
				rawLocked,
			)
		}

		c.locked = lockedEnabled
	}

	return nil
}

func (c *CargoUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// cargo install --list でインストール済みパッケージを取得
	cmd := exec.CommandContext(ctx, "cargo", "install", "--list")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cargo install --list の実行に失敗: %w", err)
	}

	packages := c.parseInstallList(string(output))

	// cargo は個別の outdated チェックがないため、
	// AvailableUpdates は 0 とし、インストール済みパッケージのみ返す
	// 実際の更新可否は update 実行時に判定される
	return &CheckResult{
		AvailableUpdates: 0,
		Packages:         packages,
		Message:          fmt.Sprintf("%d 件のインストール済みパッケージを確認（更新可否は実行時に判定）", len(packages)),
	}, nil
}

func (c *CargoUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	if opts.DryRun {
		checkResult, err := c.Check(ctx)
		if err != nil {
			return nil, err
		}

		if len(checkResult.Packages) == 0 {
			result.Message = "cargo でインストールされたパッケージがありません"
			return result, nil
		}

		result.Message = fmt.Sprintf(
			"%d 件のインストール済みパッケージについて更新を確認します（%s）",
			len(checkResult.Packages),
			c.dryRunModeSuffix(),
		)
		result.InstalledPackages = checkResult.Packages

		return result, nil
	}

	// cargo-update がなければ自動インストール、フルパスを取得
	updateBin, err := c.ensureCargoUpdate(ctx)
	if err != nil {
		return result, err
	}

	// フルパスで直接実行（PATH に ~/.cargo/bin がない環境でも動作）
	var buf bytes.Buffer

	args, err := cargoUpdateArgs(updateBin, cargoUpdateOptions{
		includeGit: c.includeGit(),
		locked:     c.locked,
	})
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, err
	}

	cmd := exec.CommandContext(ctx, updateBin, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// crates.io 側が失敗し Git 側のみ成功した場合など、非ゼロ終了前に出力された成功件数を引き継ぐ
		result.UpdatedCount = c.parseUpdateCount(buf.String())
		result.Errors = append(result.Errors, err)

		cmdDesc := formatCargoUpdateCommand(args)
		if running := c.runningCargoBinProcesses(); len(running) > 0 {
			return result, fmt.Errorf(
				"%s に失敗（実行中の cargo バイナリがあるため置き換えに失敗した可能性があります: %s）: %w",
				cmdDesc,
				strings.Join(running, ", "),
				err,
			)
		}

		return result, fmt.Errorf("%s に失敗: %w", cmdDesc, err)
	}

	result.UpdatedCount = c.parseUpdateCount(buf.String())

	if result.UpdatedCount > 0 {
		result.Message = fmt.Sprintf("%d 件のパッケージを更新しました", result.UpdatedCount)
	} else {
		result.Message = "cargo パッケージを確認しました（更新なし）"
	}

	return result, nil
}

func (c *CargoUpdater) dryRunModeSuffix() string {
	var tags []string
	if c.includeGit() {
		tags = append(tags, "Git パッケージ含む")
	}

	if c.locked {
		tags = append(tags, "--locked")
	}

	if len(tags) == 0 {
		return "DryRunモード"
	}

	return strings.Join(tags, "・") + " / DryRunモード"
}

// ensureCargoUpdate は cargo-update がなければ自動インストールし、
// cargo-install-update のフルパスを返します。
func (c *CargoUpdater) ensureCargoUpdate(ctx context.Context) (string, error) {
	if path, err := exec.LookPath("cargo-install-update"); err == nil {
		return path, nil
	}

	fmt.Println("ℹ️  cargo-update が見つかりません。自動インストールします...")

	cmd := exec.CommandContext(ctx, "cargo", "install", "cargo-update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cargo-update のインストールに失敗: %w", err)
	}

	fmt.Println("✅ cargo-update のインストールが完了しました。")

	// PATH への反映を再確認
	if path, err := exec.LookPath("cargo-install-update"); err == nil {
		return path, nil
	}

	// PATH に反映されていない場合は CARGO_HOME/bin を直接確認
	path, err := cargoInstallUpdateBinPath()
	if err != nil {
		return "", fmt.Errorf(
			"cargo-update のインストールは成功しましたが、cargo-install-update が PATH に見つかりません。"+
				"~/.cargo/bin を PATH に追加してから再実行してください: %w", err)
	}

	return path, nil
}

// cargoUpdateArgs はインストール済みの cargo-update バージョンと設定に応じた実行引数を返します。
// v19-: --version が "cargo-install-update N.N.N" を返す → ["-a", ...]（直接引数形式）
// v20+: --version が "cargo N.N.N" を返す、または検出失敗 → ["install-update", "-a", ...]（サブコマンド形式）
// locked が有効かつ検出バージョンが v14.1.0 未満の場合は、更新前にアップグレード案内エラーを返します。
func cargoUpdateArgs(bin string, opts cargoUpdateOptions) ([]string, error) {
	info := inspectCargoUpdateBinary(bin)

	if opts.locked && info.versionKnown && !info.supportsLocked {
		return nil, fmt.Errorf(
			"sys.managers.%s.%s には cargo-update v%s 以上が必要です（検出バージョン: v%s）。`cargo install cargo-update` でアップグレードしてから再実行してください",
			config.SysManagerCargo,
			config.CargoConfigKeyLocked,
			minCargoUpdateLockedVersion,
			info.version,
		)
	}

	var args []string
	if info.isLegacyCLI {
		args = []string{"-a"}
	} else {
		args = []string{"install-update", "-a"}
	}

	if opts.includeGit {
		args = append(args, "-g")
	}

	if opts.locked {
		args = append(args, "--locked")
	}

	return args, nil
}

func inspectCargoUpdateBinary(bin string) cargoUpdateBinaryInfo {
	out, err := exec.CommandContext(context.Background(), bin, "--version").Output()
	if err != nil {
		return cargoUpdateBinaryInfo{supportsLocked: true}
	}

	return parseCargoUpdateVersionOutput(string(out))
}

func parseCargoUpdateVersionOutput(output string) cargoUpdateBinaryInfo {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return cargoUpdateBinaryInfo{supportsLocked: true}
	}

	info := cargoUpdateBinaryInfo{
		isLegacyCLI:    fields[0] == "cargo-install-update",
		supportsLocked: true,
	}

	if len(fields) >= 2 {
		if major, minor, ok := parseCargoUpdateSemverMajorMinor(fields[1]); ok {
			info.versionKnown = true
			info.version = strings.TrimPrefix(fields[1], "v")
			info.supportsLocked = major > minCargoUpdateLockedMajor ||
				(major == minCargoUpdateLockedMajor && minor >= minCargoUpdateLockedMinor)
		}
	}

	return info
}

func parseCargoUpdateSemverMajorMinor(raw string) (major, minor int, ok bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed = trimmed[:idx]
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}

	maj, err := strconv.Atoi(parts[0])
	if err != nil || maj < 0 {
		return 0, 0, false
	}

	minVer, err := strconv.Atoi(parts[1])
	if err != nil || minVer < 0 {
		return 0, 0, false
	}

	return maj, minVer, true
}

func formatCargoUpdateCommand(args []string) string {
	if len(args) > 0 && args[0] == "install-update" {
		return "cargo " + strings.Join(args, " ")
	}

	return "cargo install-update " + strings.Join(args, " ")
}

func cargoBinDir() (string, error) {
	cargoHome := os.Getenv("CARGO_HOME")
	if cargoHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		cargoHome = filepath.Join(home, ".cargo")
	}

	return filepath.Join(cargoHome, "bin"), nil
}

// cargoInstallUpdateBinPath は CARGO_HOME/bin の cargo-install-update のパスを返します。
// PATH に ~/.cargo/bin が含まれていない環境での自動インストール後のフォールバックに使用します。
func cargoInstallUpdateBinPath() (string, error) {
	binDir, err := cargoBinDir()
	if err != nil {
		return "", err
	}

	// 実行可能ファイルの候補（Windows では .exe/.cmd/.bat も確認）
	candidates := []string{"cargo-install-update"}
	if runtime.GOOS == windowsOS {
		candidates = []string{
			"cargo-install-update.exe",
			"cargo-install-update.cmd",
			"cargo-install-update.bat",
		}
	}

	for _, name := range candidates {
		path := filepath.Join(binDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("%s に cargo-install-update が見つかりません", binDir)
}

// runningCargoBinProcesses は更新失敗時の補足用として、$CARGO_HOME/bin 配下で実行中の .exe プロセスを返します。
// 失敗原因の補足専用のため、確認に失敗した場合は空スライスを返し元の更新エラーを優先します。
func (c *CargoUpdater) runningCargoBinProcesses() []string {
	binDir, err := cargoBinDir()
	if err != nil {
		return nil
	}

	exeNames, err := listCargoExecutableNames(binDir)
	if err != nil || len(exeNames) == 0 {
		return nil
	}

	list := c.listRunning
	if list == nil {
		list = listRunningExecutables
	}

	running, err := list(binDir, exeNames)
	if err != nil {
		return nil
	}

	return running
}

func listCargoExecutableNames(binDir string) ([]string, error) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.EqualFold(filepath.Ext(name), ".exe") {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names, nil
}

// parseUpdateCount は "cargo install-update" の出力から実際に更新されたパッケージ数を返します。
// crates.io と Git パッケージが混在すると "Updated X package(s)." / "Updated Y git package(s)." の後に
// "Overall updated N package(s)..." が出力されるため、Overall updated を最優先し、
// 見つからない場合（v19-）のみ各グループの Updated 行を合算します。
func (c *CargoUpdater) parseUpdateCount(output string) int {
	output = strings.ReplaceAll(output, "\r\n", "\n")

	var (
		hasOverall   bool
		overallCount int
		groupTotal   int
	)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		// v20+: "Overall updated N package(s)." / "Overall updated N packages: pkg1, pkg2."
		if n, ok := parseCargoOverallUpdatedLine(line); ok {
			hasOverall = true
			overallCount = n

			continue
		}

		// v19- またはグループ別集計: "Updated N package(s)." / "Updated N git package(s)."
		if n, ok := parseCargoGroupUpdatedLine(line); ok {
			groupTotal += n
		}
	}

	if hasOverall {
		return overallCount
	}

	return groupTotal
}

func parseCargoOverallUpdatedLine(line string) (int, bool) {
	if !strings.HasPrefix(line, "Overall updated ") {
		return 0, false
	}

	parts := strings.Fields(line)
	if len(parts) < 4 || !strings.HasPrefix(parts[3], "package") {
		return 0, false
	}

	n, err := strconv.Atoi(parts[2])
	if err != nil || n < 0 {
		return 0, false
	}

	return n, true
}

func parseCargoGroupUpdatedLine(line string) (int, bool) {
	if !strings.HasPrefix(line, "Updated ") {
		return 0, false
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return 0, false
	}

	isRegular := strings.HasPrefix(parts[2], "package")
	isGit := len(parts) >= 4 && parts[2] == "git" && strings.HasPrefix(parts[3], "package")

	if !isRegular && !isGit {
		return 0, false
	}

	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return 0, false
	}

	return n, true
}

// parseInstallList は "cargo install --list" の出力をパースします
// 形式:
// package-name v1.0.0:
//
//	binary1
//	binary2
//
// another-package v2.0.0:
//
//	binary3
func (c *CargoUpdater) parseInstallList(output string) []PackageInfo {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		// インデントされた行（バイナリ名）をスキップ
		if strings.HasPrefix(line, "    ") || line == "" {
			continue
		}

		// "package-name v1.0.0:" 形式をパース
		// コロンで終わることを確認
		if !strings.HasSuffix(line, ":") {
			continue
		}

		line = strings.TrimSuffix(line, ":")

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		version := strings.TrimPrefix(parts[1], "v")

		pkg := PackageInfo{
			Name:           name,
			CurrentVersion: version,
			NewVersion:     "", // cargo は事前に新バージョンを知る手段がない
		}
		packages = append(packages, pkg)
	}

	return packages
}
