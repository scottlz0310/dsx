package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	knownManagers := map[string]struct{}{
		"apt":  {},
		"brew": {},
	}

	existingDir := t.TempDir()
	existingFile := filepath.Join(t.TempDir(), "not-a-dir.txt")

	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	newValidConfig := func(root string) *Config {
		return &Config{
			Version: 1,
			Control: ControlConfig{
				Concurrency: 8,
				Timeout:     "10m",
				DryRun:      false,
			},
			UI: UIConfig{
				TUI: false,
			},
			Repo: RepoConfig{
				Root: root,
				GitHub: GitHubConfig{
					Owner:    "",
					Protocol: "https",
				},
				Sync: RepoSyncConfig{
					AutoStash:       true,
					Prune:           true,
					SubmoduleUpdate: true,
				},
				Cleanup: RepoCleanupConfig{
					Enabled:         true,
					Target:          []string{"merged"},
					ExcludeBranches: []string{"main"},
				},
			},
			Sys: SysConfig{
				Enable:   []string{"apt"},
				Managers: map[string]ManagerConfig{},
			},
			Secrets: SecretsConfig{
				Enabled:  false,
				Provider: "bitwarden",
			},
		}
	}

	testCases := []struct {
		name               string
		cfg                *Config
		opts               ValidateOptions
		wantErrorSubstrs   []string
		wantWarningSubstrs []string
	}{
		{
			name:             "nil設定はエラー",
			cfg:              nil,
			wantErrorSubstrs: []string{"config", "nil"},
		},
		{
			name: "正しい設定はエラーなし",
			cfg:  newValidConfig(existingDir),
			opts: ValidateOptions{KnownSysManagers: knownManagers},
		},
		{
			name: "未対応versionはエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Version = 2
				return c
			}(),
			wantErrorSubstrs: []string{"version", "未対応"},
		},
		{
			name: "concurrency 0はエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Control.Concurrency = 0
				return c
			}(),
			wantErrorSubstrs: []string{"control.concurrency"},
		},
		{
			name: "timeoutが空はエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Control.Timeout = ""
				return c
			}(),
			wantErrorSubstrs: []string{"control.timeout", "空"},
		},
		{
			name: "timeoutが不正ならエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Control.Timeout = "invalid"
				return c
			}(),
			wantErrorSubstrs: []string{"control.timeout", "不正"},
		},
		{
			name: "timeoutが0以下はエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Control.Timeout = "0s"
				return c
			}(),
			wantErrorSubstrs: []string{"control.timeout", "0より大きい"},
		},
		{
			name: "repo.rootが空はエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Root = " "
				return c
			}(),
			wantErrorSubstrs: []string{"repo.root", "空"},
		},
		{
			name: "repo.rootが~始まりはエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Root = "~/src"
				return c
			}(),
			wantErrorSubstrs: []string{"repo.root", "チルダ"},
		},
		{
			name: "repo.rootが~user始まりはエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Root = "~user/src"
				return c
			}(),
			wantErrorSubstrs: []string{"repo.root", "チルダ"},
		},
		{
			name: "repo.rootが存在しない場合はエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Root = filepath.Join(existingDir, "not-exist")
				return c
			}(),
			wantErrorSubstrs: []string{"repo.root", "存在しません"},
		},
		{
			name: "repo.rootがファイルならエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Root = existingFile
				return c
			}(),
			wantErrorSubstrs: []string{"repo.root", "ディレクトリではありません"},
		},
		{
			name: "repo.github.protocolが不正ならエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.GitHub.Protocol = "git"
				return c
			}(),
			wantErrorSubstrs: []string{"repo.github.protocol", "不正"},
		},
		{
			name: "repo.cleanup.targetに未知の値があると警告",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Repo.Cleanup.Target = []string{"merged", "unknown"}
				return c
			}(),
			wantWarningSubstrs: []string{"repo.cleanup.target", "未知"},
		},
		{
			name: "secrets.enabled=true で provider が空ならエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Secrets.Enabled = true
				c.Secrets.Provider = ""
				return c
			}(),
			wantErrorSubstrs: []string{"secrets.provider", "空"},
		},
		{
			name: "secrets.enabled=true で provider が未対応ならエラー",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Secrets.Enabled = true
				c.Secrets.Provider = "vault"
				return c
			}(),
			wantErrorSubstrs: []string{"secrets.provider", "未対応"},
		},
		{
			name: "sys.enable の未知マネージャは警告（KnownSysManagers指定時）",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Sys.Enable = []string{"apt", "unknown"}
				return c
			}(),
			opts:               ValidateOptions{KnownSysManagers: knownManagers},
			wantWarningSubstrs: []string{"sys.enable", "未知"},
		},
		{
			name: "sys.enable の重複は警告（KnownSysManagers指定時）",
			cfg: func() *Config {
				c := newValidConfig(existingDir)
				c.Sys.Enable = []string{"apt", "apt"}
				return c
			}(),
			opts:               ValidateOptions{KnownSysManagers: knownManagers},
			wantWarningSubstrs: []string{"sys.enable", "重複"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Validate(tc.cfg, tc.opts)

			if len(tc.wantErrorSubstrs) == 0 && len(got.Errors) != 0 {
				t.Fatalf("Validate() errors = %#v, want empty", got.Errors)
			}

			for _, substr := range tc.wantErrorSubstrs {
				if !containsIssueSubstr(got.Errors, substr) {
					t.Fatalf("errors does not contain %q: %#v", substr, got.Errors)
				}
			}

			if len(tc.wantWarningSubstrs) == 0 && len(got.Warnings) != 0 && len(tc.wantErrorSubstrs) == 0 {
				t.Fatalf("Validate() warnings = %#v, want empty", got.Warnings)
			}

			for _, substr := range tc.wantWarningSubstrs {
				if !containsIssueSubstr(got.Warnings, substr) {
					t.Fatalf("warnings does not contain %q: %#v", substr, got.Warnings)
				}
			}
		})
	}
}

func containsIssueSubstr(issues []ValidationIssue, substr string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.String(), substr) {
			return true
		}
	}

	return false
}
