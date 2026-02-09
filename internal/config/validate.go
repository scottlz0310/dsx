package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ValidateOptions は Validate の追加オプションです。
type ValidateOptions struct {
	// KnownSysManagers を指定すると、sys.enable の未知マネージャを警告します。
	// nil/空の場合はチェックしません。
	KnownSysManagers map[string]struct{}
}

// ValidationIssue は設定検証で見つかった問題（エラー/警告）です。
type ValidationIssue struct {
	Field   string
	Message string
}

func (i ValidationIssue) String() string {
	field := strings.TrimSpace(i.Field)
	if field == "" {
		return strings.TrimSpace(i.Message)
	}

	return fmt.Sprintf("%s: %s", field, strings.TrimSpace(i.Message))
}

// ValidationResult は設定検証結果です。
type ValidationResult struct {
	Errors   []ValidationIssue
	Warnings []ValidationIssue
}

// Validate は設定内容を検証し、問題点（エラー/警告）を返します。
func Validate(cfg *Config, opts ValidateOptions) ValidationResult {
	var result ValidationResult

	if cfg == nil {
		result.Errors = append(result.Errors, ValidationIssue{Field: "config", Message: "設定が nil です"})
		return result
	}

	if cfg.Version != 1 {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "version",
			Message: fmt.Sprintf("未対応のバージョンです: %d（対応: 1）", cfg.Version),
		})
	}

	validateControl(&result, cfg)
	validateRepo(&result, cfg)
	validateSecrets(&result, cfg)
	validateSys(&result, cfg, opts)

	return result
}

func validateControl(result *ValidationResult, cfg *Config) {
	if cfg.Control.Concurrency <= 0 {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "control.concurrency",
			Message: "1以上を指定してください",
		})
	}

	timeout := strings.TrimSpace(cfg.Control.Timeout)
	if timeout == "" {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "control.timeout",
			Message: "空です（例: \"10m\"）",
		})

		return
	}

	parsed, err := time.ParseDuration(timeout)
	if err != nil {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "control.timeout",
			Message: fmt.Sprintf("不正な期間です: %q（例: \"10m\"）", timeout),
		})

		return
	}

	if parsed <= 0 {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "control.timeout",
			Message: fmt.Sprintf("0より大きい値を指定してください: %q", timeout),
		})
	}
}

func validateRepo(result *ValidationResult, cfg *Config) {
	root := strings.TrimSpace(cfg.Repo.Root)
	if root == "" {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "repo.root",
			Message: "空です（例: \"~/src\" ではなくフルパスで指定してください）",
		})

		return
	}

	if strings.HasPrefix(root, "~") {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "repo.root",
			Message: fmt.Sprintf("チルダ（~）は自動展開されません: %q（フルパスで指定してください）", root),
		})

		return
	}

	cleaned := filepath.Clean(root)

	info, err := os.Stat(cleaned)
	switch {
	case err == nil:
		if !info.IsDir() {
			result.Errors = append(result.Errors, ValidationIssue{
				Field:   "repo.root",
				Message: fmt.Sprintf("ディレクトリではありません: %s", cleaned),
			})
		}
	case os.IsNotExist(err):
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "repo.root",
			Message: fmt.Sprintf("ディレクトリが存在しません: %s（必要なら作成するか、`devsync config init` を再実行してください）", cleaned),
		})
	default:
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "repo.root",
			Message: fmt.Sprintf("ディレクトリの確認に失敗しました: %s: %v", cleaned, err),
		})
	}

	protocol := strings.ToLower(strings.TrimSpace(cfg.Repo.GitHub.Protocol))
	switch protocol {
	case "https", "ssh":
		// ok
	case "":
		result.Warnings = append(result.Warnings, ValidationIssue{
			Field:   "repo.github.protocol",
			Message: "空です（既定値: https）",
		})
	default:
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "repo.github.protocol",
			Message: fmt.Sprintf("不正な値です: %q（https または ssh を指定してください）", cfg.Repo.GitHub.Protocol),
		})
	}

	allowedTargets := map[string]struct{}{
		"merged":   {},
		"squashed": {},
	}

	for _, target := range cfg.Repo.Cleanup.Target {
		trimmed := strings.ToLower(strings.TrimSpace(target))
		if trimmed == "" {
			continue
		}

		if _, ok := allowedTargets[trimmed]; ok {
			continue
		}

		result.Warnings = append(result.Warnings, ValidationIssue{
			Field:   "repo.cleanup.target",
			Message: fmt.Sprintf("未知の対象が含まれています（cleanup実装時に影響する可能性があります）: %q", target),
		})
	}
}

func validateSecrets(result *ValidationResult, cfg *Config) {
	if !cfg.Secrets.Enabled {
		return
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.Secrets.Provider))
	if provider == "" {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "secrets.provider",
			Message: "空です（例: bitwarden）",
		})

		return
	}

	if provider != "bitwarden" {
		result.Errors = append(result.Errors, ValidationIssue{
			Field:   "secrets.provider",
			Message: fmt.Sprintf("未対応のプロバイダです: %q（対応: bitwarden）", cfg.Secrets.Provider),
		})
	}
}

func validateSys(result *ValidationResult, cfg *Config, opts ValidateOptions) {
	if len(opts.KnownSysManagers) == 0 || len(cfg.Sys.Enable) == 0 {
		return
	}

	unknownSet := make(map[string]struct{}, 2)
	duplicateSet := make(map[string]struct{}, 1)
	seen := make(map[string]struct{}, len(cfg.Sys.Enable))

	for _, name := range cfg.Sys.Enable {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Field:   "sys.enable",
				Message: "空のマネージャ名が含まれています",
			})

			continue
		}

		if _, ok := seen[trimmed]; ok {
			duplicateSet[trimmed] = struct{}{}
		} else {
			seen[trimmed] = struct{}{}
		}

		if _, ok := opts.KnownSysManagers[trimmed]; !ok {
			unknownSet[trimmed] = struct{}{}
		}
	}

	duplicates := keysOfStringSet(duplicateSet)
	sort.Strings(duplicates)

	unknown := keysOfStringSet(unknownSet)
	sort.Strings(unknown)

	if len(duplicates) > 0 {
		result.Warnings = append(result.Warnings, ValidationIssue{
			Field:   "sys.enable",
			Message: fmt.Sprintf("重複指定されています: %v", duplicates),
		})
	}

	if len(unknown) > 0 {
		result.Warnings = append(result.Warnings, ValidationIssue{
			Field:   "sys.enable",
			Message: fmt.Sprintf("未知のマネージャが指定されています（typoの可能性があります）: %v", unknown),
		})
	}
}

func keysOfStringSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}

	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}

	return keys
}
