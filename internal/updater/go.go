package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/selfupdate"
)

// goTargetsEmptyMessage は go.targets が未設定の際に Check / Update が返すメッセージです。
// 単一の定数で管理することで文言の不整合を防ぎます。
const goTargetsEmptyMessage = "Go updater の targets は未設定です。\n" +
	"$GOBIN / $GOPATH/bin に既存の Go バイナリがある場合は `dsx sys discover` で go.targets 候補を確認できます。"

// GoUpdater は Go ツール (go install) の更新を管理します。
// go install コマンドでインストールしたバイナリを最新版に更新します。
type GoUpdater struct {
	// targets は更新対象のパッケージパス一覧
	// 例: ["golang.org/x/tools/gopls@latest", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"]
	targets []string

	discoverGoBinaries  func(context.Context) (*DiscoverResult, error)
	latestModuleVersion func(context.Context, string) (string, error)
	installGoTarget     func(context.Context, string) error
	selfUpdateCheck     func(context.Context, string) (*selfupdate.Info, error)
	currentVersion      string
}

// 起動時にレジストリに登録
func init() {
	Register(&GoUpdater{})
}

func (g *GoUpdater) Name() string {
	return "go"
}

func (g *GoUpdater) DisplayName() string {
	return "Go ツール (go install)"
}

func (g *GoUpdater) IsAvailable() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

func (g *GoUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}

	// targets の設定を読み込む
	if targets, ok := cfg["targets"]; ok {
		switch v := targets.(type) {
		case []interface{}:
			g.targets = make([]string, 0, len(v))

			for _, item := range v {
				if s, ok := item.(string); ok {
					g.targets = append(g.targets, s)
				}
			}
		case []string:
			g.targets = v
		}
	}

	return nil
}

func (g *GoUpdater) Check(ctx context.Context) (*CheckResult, error) {
	if len(g.targets) == 0 {
		return &CheckResult{
			Message: goTargetsEmptyMessage,
		}, nil
	}

	plan, err := g.planGoTargets(ctx, g.currentVersion)
	if err != nil {
		return nil, err
	}

	packages := planPackages(plan.installTargets())

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
		Message:          plan.checkMessage(),
	}, nil
}

func (g *GoUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	if len(g.targets) == 0 {
		result.Message = goTargetsEmptyMessage
		return result, nil
	}

	currentVersion := strings.TrimSpace(opts.CurrentVersion)
	if currentVersion == "" {
		currentVersion = g.currentVersion
	}

	plan, err := g.planGoTargets(ctx, currentVersion)
	if err != nil {
		return nil, err
	}

	if opts.DryRun {
		result.Packages = planPackages(plan.installTargets())
		result.Message = plan.dryRunMessage()

		return result, nil
	}

	// 各ツールを順番に更新
	for _, target := range plan.installTargets() {
		toolName := target.ToolName
		pkg := target.InstallTarget

		fmt.Printf("  📦 %s をインストール中...\n", toolName)

		if err := g.runGoInstall(ctx, pkg); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", toolName, err))

			continue
		}

		result.UpdatedCount++
		result.Packages = append(result.Packages, target.PackageInfo())
	}

	if result.FailedCount > 0 {
		result.Message = fmt.Sprintf("%d 件更新、%d 件失敗", result.UpdatedCount, result.FailedCount)
	} else {
		result.Message = plan.updateMessage(result.UpdatedCount)
	}

	return result, nil
}

// ConfigureRuntimeVersion は実行中 dsx のバージョンを Go updater に渡します。
func (g *GoUpdater) ConfigureRuntimeVersion(currentVersion string) {
	g.currentVersion = currentVersion
}

type goTargetAction string

const (
	goTargetInstallUpdate  goTargetAction = "install_update"
	goTargetInstallUnknown goTargetAction = "install_unknown"
	goTargetInstallPinned  goTargetAction = "install_pinned"
	goTargetSkipLatest     goTargetAction = "skip_latest"
	goTargetSkipSelf       goTargetAction = "skip_self"
)

type parsedGoTarget struct {
	Raw           string
	PackagePath   string
	Version       string
	InstallTarget string
	CompareLatest bool
}

type goTargetDecision struct {
	RawTarget      string
	PackagePath    string
	InstallTarget  string
	ToolName       string
	CurrentVersion string
	LatestVersion  string
	Reason         string
	Action         goTargetAction
}

type goUpdatePlan struct {
	Decisions        []*goTargetDecision
	DiscoveryWarning string
}

type goListModuleJSON struct {
	Version string `json:"Version"`
}

type latestModuleResult struct {
	Version string
	Err     error
}

func (g *GoUpdater) planGoTargets(ctx context.Context, currentVersion string) (*goUpdatePlan, error) {
	targets := make([]parsedGoTarget, 0, len(g.targets))
	for _, rawTarget := range g.targets {
		target, err := parseGoTarget(rawTarget)
		if err != nil {
			return nil, err
		}

		targets = append(targets, target)
	}

	discovered, discoveryWarning, err := g.discoverInstalledGoBinaries(ctx)
	if err != nil {
		return nil, err
	}

	installedByPackage := buildGoBinaryMap(discovered)
	latestCache := make(map[string]latestModuleResult)
	plan := &goUpdatePlan{DiscoveryWarning: discoveryWarning}

	for _, target := range targets {
		decision, err := g.planGoTarget(ctx, target, installedByPackage, latestCache, currentVersion)
		if err != nil {
			return nil, err
		}

		plan.Decisions = append(plan.Decisions, decision)
	}

	return plan, nil
}

func (g *GoUpdater) planGoTarget(ctx context.Context, target parsedGoTarget, installedByPackage map[string]*GoBinaryInfo, latestCache map[string]latestModuleResult, currentVersion string) (*goTargetDecision, error) {
	if isDSXSelfPackage(target.PackagePath) {
		return g.planDSXSelfTarget(ctx, target, currentVersion)
	}

	if !target.CompareLatest {
		return newPinnedDecision(target), nil
	}

	info, ok := installedByPackage[target.PackagePath]
	if !ok {
		return newUnknownDecision(target, "対応するバイナリが見つかりません"), nil
	}

	return g.planLatestGoTarget(ctx, target, info, latestCache), nil
}

func (g *GoUpdater) planLatestGoTarget(ctx context.Context, target parsedGoTarget, info *GoBinaryInfo, latestCache map[string]latestModuleResult) *goTargetDecision {
	if info.ModulePath == "" {
		return newUnknownDecision(target, "ModulePath が空です")
	}

	if info.InstalledVersion == "" || info.InstalledVersion == selfupdate.DevelVersion {
		return newUnknownDecision(target, "InstalledVersion が比較不能です")
	}

	latest := g.cachedLatestModuleVersion(ctx, info.ModulePath, latestCache)
	if latest.Err != nil {
		return newUnknownDecision(target, fmt.Sprintf("latest version の取得に失敗しました: %v", latest.Err))
	}

	if latest.Version == "" {
		return newUnknownDecision(target, "latest version が空です")
	}

	if info.InstalledVersion == latest.Version {
		return newLatestSkipDecision(target, info, latest.Version)
	}

	return newUpdateDecision(target, info, latest.Version)
}

func (g *GoUpdater) cachedLatestModuleVersion(ctx context.Context, modulePath string, latestCache map[string]latestModuleResult) latestModuleResult {
	latest, ok := latestCache[modulePath]
	if ok {
		return latest
	}

	version, latestErr := g.getLatestModuleVersion(ctx, modulePath)
	latest = latestModuleResult{Version: version, Err: latestErr}
	latestCache[modulePath] = latest

	return latest
}

func (g *GoUpdater) discoverInstalledGoBinaries(ctx context.Context) (*DiscoverResult, string, error) {
	discover := g.discoverGoBinaries
	if discover == nil {
		discover = DiscoverGoBinaries
	}

	result, err := discover(ctx)
	if err == nil {
		return result, "", nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, "", err
	}

	return &DiscoverResult{}, fmt.Sprintf("Go バイナリの検出に失敗したため、dsx 本体を除く @latest target は判定不能として扱います: %v", err), nil
}

func buildGoBinaryMap(result *DiscoverResult) map[string]*GoBinaryInfo {
	installed := make(map[string]*GoBinaryInfo)
	if result == nil {
		return installed
	}

	for i := range result.Detected {
		info := &result.Detected[i]
		if info.PackagePath == "" {
			continue
		}

		installed[info.PackagePath] = info
	}

	return installed
}

func parseGoTarget(raw string) (parsedGoTarget, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return parsedGoTarget{}, fmt.Errorf("go.targets に空の target が含まれています")
	}

	target := parsedGoTarget{
		Raw:           trimmed,
		PackagePath:   trimmed,
		InstallTarget: trimmed + latestVersionSuffix,
		CompareLatest: true,
	}

	if idx := strings.LastIndex(trimmed, "@"); idx >= 0 {
		if idx == 0 || idx == len(trimmed)-1 {
			return parsedGoTarget{}, fmt.Errorf("go.targets に不正な target が含まれています: %q", trimmed)
		}

		target.PackagePath = trimmed[:idx]
		target.Version = trimmed[idx+1:]
		target.InstallTarget = trimmed
		target.CompareLatest = target.Version == "latest"
	}

	return target, nil
}

func isDSXSelfPackage(packagePath string) bool {
	return packagePath == selfupdate.GoInstallPackage
}

func (g *GoUpdater) planDSXSelfTarget(ctx context.Context, target parsedGoTarget, currentVersion string) (*goTargetDecision, error) {
	decision := &goTargetDecision{
		RawTarget:     target.Raw,
		PackagePath:   target.PackagePath,
		InstallTarget: target.InstallTarget,
		ToolName:      extractToolName(target.PackagePath),
		Action:        goTargetSkipSelf,
		Reason:        "dsx 本体は Go updater では更新しません",
	}

	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" {
		decision.Reason = "dsx 本体の更新有無を判定できないためスキップしました。必要に応じて `dsx self-update --check` を実行してください"
		return decision, nil
	}

	check := g.selfUpdateCheck
	if check == nil {
		check = func(ctx context.Context, currentVersion string) (*selfupdate.Info, error) {
			return selfupdate.CheckAvailable(ctx, currentVersion, nil)
		}
	}

	info, err := check(ctx, currentVersion)
	if err != nil {
		decision.Reason = fmt.Sprintf("dsx 本体の更新有無を判定できないためスキップしました。必要に応じて `dsx self-update --check` を実行してください: %v", err)
		return decision, nil
	}

	if info == nil {
		decision.CurrentVersion = currentVersion
		decision.Reason = "dsx 本体は最新版、または更新対象外のためスキップしました"
		return decision, nil
	}

	return decision, fmt.Errorf(
		"go updater の対象に dsx 本体が含まれています。dsx 本体は Go updater では更新できません。`dsx self-update` を実行してください: 現在 %s / 最新 %s",
		info.CurrentVersion,
		info.LatestVersion,
	)
}

func newPinnedDecision(target parsedGoTarget) *goTargetDecision {
	return &goTargetDecision{
		RawTarget:     target.Raw,
		PackagePath:   target.PackagePath,
		InstallTarget: target.InstallTarget,
		ToolName:      extractToolName(target.PackagePath),
		LatestVersion: target.Version,
		Reason:        "固定バージョン target のため従来通り実行します",
		Action:        goTargetInstallPinned,
	}
}

func newUnknownDecision(target parsedGoTarget, reason string) *goTargetDecision {
	return &goTargetDecision{
		RawTarget:     target.Raw,
		PackagePath:   target.PackagePath,
		InstallTarget: target.InstallTarget,
		ToolName:      extractToolName(target.PackagePath),
		LatestVersion: latestVersionSuffix,
		Reason:        reason,
		Action:        goTargetInstallUnknown,
	}
}

func newLatestSkipDecision(target parsedGoTarget, info *GoBinaryInfo, latestVersion string) *goTargetDecision {
	return &goTargetDecision{
		RawTarget:      target.Raw,
		PackagePath:    target.PackagePath,
		InstallTarget:  target.InstallTarget,
		ToolName:       extractToolName(target.PackagePath),
		CurrentVersion: info.InstalledVersion,
		LatestVersion:  latestVersion,
		Reason:         "最新版です",
		Action:         goTargetSkipLatest,
	}
}

func newUpdateDecision(target parsedGoTarget, info *GoBinaryInfo, latestVersion string) *goTargetDecision {
	return &goTargetDecision{
		RawTarget:      target.Raw,
		PackagePath:    target.PackagePath,
		InstallTarget:  target.InstallTarget,
		ToolName:       extractToolName(target.PackagePath),
		CurrentVersion: info.InstalledVersion,
		LatestVersion:  latestVersion,
		Reason:         "更新があります",
		Action:         goTargetInstallUpdate,
	}
}

func (g *GoUpdater) getLatestModuleVersion(ctx context.Context, modulePath string) (string, error) {
	run := g.latestModuleVersion
	if run != nil {
		return run(ctx, modulePath)
	}

	output, err := runCommandOutputWithLocaleC(
		ctx,
		"go",
		[]string{"list", "-m", "-json", modulePath + latestVersionSuffix},
		"go list -m -json の実行に失敗: %w",
	)
	if err != nil {
		return "", err
	}

	return parseGoListModuleVersion(output)
}

func parseGoListModuleVersion(output []byte) (string, error) {
	var payload goListModuleJSON
	if err := json.Unmarshal(output, &payload); err != nil {
		return "", fmt.Errorf(
			"go list -m -json の解析に失敗: %w",
			buildCommandOutputErr(err, output),
		)
	}

	return strings.TrimSpace(payload.Version), nil
}

func (g *GoUpdater) runGoInstall(ctx context.Context, target string) error {
	run := g.installGoTarget
	if run != nil {
		return run(ctx, target)
	}

	cmd := exec.CommandContext(ctx, "go", "install", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return cmd.Run()
}

func (p *goUpdatePlan) installTargets() []*goTargetDecision {
	targets := make([]*goTargetDecision, 0, len(p.Decisions))

	for i := range p.Decisions {
		decision := p.Decisions[i]
		if decision.shouldInstall() {
			targets = append(targets, decision)
		}
	}

	return targets
}

func (p *goUpdatePlan) count(action goTargetAction) int {
	count := 0
	for i := range p.Decisions {
		decision := p.Decisions[i]
		if decision.Action == action {
			count++
		}
	}

	return count
}

func (p *goUpdatePlan) skippedCount() int {
	return p.count(goTargetSkipLatest) + p.count(goTargetSkipSelf)
}

func (p *goUpdatePlan) unknownOrPinnedCount() int {
	return p.count(goTargetInstallUnknown) + p.count(goTargetInstallPinned)
}

func (p *goUpdatePlan) checkMessage() string {
	installCount := len(p.installTargets())
	if installCount == 0 {
		return p.noInstallMessage()
	}

	return p.summaryMessage(fmt.Sprintf("更新予定: %d 件", installCount))
}

func (p *goUpdatePlan) dryRunMessage() string {
	installCount := len(p.installTargets())
	if installCount == 0 {
		return p.noInstallMessage() + "（DryRunモード）"
	}

	return p.summaryMessage(fmt.Sprintf("更新予定: %d 件", installCount)) + "（DryRunモード）"
}

func (p *goUpdatePlan) updateMessage(updatedCount int) string {
	if updatedCount == 0 && len(p.installTargets()) == 0 {
		return p.noInstallMessage()
	}

	return p.summaryMessage(fmt.Sprintf("更新: %d 件", updatedCount))
}

func (p *goUpdatePlan) noInstallMessage() string {
	if p.skippedCount() > 0 {
		message := fmt.Sprintf("Go ツールの更新対象はありません（スキップ: %d 件）", p.skippedCount())
		if p.DiscoveryWarning != "" {
			message += "\n" + p.DiscoveryWarning
		}

		return message
	}

	if p.DiscoveryWarning != "" {
		return p.DiscoveryWarning
	}

	return "Go ツールの更新対象はありません"
}

func (p *goUpdatePlan) summaryMessage(prefix string) string {
	message := fmt.Sprintf(
		"%s / スキップ: %d 件 / 判定不能または固定バージョン: %d 件",
		prefix,
		p.skippedCount(),
		p.unknownOrPinnedCount(),
	)
	if p.DiscoveryWarning != "" {
		message += "\n" + p.DiscoveryWarning
	}

	return message
}

func (d *goTargetDecision) shouldInstall() bool {
	return d.Action == goTargetInstallUpdate || d.Action == goTargetInstallUnknown || d.Action == goTargetInstallPinned
}

func (d *goTargetDecision) PackageInfo() PackageInfo {
	return PackageInfo{
		Name:           d.ToolName,
		CurrentVersion: d.CurrentVersion,
		NewVersion:     d.displayNewVersion(),
	}
}

func (d *goTargetDecision) displayNewVersion() string {
	if d.LatestVersion != "" {
		return d.LatestVersion
	}

	if strings.HasSuffix(d.InstallTarget, latestVersionSuffix) {
		return latestVersionSuffix
	}

	return d.InstallTarget
}

func planPackages(decisions []*goTargetDecision) []PackageInfo {
	packages := make([]PackageInfo, 0, len(decisions))
	for i := range decisions {
		decision := decisions[i]
		packages = append(packages, decision.PackageInfo())
	}

	return packages
}

// extractToolName はパッケージパスからツール名を抽出します
// 例: "github.com/golangci/golangci-lint/cmd/golangci-lint@latest" -> "golangci-lint"
func extractToolName(pkg string) string {
	// @version を除去
	if idx := strings.Index(pkg, "@"); idx != -1 {
		pkg = pkg[:idx]
	}

	// 最後のパスセグメントを取得
	parts := strings.Split(pkg, "/")

	return parts[len(parts)-1]
}

// DefaultGoTargets はよく使われるGoツールのデフォルトリストを返します。
// 設定ファイルで targets が未指定の場合の参考として使用できます。
func DefaultGoTargets() []string {
	return []string{
		"golang.org/x/tools/gopls@latest",
		"github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
		"github.com/go-delve/delve/cmd/dlv@latest",
		"github.com/fatih/gomodifytags@latest",
		"github.com/cweill/gotests/gotests@latest",
		"github.com/josharian/impl@latest",
	}
}

// ListInstalledGoTools は $GOPATH/bin または $GOBIN にインストールされたツールを一覧表示します。
func ListInstalledGoTools() ([]string, error) {
	// GOBIN を優先、なければ GOPATH/bin
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}

			gopath = home + "/go"
		}

		gobin = gopath + "/bin"
	}

	entries, err := os.ReadDir(gobin)
	if err != nil {
		return nil, fmt.Errorf("$GOBIN (%s) の読み取りに失敗: %w", gobin, err)
	}

	var tools []string

	for _, entry := range entries {
		if !entry.IsDir() {
			tools = append(tools, entry.Name())
		}
	}

	return tools, nil
}

// GoBinaryInfo は "go version -m <binary>" の出力から取得したバイナリ情報を保持します。
type GoBinaryInfo struct {
	BinaryPath       string // バイナリの絶対パス
	BinaryName       string // filepath.Base(BinaryPath)、.exe を含む（trim しない）
	PackagePath      string // `path` 行 → go install に使う
	ModulePath       string // `mod` 行フィールド2（モジュールルート）
	InstalledVersion string // `mod` 行フィールド3
	GoVersion        string // ParseGoBinaryInfo では設定しない（将来タスクで対応予定）
}

// UpdateTarget は go install に渡すパスを返す（PackagePath + "@latest"）。
// フィールドではなくメソッドにすることで PackagePath との不整合を防ぐ。
// レシーバが nil の場合は空文字を返す。
func (i *GoBinaryInfo) UpdateTarget() string {
	if i == nil || i.PackagePath == "" {
		return ""
	}

	return i.PackagePath + "@latest"
}

// DiscoverResult は DiscoverGoBinaries / DiscoverGoBinariesInDir の結果を保持します。
type DiscoverResult struct {
	Detected []GoBinaryInfo
	Skipped  []SkippedBinary
}

// SkippedBinary はスキャン中にスキップされたバイナリの情報を保持します。
type SkippedBinary struct {
	Name   string
	Reason string // "Go モジュール情報なし" / "バックアップファイル"
}

// runGoVersionM は "go version -m <binaryPath>" を実行して出力を返します。
func runGoVersionM(ctx context.Context, binaryPath string) ([]byte, error) {
	return runCommandOutputWithLocaleC(ctx, "go", []string{"version", "-m", binaryPath}, "go version -m の実行に失敗: %w")
}

// discoverInDir は binDir 内のバイナリをスキャンし、DiscoverResult を返します。
// runCmd はテスト時に fakeRunCmd を注入できる関数パラメータです。
func discoverInDir(ctx context.Context, binDir string,
	runCmd func(context.Context, string) ([]byte, error)) (*DiscoverResult, error) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, fmt.Errorf("%s の読み取りに失敗: %w", binDir, err)
	}

	result := &DiscoverResult{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// *~ / *.exe~ パターン（バックアップファイル）を除外
		if strings.HasSuffix(name, "~") {
			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "バックアップファイル",
			})

			continue
		}

		fullPath := filepath.Join(binDir, name)

		output, err := runCmd(ctx, fullPath)
		if err != nil {
			// コンテキストキャンセル・期限切れの場合はスキャンを中断する
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}

			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "Go モジュール情報なし",
			})

			continue
		}

		info, err := ParseGoBinaryInfo(fullPath, string(output))
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "Go モジュール情報なし",
			})

			continue
		}

		result.Detected = append(result.Detected, *info)
	}

	return result, nil
}

// DiscoverGoBinariesInDir は指定ディレクトリ内の Go バイナリ情報を収集します。
func DiscoverGoBinariesInDir(ctx context.Context, binDir string) (*DiscoverResult, error) {
	return discoverInDir(ctx, binDir, runGoVersionM)
}

// DiscoverGoBinaries は $GOBIN → $GOPATH/bin → ~/go/bin の順でパスを解決し、
// Go バイナリ情報を収集します。
func DiscoverGoBinaries(ctx context.Context) (*DiscoverResult, error) {
	binDir := os.Getenv("GOBIN")
	if binDir == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
			}

			gopath = filepath.Join(home, "go")
		}

		// GOPATH は OS によってはパスリスト（Unix では ":" 区切り、Windows では ";" 区切り）に
		// なり得るため、filepath.SplitList で先頭エントリを取得してから "bin" を連結する。
		// 先頭エントリが空の場合（区切り文字のみの GOPATH 等）は ~/go にフォールバックする。
		gopathEntries := filepath.SplitList(gopath)
		firstEntry := ""

		if len(gopathEntries) > 0 {
			firstEntry = gopathEntries[0]
		}

		if firstEntry == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
			}

			firstEntry = filepath.Join(home, "go")
		}

		binDir = filepath.Join(firstEntry, "bin")
	}

	return DiscoverGoBinariesInDir(ctx, binDir)
}

// ParseGoBinaryInfo は "go version -m <binary>" の出力を解析し、GoBinaryInfo を返します。
// path 行が存在しない場合は nil と error を返します。
func ParseGoBinaryInfo(binaryPath, output string) (*GoBinaryInfo, error) {
	info := &GoBinaryInfo{
		BinaryPath: binaryPath,
		BinaryName: filepath.Base(binaryPath),
	}

	foundPath := false
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)

		if len(fields) >= 2 && fields[0] == "path" {
			info.PackagePath = fields[1]
			foundPath = true
		}

		if len(fields) >= 3 && fields[0] == "mod" {
			info.ModulePath = fields[1]
			info.InstalledVersion = fields[2]
		}
	}

	if !foundPath {
		return nil, fmt.Errorf("go version -m の出力に path 行が見つかりませんでした: %s", binaryPath)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("go version -m 出力のスキャン中にエラーが発生しました: %w", err)
	}

	return info, nil
}
