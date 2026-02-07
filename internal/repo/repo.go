// Package repo はローカル Git リポジトリの検出と状態取得を提供します。
package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Status はリポジトリ状態を表します。
type Status string

const (
	StatusClean      Status = "clean"
	StatusDirty      Status = "dirty"
	StatusUnpushed   Status = "unpushed"
	StatusNoUpstream Status = "no_upstream"
)

// Info は単一リポジトリの情報です。
type Info struct {
	Name        string
	Path        string
	Status      Status
	Dirty       bool
	Ahead       int
	HasUpstream bool
}

// Discover は root 配下（root 自体を含む）で Git リポジトリを検出します。
// 現時点では root 直下のディレクトリを対象とします。
func Discover(root string) ([]string, error) {
	resolvedRoot, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}

	if validateErr := validateRoot(resolvedRoot); validateErr != nil {
		return nil, validateErr
	}

	result := make([]string, 0)

	if hasGitMetadata(resolvedRoot) {
		result = append(result, resolvedRoot)
	}

	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("ルートディレクトリの読み取りに失敗: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		candidate := filepath.Join(resolvedRoot, entry.Name())
		if hasGitMetadata(candidate) {
			result = append(result, candidate)
		}
	}

	sort.Strings(result)

	return result, nil
}

// List は root 配下のリポジトリ一覧と状態を取得します。
func List(ctx context.Context, root string) ([]Info, error) {
	paths, err := Discover(root)
	if err != nil {
		return nil, err
	}

	infos := make([]Info, 0, len(paths))

	for _, path := range paths {
		info, inspectErr := Inspect(ctx, path)
		if inspectErr != nil {
			return nil, inspectErr
		}

		infos = append(infos, info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// Inspect は単一リポジトリの状態を取得します。
func Inspect(ctx context.Context, repoPath string) (Info, error) {
	cleanPath := filepath.Clean(repoPath)

	dirty, err := isDirty(ctx, cleanPath)
	if err != nil {
		return Info{}, fmt.Errorf("%s の状態取得に失敗: %w", cleanPath, err)
	}

	hasUpstream, ahead, err := getAheadCount(ctx, cleanPath)
	if err != nil {
		return Info{}, fmt.Errorf("%s の追跡状態取得に失敗: %w", cleanPath, err)
	}

	status := classifyStatus(dirty, hasUpstream, ahead)

	return Info{
		Name:        filepath.Base(cleanPath),
		Path:        cleanPath,
		Status:      status,
		Dirty:       dirty,
		Ahead:       ahead,
		HasUpstream: hasUpstream,
	}, nil
}

// StatusLabel は状態を日本語ラベルに変換します。
func StatusLabel(status Status) string {
	switch status {
	case StatusClean:
		return "クリーン"
	case StatusDirty:
		return "ダーティ"
	case StatusUnpushed:
		return "未プッシュ"
	case StatusNoUpstream:
		return "追跡なし"
	default:
		return "不明"
	}
}

func resolveRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("repo.root が空です")
	}

	resolved := root
	if strings.HasPrefix(resolved, "~/") || resolved == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
		}

		if resolved == "~" {
			resolved = home
		} else {
			resolved = filepath.Join(home, strings.TrimPrefix(resolved, "~/"))
		}
	}

	return filepath.Clean(resolved), nil
}

func validateRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("ルートディレクトリにアクセスできません: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("ルートパスがディレクトリではありません: %s", root)
	}

	return nil
}

func hasGitMetadata(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}

	if info.IsDir() {
		return true
	}

	if !info.Mode().IsRegular() {
		return false
	}

	content, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir:") {
		return false
	}

	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))

	return gitDir != ""
}

func isDirty(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) != "", nil
}

func getAheadCount(ctx context.Context, repoPath string) (hasUpstream bool, ahead int, err error) {
	upstreamCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if _, cmdErr := upstreamCmd.Output(); cmdErr != nil {
		if isNoUpstreamError(cmdErr) {
			return false, 0, nil
		}

		return false, 0, cmdErr
	}

	aheadCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", "@{u}..HEAD")

	output, err := aheadCmd.Output()
	if err != nil {
		return true, 0, err
	}

	ahead, err = strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return true, 0, fmt.Errorf("ahead 件数のパースに失敗: %w", err)
	}

	return true, ahead, nil
}

func isNoUpstreamError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	stderr := strings.ToLower(string(exitErr.Stderr))

	return strings.Contains(stderr, "no upstream configured") ||
		strings.Contains(stderr, "no upstream branch")
}

func classifyStatus(dirty, hasUpstream bool, ahead int) Status {
	if dirty {
		return StatusDirty
	}

	if !hasUpstream {
		return StatusNoUpstream
	}

	if ahead > 0 {
		return StatusUnpushed
	}

	return StatusClean
}
