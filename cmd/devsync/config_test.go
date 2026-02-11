package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/AlecAivazis/survey/v2"
	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/testutil"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stderr

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() unexpected error: %v", err)
	}

	os.Stderr = writeEnd

	fn()

	if closeErr := writeEnd.Close(); closeErr != nil {
		t.Fatalf("stderr writer close error: %v", closeErr)
	}

	os.Stderr = original

	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, readEnd); copyErr != nil {
		t.Fatalf("failed to copy stderr: %v", copyErr)
	}

	if closeErr := readEnd.Close(); closeErr != nil {
		t.Fatalf("stderr reader close error: %v", closeErr)
	}

	return buf.String()
}

func TestNormalizeRepoRoot(t *testing.T) {
	testCases := []struct {
		name      string
		setup     func(t *testing.T)
		input     string
		want      string
		expectErr bool
	}{
		{
			name:      "空文字はエラー",
			input:     "   ",
			expectErr: true,
		},
		{
			name:  "通常パスはクリーン化される",
			input: "/tmp/work/../work/src",
			want:  "/tmp/work/src",
		},
		{
			name: "チルダ展開",
			setup: func(t *testing.T) {
				home := t.TempDir()
				testutil.SetTestHome(t, home)
			},
			input: "~/src",
			want:  "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			got, err := normalizeRepoRoot(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("normalizeRepoRoot(%q) error = nil, want error", tc.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("normalizeRepoRoot(%q) unexpected error: %v", tc.input, err)
			}

			want := tc.want
			if tc.input == "~/src" {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					t.Fatalf("os.UserHomeDir() unexpected error: %v", homeErr)
				}

				want = filepath.Join(home, "src")
			}

			if got != filepath.Clean(want) {
				t.Fatalf("normalizeRepoRoot(%q) = %q, want %q", tc.input, got, filepath.Clean(want))
			}
		})
	}
}

func TestEnsureRepoRoot(t *testing.T) {
	t.Parallel()

	t.Run("既存ディレクトリはそのまま成功", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		called := false

		err := ensureRepoRoot(dir, func(path string) (bool, error) {
			called = true
			return false, nil
		})
		if err != nil {
			t.Fatalf("ensureRepoRoot() unexpected error: %v", err)
		}

		if called {
			t.Fatalf("confirmCreate should not be called for existing directory")
		}
	})

	t.Run("既存ファイルはエラー", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()

		filePath := filepath.Join(root, "repo-root-file")
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := ensureRepoRoot(filePath, func(path string) (bool, error) {
			return true, nil
		})
		if err == nil {
			t.Fatalf("ensureRepoRoot() error = nil, want error")
		}
	})

	t.Run("未存在ディレクトリで作成承認時は作成して成功", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := filepath.Join(root, "new-src")

		err := ensureRepoRoot(target, func(path string) (bool, error) {
			if path != target {
				t.Fatalf("confirm path = %q, want %q", path, target)
			}

			return true, nil
		})
		if err != nil {
			t.Fatalf("ensureRepoRoot() unexpected error: %v", err)
		}

		info, statErr := os.Stat(target)
		if statErr != nil {
			t.Fatalf("created directory stat error: %v", statErr)
		}

		if !info.IsDir() {
			t.Fatalf("created path is not directory: %s", target)
		}
	})

	t.Run("未存在ディレクトリで拒否時はキャンセル終了", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := filepath.Join(root, "new-src")

		err := ensureRepoRoot(target, func(path string) (bool, error) {
			return false, nil
		})
		if !errors.Is(err, errConfigInitCanceled) {
			t.Fatalf("ensureRepoRoot() error = %v, want errConfigInitCanceled", err)
		}

		if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("target should not exist after cancel, statErr=%v", statErr)
		}
	})
}

func TestResolveGitHubOwnerDefault(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		lookup func(context.Context) (string, error)
		want   string
	}{
		{
			name: "取得成功時はtrimした値を返す",
			lookup: func(context.Context) (string, error) {
				return "scottlz0310\n", nil
			},
			want: "scottlz0310",
		},
		{
			name: "空白のみは空文字を返す",
			lookup: func(context.Context) (string, error) {
				return "   \n", nil
			},
			want: "",
		},
		{
			name: "取得失敗時は空文字を返す",
			lookup: func(context.Context) (string, error) {
				return "", errors.New("lookup failed")
			},
			want: "",
		},
		{
			name:   "lookup未指定時は空文字を返す",
			lookup: nil,
			want:   "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveGitHubOwnerDefault(context.Background(), tc.lookup)
			if got != tc.want {
				t.Fatalf("resolveGitHubOwnerDefault() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadExistingConfigForInit(t *testing.T) {
	t.Run("設定ファイル状態確認エラー時は警告を出してフォールバック", func(t *testing.T) {
		home := t.TempDir()
		testutil.SetTestHome(t, home)

		configPath := filepath.Join(home, ".config", "devsync", "config.yaml")
		if err := os.MkdirAll(configPath, 0o755); err != nil {
			t.Fatalf("failed to create directory at config path: %v", err)
		}

		var (
			gotCfg  *config.Config
			gotPath string
			gotOK   bool
		)

		stderr := captureStderr(t, func() {
			gotCfg, gotPath, gotOK = loadExistingConfigForInit()
		})

		if gotCfg != nil {
			t.Fatalf("cfg = %#v, want nil", gotCfg)
		}

		if gotOK {
			t.Fatalf("ok = true, want false")
		}

		if gotPath != configPath {
			t.Fatalf("configPath = %q, want %q", gotPath, configPath)
		}

		if !strings.Contains(stderr, "設定ファイル状態の確認に失敗") {
			t.Fatalf("stderr does not contain expected warning: %q", stderr)
		}
	})

	t.Run("既存設定の読み込み失敗時は警告を出してフォールバック", func(t *testing.T) {
		home := t.TempDir()
		testutil.SetTestHome(t, home)

		configDir := filepath.Join(home, ".config", "devsync")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}

		configPath := filepath.Join(configDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte("version: [invalid"), 0o644); err != nil {
			t.Fatalf("failed to write invalid config: %v", err)
		}

		var (
			gotCfg  *config.Config
			gotPath string
			gotOK   bool
		)

		stderr := captureStderr(t, func() {
			gotCfg, gotPath, gotOK = loadExistingConfigForInit()
		})

		if gotCfg != nil {
			t.Fatalf("cfg = %#v, want nil", gotCfg)
		}

		if gotOK {
			t.Fatalf("ok = true, want false")
		}

		if gotPath != configPath {
			t.Fatalf("configPath = %q, want %q", gotPath, configPath)
		}

		if !strings.Contains(stderr, "既存設定の読み込みに失敗") {
			t.Fatalf("stderr does not contain expected warning: %q", stderr)
		}

		if !strings.Contains(stderr, configPath) {
			t.Fatalf("stderr does not contain config path: %q", stderr)
		}
	})
}

func TestBuildConfigInitDefaults(t *testing.T) {
	t.Parallel()

	promptOptions := []string{"apt", "brew", "go", "npm", "snap", "flatpak", "fwupdmgr", "pipx", "cargo"}

	testCases := []struct {
		name                string
		home                string
		recommendedManagers []string
		existingCfg         *config.Config
		autoOwner           string
		want                configInitDefaults
	}{
		{
			name:                "既存設定なしは推奨値と自動オーナーを使用",
			home:                "/home/dev",
			recommendedManagers: []string{"apt", "snap"},
			existingCfg:         nil,
			autoOwner:           "auto-user",
			want: configInitDefaults{
				RepoRoot:        filepath.FromSlash("/home/dev/src"),
				GitHubOwner:     "auto-user",
				Concurrency:     8,
				EnableTUI:       false,
				EnabledManagers: []string{"apt", "snap"},
			},
		},
		{
			name:                "既存設定がある場合は既存値を優先",
			home:                "/home/dev",
			recommendedManagers: []string{"brew", "go"},
			existingCfg: &config.Config{
				Control: config.ControlConfig{
					Concurrency: 16,
				},
				UI: config.UIConfig{
					TUI: true,
				},
				Repo: config.RepoConfig{
					Root: "/work/repos",
					GitHub: config.GitHubConfig{
						Owner: "my-org",
					},
				},
				Sys: config.SysConfig{
					Enable: []string{"apt", "unknown", "apt", "npm"},
				},
			},
			autoOwner: "auto-user",
			want: configInitDefaults{
				RepoRoot:        "/work/repos",
				GitHubOwner:     "my-org",
				Concurrency:     16,
				EnableTUI:       true,
				EnabledManagers: []string{"apt", "npm"},
			},
		},
		{
			name:                "既存設定のenableが空なら推奨値へフォールバック",
			home:                "/home/dev",
			recommendedManagers: []string{"brew", "not-supported"},
			existingCfg: &config.Config{
				Control: config.ControlConfig{
					Concurrency: 0,
				},
				Repo: config.RepoConfig{
					Root: "/repos",
				},
				Sys: config.SysConfig{
					Enable: []string{},
				},
			},
			autoOwner: "auto-user",
			want: configInitDefaults{
				RepoRoot:        "/repos",
				GitHubOwner:     "auto-user",
				Concurrency:     8,
				EnableTUI:       false,
				EnabledManagers: []string{"brew"},
			},
		},
		{
			name:                "候補が空の場合はprompt options全体を使用",
			home:                "/home/dev",
			recommendedManagers: nil,
			existingCfg: &config.Config{
				Sys: config.SysConfig{
					Enable: []string{"unknown"},
				},
			},
			autoOwner: "",
			want: configInitDefaults{
				RepoRoot:        filepath.FromSlash("/home/dev/src"),
				GitHubOwner:     "",
				Concurrency:     8,
				EnableTUI:       false,
				EnabledManagers: promptOptions,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildConfigInitDefaults(tc.home, tc.recommendedManagers, promptOptions, tc.existingCfg, tc.autoOwner)

			if got.RepoRoot != tc.want.RepoRoot {
				t.Fatalf("RepoRoot = %q, want %q", got.RepoRoot, tc.want.RepoRoot)
			}

			if got.GitHubOwner != tc.want.GitHubOwner {
				t.Fatalf("GitHubOwner = %q, want %q", got.GitHubOwner, tc.want.GitHubOwner)
			}

			if got.Concurrency != tc.want.Concurrency {
				t.Fatalf("Concurrency = %d, want %d", got.Concurrency, tc.want.Concurrency)
			}

			if got.EnableTUI != tc.want.EnableTUI {
				t.Fatalf("EnableTUI = %v, want %v", got.EnableTUI, tc.want.EnableTUI)
			}

			if !reflect.DeepEqual(got.EnabledManagers, tc.want.EnabledManagers) {
				t.Fatalf("EnabledManagers = %#v, want %#v", got.EnabledManagers, tc.want.EnabledManagers)
			}
		})
	}
}

func TestGeneratedShellScripts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		buildScript     func(exePath string) string
		requiredPhrases []string
	}{
		{
			name:        "bashスクリプトはアンロック・env読込・run呼び出しを含む",
			buildScript: getBashScript,
			requiredPhrases: []string{
				`command -v devsync`,
				`bw login --check`,
				`token="$(bw unlock --raw)"`,
				`env_output="$("$DEVSYNC_PATH" env export)"`,
				`if [ $status -ne 0 ]; then`,
				`devsync-unlock || return 1`,
				`devsync-load-env || return 1`,
				`"$DEVSYNC_PATH" run "$@"`,
			},
		},
		{
			name:        "zshスクリプトはアンロック・env読込・run呼び出しを含む",
			buildScript: getZshScript,
			requiredPhrases: []string{
				`command -v devsync`,
				`bw login --check`,
				`token="$(bw unlock --raw)"`,
				`env_output="$("$DEVSYNC_PATH" env export)"`,
				`if [[ $status -ne 0 ]]; then`,
				`devsync-unlock || return 1`,
				`devsync-load-env || return 1`,
				`"$DEVSYNC_PATH" run "$@"`,
			},
		},
		{
			name:        "PowerShellスクリプトはアンロック・env読込・run呼び出しを含む",
			buildScript: getPowerShellScript,
			requiredPhrases: []string{
				`Get-Command devsync`,
				`bw login --check`,
				`$token = & bw unlock --raw`,
				`$envExports = & $DEVSYNC_PATH env export`,
				`$commandText = @($envExports) -join [Environment]::NewLine`,
				`if ([string]::IsNullOrWhiteSpace($commandText)) {`,
				`Invoke-Expression -Command $commandText -ErrorAction Stop`,
				`Write-Error "環境変数の読み込み中にエラーが発生しました: $_"`,
				`if (-not (devsync-unlock)) { return 1 }`,
				`if (-not (devsync-load-env)) { return 1 }`,
				`& $DEVSYNC_PATH run @args`,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			script := tc.buildScript("/tmp/devsync")
			for _, phrase := range tc.requiredPhrases {
				if !strings.Contains(script, phrase) {
					t.Fatalf("generated script does not contain required phrase %q", phrase)
				}
			}
		})
	}
}

func TestDecodePowerShellTextOutput(t *testing.T) {
	t.Parallel()

	encodeUTF16 := func(order binary.ByteOrder, s string) []byte {
		u16 := utf16.Encode([]rune(s))
		out := make([]byte, len(u16)*2)

		for i, v := range u16 {
			order.PutUint16(out[i*2:i*2+2], v)
		}

		return out
	}

	testCases := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "UTF-8/ASCII はそのまま返す",
			input: []byte("SGVsbG8=\n"),
			want:  "SGVsbG8=\n",
		},
		{
			name:  "UTF-16LE (BOMあり) を復元できる",
			input: append([]byte{0xFF, 0xFE}, encodeUTF16(binary.LittleEndian, "SGVsbG8=\r\n")...),
			want:  "SGVsbG8=\r\n",
		},
		{
			name:  "UTF-16BE (BOMあり) を復元できる",
			input: append([]byte{0xFE, 0xFF}, encodeUTF16(binary.BigEndian, "SGVsbG8=\n")...),
			want:  "SGVsbG8=\n",
		},
		{
			name:  "UTF-16LE (BOMなし) でも NUL を含む場合は復元できる",
			input: encodeUTF16(binary.LittleEndian, "SGVsbG8=\n"),
			want:  "SGVsbG8=\n",
		},
		{
			name:  "空入力は空文字",
			input: nil,
			want:  "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := decodePowerShellTextOutput(tc.input)
			if got != tc.want {
				t.Fatalf("decodePowerShellTextOutput() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetPowerShellProfilePathWithOutput(t *testing.T) {
	t.Parallel()

	encodeUTF16LEWithBOM := func(s string) []byte {
		u16 := utf16.Encode([]rune(s))
		out := make([]byte, 2+len(u16)*2)
		out[0] = 0xFF
		out[1] = 0xFE

		for i, v := range u16 {
			binary.LittleEndian.PutUint16(out[2+i*2:2+i*2+2], v)
		}

		return out
	}

	testCases := []struct {
		name            string
		shell           string
		outputBuilder   func(profilePath string) []byte
		outputErr       error
		wantCommand     string
		wantArgs        []string
		wantProfilePath bool
		wantDirCreated  bool
		wantErr         bool
	}{
		{
			name:  "pwsh は Base64(UTF-8) の出力を復元して返す",
			shell: "pwsh",
			outputBuilder: func(profilePath string) []byte {
				encoded := base64.StdEncoding.EncodeToString([]byte(profilePath))
				return []byte(encoded + "\r\n")
			},
			wantCommand:     "pwsh",
			wantArgs:        []string{"-NoProfile", "-NonInteractive", "-Command", "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"},
			wantProfilePath: true,
			wantDirCreated:  true,
			wantErr:         false,
		},
		{
			name:  "powershell は UTF-16LE (BOMあり) の出力でも復元できる",
			shell: "powershell",
			outputBuilder: func(profilePath string) []byte {
				encoded := base64.StdEncoding.EncodeToString([]byte(profilePath))
				return encodeUTF16LEWithBOM(encoded + "\r\n")
			},
			wantCommand:     "powershell",
			wantArgs:        []string{"-NoProfile", "-NonInteractive", "-Command", "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"},
			wantProfilePath: true,
			wantDirCreated:  true,
			wantErr:         false,
		},
		{
			name:  "出力が空ならエラー",
			shell: "pwsh",
			outputBuilder: func(string) []byte {
				return []byte("\r\n")
			},
			wantCommand: "pwsh",
			wantArgs:    []string{"-NoProfile", "-NonInteractive", "-Command", "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"},
			wantErr:     true,
		},
		{
			name:  "Base64 デコードできなければエラー",
			shell: "pwsh",
			outputBuilder: func(string) []byte {
				return []byte("not base64\r\n")
			},
			wantCommand: "pwsh",
			wantArgs:    []string{"-NoProfile", "-NonInteractive", "-Command", "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"},
			wantErr:     true,
		},
		{
			name:  "Base64 デコード後のパスが空ならエラー",
			shell: "pwsh",
			outputBuilder: func(string) []byte {
				encoded := base64.StdEncoding.EncodeToString([]byte(" \r\n"))
				return []byte(encoded + "\r\n")
			},
			wantCommand: "pwsh",
			wantArgs:    []string{"-NoProfile", "-NonInteractive", "-Command", "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"},
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			profilePath := filepath.Join(root, "OneDrive", "ドキュメント", "PowerShell", "Microsoft.PowerShell_profile.ps1")
			profileDir := filepath.Dir(profilePath)

			gotCommand := ""
			gotArgs := []string(nil)

			output := tc.outputBuilder(profilePath)
			got, err := getPowerShellProfilePathWithOutput(tc.shell, func(command string, args ...string) ([]byte, error) {
				gotCommand = command

				gotArgs = append([]string(nil), args...)
				return output, tc.outputErr
			})

			if gotCommand != tc.wantCommand {
				t.Fatalf("command = %q, want %q", gotCommand, tc.wantCommand)
			}

			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", gotArgs, tc.wantArgs)
			}

			if tc.wantErr {
				if err == nil {
					t.Fatalf("error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantProfilePath && got != profilePath {
				t.Fatalf("profilePath = %q, want %q", got, profilePath)
			}

			if tc.wantDirCreated {
				if _, statErr := os.Stat(profileDir); statErr != nil {
					t.Fatalf("profileDir should exist, statErr=%v", statErr)
				}
			}
		})
	}
}

func TestQuoteForPosixShell(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "空文字は単一引用符の空",
			input: "",
			want:  "''",
		},
		{
			name:  "通常パスは単一引用符で囲む",
			input: "/home/user/.bashrc",
			want:  `'/home/user/.bashrc'`,
		},
		{
			name:  "スペースを含むパスも単一引用符で囲む",
			input: "/home/user/My Folder/.bashrc",
			want:  `'/home/user/My Folder/.bashrc'`,
		},
		{
			name:  "シングルクォートは POSIX 形式でエスケープする",
			input: "/home/user/it's/.bashrc",
			want:  `'/home/user/it'\''s/.bashrc'`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := quoteForPosixShell(tc.input)
			if got != tc.want {
				t.Fatalf("quoteForPosixShell(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestQuoteForPowerShell(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "空文字は単一引用符の空",
			input: "",
			want:  "''",
		},
		{
			name:  "通常パスは単一引用符で囲む",
			input: `C:\Users\jojob\OneDrive\ドキュメント\PowerShell\Microsoft.PowerShell_profile.ps1`,
			want:  `'C:\Users\jojob\OneDrive\ドキュメント\PowerShell\Microsoft.PowerShell_profile.ps1'`,
		},
		{
			name:  "シングルクォートは 2 つにしてエスケープする",
			input: `C:\Users\O'Brien\file.ps1`,
			want:  `'C:\Users\O''Brien\file.ps1'`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := quoteForPowerShell(tc.input)
			if got != tc.want {
				t.Fatalf("quoteForPowerShell(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetSourceKeyword(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		shell string
		want  string
	}{
		{
			name:  "sh は dot コマンドを返す",
			shell: "sh",
			want:  ".",
		},
		{
			name:  "bash は source を返す",
			shell: shellBash,
			want:  "source",
		},
		{
			name:  "zsh は source を返す",
			shell: shellZsh,
			want:  "source",
		},
		{
			name:  "fish は source を返す",
			shell: "fish",
			want:  "source",
		},
		{
			name:  "不明なシェルも source を返す",
			shell: "unknown",
			want:  "source",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := getSourceKeyword(tc.shell)
			if got != tc.want {
				t.Errorf("getSourceKeyword(%q) = %q, want %q", tc.shell, got, tc.want)
			}
		})
	}
}

func TestBuildReloadCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		shell      string
		rcFilePath string
		want       string
	}{
		{
			name:       "pwsh は $PROFILE を dot source する",
			shell:      "pwsh",
			rcFilePath: "/tmp/.bashrc",
			want:       ". $PROFILE",
		},
		{
			name:       "powershell は $PROFILE を dot source する",
			shell:      shellPowerShell,
			rcFilePath: "/tmp/.bashrc",
			want:       ". $PROFILE",
		},
		{
			name:       "bash は rc ファイルを source する（パスはクォート）",
			shell:      shellBash,
			rcFilePath: "/home/user/My Folder/.bashrc",
			want:       `source '/home/user/My Folder/.bashrc'`,
		},
		{
			name:       "zsh は rc ファイルを source する（パスはクォート）",
			shell:      shellZsh,
			rcFilePath: "/home/user/My Folder/.zshrc",
			want:       `source '/home/user/My Folder/.zshrc'`,
		},
		{
			name:       "sh は dot コマンドを使用する（POSIX互換）",
			shell:      "sh",
			rcFilePath: "/home/user/.profile",
			want:       `. '/home/user/.profile'`,
		},
		{
			name:       "sh でスペースを含むパスも正しくクォートされる",
			shell:      "sh",
			rcFilePath: "/home/user/My Folder/.profile",
			want:       `. '/home/user/My Folder/.profile'`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildReloadCommand(tc.shell, tc.rcFilePath)
			if got != tc.want {
				t.Fatalf("buildReloadCommand(%q, %q) = %q, want %q", tc.shell, tc.rcFilePath, got, tc.want)
			}
		})
	}
}

func TestResolveInitScript(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	testCases := []struct {
		name         string
		shell        string
		wantFilename string
		wantContains string
	}{
		{
			name:         "powershell は init.ps1 を生成する",
			shell:        shellPowerShell,
			wantFilename: "init.ps1",
			wantContains: "shell integration for PowerShell",
		},
		{
			name:         "pwsh は init.ps1 を生成する",
			shell:        "pwsh",
			wantFilename: "init.ps1",
			wantContains: "shell integration for PowerShell",
		},
		{
			name:         "zsh は init.zsh を生成する",
			shell:        shellZsh,
			wantFilename: "init.zsh",
			wantContains: "shell integration for zsh",
		},
		{
			name:         "bash は init.bash を生成する",
			shell:        shellBash,
			wantFilename: "init.bash",
			wantContains: "shell integration for bash",
		},
		{
			name:         "未対応シェルは init.sh を生成する（中身は bash スクリプト）",
			shell:        "sh",
			wantFilename: "init.sh",
			wantContains: "shell integration for bash",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotPath, gotContent := resolveInitScript(tc.shell, configDir, "/tmp/devsync")
			if filepath.Base(gotPath) != tc.wantFilename {
				t.Fatalf("filename = %q, want %q", filepath.Base(gotPath), tc.wantFilename)
			}

			if !strings.Contains(gotContent, tc.wantContains) {
				t.Fatalf("content does not contain %q", tc.wantContains)
			}
		})
	}
}

func TestResolveShellRcFile_Posix(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	scriptPath := filepath.Join(home, ".config", "devsync", "init.bash")

	testCases := []struct {
		name            string
		shell           string
		wantRcFilePath  string
		wantSource      string
		wantSupported   bool
		wantErr         bool
		wantErrContains string
	}{
		{
			name:           "bash は .bashrc を返す",
			shell:          shellBash,
			wantRcFilePath: filepath.Join(home, ".bashrc"),
			wantSource:     "source " + quoteForPosixShell(scriptPath),
			wantSupported:  true,
		},
		{
			name:           "zsh は .zshrc を返す",
			shell:          shellZsh,
			wantRcFilePath: filepath.Join(home, ".zshrc"),
			wantSource:     "source " + quoteForPosixShell(scriptPath),
			wantSupported:  true,
		},
		{
			name:          "未対応シェルは supported=false",
			shell:         "fish",
			wantSupported: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotRc, gotSource, gotSupported, err := resolveShellRcFile(tc.shell, home, scriptPath)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr=%v", err, tc.wantErr)
			}

			if tc.wantErrContains != "" && err != nil && !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("err = %q does not contain %q", err.Error(), tc.wantErrContains)
			}

			if gotSupported != tc.wantSupported {
				t.Fatalf("supported = %v, want %v", gotSupported, tc.wantSupported)
			}

			if gotRc != tc.wantRcFilePath {
				t.Fatalf("rcFilePath = %q, want %q", gotRc, tc.wantRcFilePath)
			}

			if gotSource != tc.wantSource {
				t.Fatalf("sourceCommand = %q, want %q", gotSource, tc.wantSource)
			}
		})
	}
}

func TestResolveShellRcFile_PowerShell(t *testing.T) {
	original := getPowerShellProfilePathStep
	t.Cleanup(func() {
		getPowerShellProfilePathStep = original
	})

	scriptPath := `C:\Users\jojob\.config\devsync\init.ps1`
	wantProfile := `C:\Users\jojob\OneDrive\ドキュメント\PowerShell\Microsoft.PowerShell_profile.ps1`

	t.Run("取得成功ならプロファイルパスと dot source コマンドを返す", func(t *testing.T) {
		getPowerShellProfilePathStep = func(string) (string, error) {
			return wantProfile, nil
		}

		gotRc, gotSource, gotSupported, err := resolveShellRcFile(shellPowerShell, "ignored", scriptPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !gotSupported {
			t.Fatalf("supported = false, want true")
		}

		if gotRc != wantProfile {
			t.Fatalf("rcFilePath = %q, want %q", gotRc, wantProfile)
		}

		wantSource := ". " + quoteForPowerShell(scriptPath)
		if gotSource != wantSource {
			t.Fatalf("sourceCommand = %q, want %q", gotSource, wantSource)
		}
	})

	t.Run("取得失敗なら err を返す", func(t *testing.T) {
		getPowerShellProfilePathStep = func(string) (string, error) {
			return "", errors.New("lookup failed")
		}

		_, _, gotSupported, err := resolveShellRcFile(shellPowerShell, "ignored", scriptPath)
		if err == nil {
			t.Fatalf("error = nil, want error")
		}

		if !gotSupported {
			t.Fatalf("supported = false, want true")
		}
	})
}

func TestConfirmAddToRc(t *testing.T) {
	original := surveyAskOneStep
	t.Cleanup(func() {
		surveyAskOneStep = original
	})

	t.Run("AskOne が true を設定した場合は true を返す", func(t *testing.T) {
		surveyAskOneStep = func(prompt survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
			v, ok := response.(*bool)
			if !ok {
				return errors.New("response must be *bool")
			}

			*v = true

			return nil
		}

		got, err := confirmAddToRc("/tmp/.bashrc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !got {
			t.Fatalf("got=false, want true")
		}
	})

	t.Run("AskOne がエラーならエラーを返す", func(t *testing.T) {
		surveyAskOneStep = func(prompt survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
			return errors.New("ask failed")
		}

		_, err := confirmAddToRc("/tmp/.bashrc")
		if err == nil {
			t.Fatalf("error = nil, want error")
		}
	})
}

func createConfigDir(t *testing.T, home string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(home, ".config", "devsync"), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
}

func TestGenerateShellInit_UnsupportedShell(t *testing.T) {
	t.Setenv("PSModulePath", "")
	t.Setenv("SHELL", "")

	home := t.TempDir()

	createConfigDir(t, home)

	if err := generateShellInit(home); err != nil {
		t.Fatalf("generateShellInit() unexpected error: %v", err)
	}

	scriptPath := filepath.Join(home, ".config", "devsync", "init.sh")
	if _, statErr := os.Stat(scriptPath); statErr != nil {
		t.Fatalf("init script should exist, statErr=%v", statErr)
	}

	rcFilePath := filepath.Join(home, ".bashrc")
	if _, statErr := os.Stat(rcFilePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rc file should not be created, statErr=%v", statErr)
	}
}

func TestGenerateShellInit_Bash_Declined(t *testing.T) {
	original := surveyAskOneStep
	t.Cleanup(func() {
		surveyAskOneStep = original
	})

	t.Setenv("PSModulePath", "")
	t.Setenv("SHELL", "/bin/bash")

	surveyAskOneStep = func(prompt survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
		v, ok := response.(*bool)
		if !ok {
			return errors.New("response must be *bool")
		}

		*v = false

		return nil
	}

	home := t.TempDir()

	createConfigDir(t, home)

	if err := generateShellInit(home); err != nil {
		t.Fatalf("generateShellInit() unexpected error: %v", err)
	}

	scriptPath := filepath.Join(home, ".config", "devsync", "init.bash")
	if _, statErr := os.Stat(scriptPath); statErr != nil {
		t.Fatalf("init script should exist, statErr=%v", statErr)
	}

	rcFilePath := filepath.Join(home, ".bashrc")
	if _, statErr := os.Stat(rcFilePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rc file should not be created, statErr=%v", statErr)
	}
}

func TestGenerateShellInit_Bash_Accepted(t *testing.T) {
	original := surveyAskOneStep
	t.Cleanup(func() {
		surveyAskOneStep = original
	})

	t.Setenv("PSModulePath", "")
	t.Setenv("SHELL", "/bin/bash")

	surveyAskOneStep = func(prompt survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
		v, ok := response.(*bool)
		if !ok {
			return errors.New("response must be *bool")
		}

		*v = true

		return nil
	}

	home := t.TempDir()

	createConfigDir(t, home)

	if err := generateShellInit(home); err != nil {
		t.Fatalf("generateShellInit() unexpected error: %v", err)
	}

	scriptPath := filepath.Join(home, ".config", "devsync", "init.bash")
	if _, statErr := os.Stat(scriptPath); statErr != nil {
		t.Fatalf("init script should exist, statErr=%v", statErr)
	}

	rcFilePath := filepath.Join(home, ".bashrc")

	content, readErr := os.ReadFile(rcFilePath)
	if readErr != nil {
		t.Fatalf("rc file should exist, readErr=%v", readErr)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "# >>> devsync >>>") {
		t.Fatalf("rc file does not contain marker begin: %q", contentStr)
	}

	if !strings.Contains(contentStr, "# <<< devsync <<<") {
		t.Fatalf("rc file does not contain marker end: %q", contentStr)
	}

	if !strings.Contains(contentStr, scriptPath) {
		t.Fatalf("rc file does not contain script path: %q", contentStr)
	}
}

func TestGenerateShellInit_PowerShell_ProfileLookupFailed(t *testing.T) {
	original := getPowerShellProfilePathStep
	t.Cleanup(func() {
		getPowerShellProfilePathStep = original
	})

	t.Setenv("PSModulePath", "dummy")
	t.Setenv("SHELL", "")

	getPowerShellProfilePathStep = func(string) (string, error) {
		return "", errors.New("lookup failed")
	}

	home := t.TempDir()

	createConfigDir(t, home)

	if err := generateShellInit(home); err != nil {
		t.Fatalf("generateShellInit() unexpected error: %v", err)
	}

	scriptPath := filepath.Join(home, ".config", "devsync", "init.ps1")
	if _, statErr := os.Stat(scriptPath); statErr != nil {
		t.Fatalf("init script should exist, statErr=%v", statErr)
	}
}
