package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// FwupdmgrUpdater は fwupdmgr (Linux Firmware 更新) の実装です。
type FwupdmgrUpdater struct{}

// 起動時にレジストリへ登録します。
func init() {
	Register(&FwupdmgrUpdater{})
}

func (f *FwupdmgrUpdater) Name() string {
	return "fwupdmgr"
}

func (f *FwupdmgrUpdater) DisplayName() string {
	return "fwupdmgr (Linux Firmware)"
}

func (f *FwupdmgrUpdater) IsAvailable() bool {
	_, err := exec.LookPath("fwupdmgr")
	return err == nil
}

func (f *FwupdmgrUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (f *FwupdmgrUpdater) Check(ctx context.Context) (*CheckResult, error) {
	cmd := exec.CommandContext(ctx, "fwupdmgr", "get-updates", "--json")

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		stderrOutput := stderr.String()
		if isNoFwupdmgrUpdatesMessage(stderrOutput) || isNoFwupdmgrUpdatesMessage(string(output)) {
			return &CheckResult{
				AvailableUpdates: 0,
				Packages:         []PackageInfo{},
			}, nil
		}

		return nil, fmt.Errorf(
			"fwupdmgr get-updates の実行に失敗: %w",
			buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	packages, parseErr := f.parseGetUpdatesJSON(output)
	if parseErr != nil {
		return nil, fmt.Errorf(
			"fwupdmgr get-updates の出力解析に失敗: %w",
			buildCommandOutputErr(parseErr, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (f *FwupdmgrUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := f.Check(ctx)
	if err != nil {
		return nil, err
	}

	if checkResult.AvailableUpdates == 0 {
		return &UpdateResult{Message: "適用可能なファームウェア更新はありません"}, nil
	}

	if opts.DryRun {
		return &UpdateResult{
			Message:  fmt.Sprintf("%d 件のファームウェア更新が適用可能です（DryRunモード）", checkResult.AvailableUpdates),
			Packages: checkResult.Packages,
		}, nil
	}

	return f.runUpdateCommand(ctx, checkResult)
}

func (f *FwupdmgrUpdater) runUpdateCommand(ctx context.Context, checkResult *CheckResult) (*UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "fwupdmgr", "update", "-y")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	result := &UpdateResult{
		Packages: checkResult.Packages,
	}

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("fwupdmgr update に失敗: %w", err)
	}

	result.UpdatedCount = len(checkResult.Packages)
	result.Message = fmt.Sprintf("%d 件のファームウェア更新を実行しました", result.UpdatedCount)

	return result, nil
}

func (f *FwupdmgrUpdater) parseGetUpdatesJSON(output []byte) ([]PackageInfo, error) {
	if len(output) == 0 {
		return nil, errors.New("fwupdmgr get-updates の出力が空です")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("JSON の解析に失敗: %w", err)
	}

	rawDevices, ok := lookupMapValueIgnoreCase(payload, "devices")
	if !ok {
		return nil, errors.New("devices キーが見つかりません")
	}

	devices, ok := rawDevices.([]interface{})
	if !ok {
		return nil, errors.New("devices の型が不正です")
	}

	packages := make([]PackageInfo, 0, len(devices))

	for _, raw := range devices {
		device, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		releases := lookupMapSliceIgnoreCase(device, "releases")
		if len(releases) == 0 {
			continue
		}

		name := strings.TrimSpace(lookupMapStringIgnoreCase(device, "name", "deviceName", "guid"))
		if name == "" {
			continue
		}

		newVersion := strings.TrimSpace(extractReleaseVersion(releases[0]))
		if newVersion == "" {
			continue
		}

		packages = append(packages, PackageInfo{
			Name:           name,
			CurrentVersion: strings.TrimSpace(lookupMapStringIgnoreCase(device, "currentVersion", "version")),
			NewVersion:     newVersion,
		})
	}

	return packages, nil
}

func isNoFwupdmgrUpdatesMessage(output string) bool {
	normalized := strings.ToLower(output)

	switch {
	case strings.Contains(normalized, "no updatable devices"):
		return true
	case strings.Contains(normalized, "no updates available"):
		return true
	case strings.Contains(normalized, "no upgrades for"):
		return true
	default:
		return false
	}
}

func lookupMapValueIgnoreCase(src map[string]interface{}, key string) (interface{}, bool) {
	for k, v := range src {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}

	return nil, false
}

func lookupMapStringIgnoreCase(src map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := lookupMapValueIgnoreCase(src, key)
		if !ok {
			continue
		}

		value, ok := raw.(string)
		if !ok {
			continue
		}

		if strings.TrimSpace(value) == "" {
			continue
		}

		return value
	}

	return ""
}

func lookupMapSliceIgnoreCase(src map[string]interface{}, key string) []interface{} {
	raw, ok := lookupMapValueIgnoreCase(src, key)
	if !ok {
		return nil
	}

	value, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	return value
}

func extractReleaseVersion(raw interface{}) string {
	release, ok := raw.(map[string]interface{})
	if !ok {
		return ""
	}

	return lookupMapStringIgnoreCase(release, "version")
}
