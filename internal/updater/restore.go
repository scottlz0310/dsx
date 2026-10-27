package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"

	"github.com/scottlz0310/dsx/internal/restore"
	"github.com/scottlz0310/dsx/internal/snapshot"
)

const (
	restoreInstallCommand = "install"
	restoreCargoManager   = "cargo"
	restoreWingetManager  = "winget"
)

type restoreHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// restoreQueryError は原因を保持しつつ、URL やプロキシの認証情報を表示しません。
type restoreQueryError struct {
	message string
	cause   error
}

func (e *restoreQueryError) Error() string { return e.message }
func (e *restoreQueryError) Unwrap() error { return e.cause }

type restoreResolver struct {
	run    snapshotCommandRunner
	client restoreHTTPClient
	goos   string
	goarch string
}

var restoreSemver = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[A-Za-z0-9.-]+)?(\+[A-Za-z0-9.-]+)?$`)
var restorePackageVersion = regexp.MustCompile(`^\d[A-Za-z0-9.!+_-]*$`)

// RestoreManagers は初期8マネージャの読み取り専用の照会処理を接続します。
func RestoreManagers(updaters []Updater) map[string]restore.Manager {
	resolver := &restoreResolver{run: runSnapshotCommand, client: http.DefaultClient, goos: runtime.GOOS, goarch: runtime.GOARCH}

	managers := make(map[string]restore.Manager)
	for _, updater := range updaters {
		name := updater.Name()
		switch name {
		case "go", "npm", "pnpm", "pipx", "uv", restoreCargoManager, "gem", restoreWingetManager:
		default:
			continue
		}

		provider, ok := updater.(SnapshotProvider)
		if !ok {
			continue
		}

		managers[name] = restore.Manager{
			Available:  updater.IsAvailable,
			Compatible: func(goos string) bool { return name != restoreWingetManager || goos == windowsOS },
			Installed:  provider.Snapshot,
			Resolve: func(ctx context.Context, tool snapshot.Tool) (restore.Command, error) {
				return resolver.resolve(ctx, name, tool)
			},
		}
	}

	return managers
}

func (r *restoreResolver) resolve(ctx context.Context, manager string, tool snapshot.Tool) (restore.Command, error) {
	if _, err := snapshot.Normalize(manager, []snapshot.Tool{tool}); err != nil {
		return restore.Command{}, err
	}

	if err := validateRestoreVersion(manager, tool.Version); err != nil {
		return restore.Command{}, err
	}

	switch manager {
	case "go":
		return r.resolveGo(ctx, tool)
	case "npm", "pnpm":
		return r.resolveNode(ctx, manager, tool)
	case "pipx", "uv":
		return r.resolvePython(ctx, manager, tool)
	case restoreCargoManager:
		return r.resolveCargo(ctx, tool)
	case "gem":
		return r.resolveGem(ctx, tool)
	case restoreWingetManager:
		return r.resolveWinget(ctx, tool)
	default:
		return restore.Command{}, fmt.Errorf("復元計画に未対応です")
	}
}

func validateRestoreVersion(manager, version string) error {
	if version == "" || manager == restoreWingetManager {
		return nil
	}

	valid := restorePackageVersion.MatchString(version)
	if manager == "go" || manager == "npm" || manager == "pnpm" || manager == restoreCargoManager {
		valid = restoreSemver.MatchString(version)
	}

	if manager == "go" && !strings.HasPrefix(version, "v") {
		valid = false
	}

	if !valid {
		return fmt.Errorf("範囲やタグではなく、正確なバージョンを指定してください")
	}

	return nil
}

func (r *restoreResolver) resolveNode(ctx context.Context, manager string, tool snapshot.Tool) (restore.Command, error) {
	requestedVersion := tool.Version
	if requestedVersion == "" {
		requestedVersion = "latest"
	}

	data, err := r.run(ctx, manager, "view", tool.Package+"@"+requestedVersion, "version", "os", "cpu", "--json")
	if err != nil {
		return restore.Command{}, err
	}

	metadata, parseErr := parseNodeRestoreMetadata(data)
	if parseErr != nil {
		return restore.Command{}, parseErr
	}

	if !r.nodePlatformMatches(metadata.OS, metadata.CPU) {
		return restore.Command{}, fmt.Errorf("パッケージの OS または CPU 条件が現在の環境と一致しません")
	}

	if err := checkResolvedVersion(manager, tool.Version, metadata.Version); err != nil {
		return restore.Command{}, err
	}

	return restore.Command{Args: []string{manager, restoreInstallCommand, "--global", tool.Package + "@" + metadata.Version}, Version: metadata.Version}, nil
}

type nodeRestoreMetadata struct {
	Version string   `json:"version"`
	OS      []string `json:"os"`
	CPU     []string `json:"cpu"`
}

func parseNodeRestoreMetadata(data []byte) (nodeRestoreMetadata, error) {
	output := bytes.TrimSpace(data)
	if len(output) > 0 && output[0] == '[' {
		// npm の複数フィールド照会は、単一版でも配列として返されます。
		var records []json.RawMessage
		if err := json.Unmarshal(output, &records); err != nil || len(records) != 1 {
			return nodeRestoreMetadata{}, fmt.Errorf("提供版は単一の版である必要があります")
		}

		output = bytes.TrimSpace(records[0])
	}

	var (
		metadata nodeRestoreMetadata
		parseErr error
	)
	if len(output) > 0 && output[0] == '"' {
		parseErr = json.Unmarshal(output, &metadata.Version)
	} else {
		parseErr = json.Unmarshal(output, &metadata)
	}

	if parseErr != nil || metadata.Version == "" {
		return nodeRestoreMetadata{}, fmt.Errorf("提供版の JSON が不正です")
	}

	return metadata, nil
}

func (r *restoreResolver) nodePlatformMatches(osConditions, cpuConditions []string) bool {
	goos := r.goos
	if goos == windowsOS {
		goos = "win32"
	}

	goarch := r.goarch
	switch goarch {
	case "amd64":
		goarch = "x64"
	case "386":
		goarch = "ia32"
	}

	return nodeRestorePlatformMatches(osConditions, goos) && nodeRestorePlatformMatches(cpuConditions, goarch)
}

func nodeRestorePlatformMatches(conditions []string, current string) bool {
	allowed := len(conditions) == 0

	hasPositive := false
	for _, condition := range conditions {
		if condition == "!"+current {
			return false
		}

		if strings.HasPrefix(condition, "!") {
			continue
		}

		hasPositive = true
		allowed = allowed || condition == current || condition == "any"
	}

	return allowed || !hasPositive
}

func (r *restoreResolver) resolveWinget(ctx context.Context, tool snapshot.Tool) (restore.Command, error) {
	args := []string{"show", "--id", tool.Package, "--exact", "--disable-interactivity", "--accept-source-agreements"}

	install := []string{restoreWingetManager, restoreInstallCommand, "--id", tool.Package, "--exact"}
	if tool.Version != "" {
		args = append(args, "--version", tool.Version)
		install = append(install, "--version", tool.Version)
	}

	if tool.Source != "" {
		args = append(args, "--source", tool.Source)
		install = append(install, "--source", tool.Source)
	}

	if _, err := r.run(ctx, restoreWingetManager, args...); err != nil {
		return restore.Command{}, err
	}

	return restore.Command{Args: install, Version: tool.Version}, nil
}

func checkResolvedVersion(manager, requested, resolved string) error {
	if resolved == "" {
		return fmt.Errorf("提供版を取得できませんでした")
	}

	if err := validateRestoreVersion(manager, resolved); err != nil {
		return err
	}

	if requested != "" && strings.TrimPrefix(requested, "v") != strings.TrimPrefix(resolved, "v") {
		return fmt.Errorf("指定版と提供版が一致しません（別バージョンへの置き換えは行いません）")
	}

	return nil
}

// getMetadata は公開レジストリのメタデータだけを読み、応答本文や認証情報をエラーへ含めません。
func (r *restoreResolver) getMetadata(ctx context.Context, address string) (data []byte, found bool, err error) {
	req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if requestErr != nil {
		return nil, false, &restoreQueryError{message: "メタデータの要求を作成できません", cause: requestErr}
	}

	req.Header.Set("User-Agent", "dsx/restore-plan (https://github.com/scottlz0310/dsx)")

	response, fetchErr := r.client.Do(req)
	if fetchErr != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, &restoreQueryError{message: "メタデータの取得に失敗しました", cause: fetchErr}
	}

	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			err = errors.Join(err, &restoreQueryError{message: "メタデータの応答を閉じる処理に失敗しました", cause: closeErr})
		}
	}()

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return nil, false, nil
	}

	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("メタデータの取得に失敗（HTTP %d）", response.StatusCode)
	}

	const maxMetadata = 16 << 20

	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxMetadata+1))
	if readErr != nil {
		return nil, false, &restoreQueryError{message: "メタデータの読み取りに失敗しました", cause: readErr}
	}

	if len(data) > maxMetadata {
		return nil, false, fmt.Errorf("メタデータが上限の16 MiBを超えています")
	}

	return data, true, nil
}

func metadataAddress(base, name, requestedVersion, suffix string) string {
	address := base + url.PathEscape(name)
	if requestedVersion != "" {
		address += "/" + url.PathEscape(requestedVersion)
	}

	return address + suffix
}
