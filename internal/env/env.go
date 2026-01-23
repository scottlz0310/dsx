package env

import (
	"os"
	"strings"
)

// IsContainer は現在のプロセスがコンテナ内で実行されているかどうかを判定します。
// Checks for:
// 1. /.dockerenv file presence
// 2. CODESPACES env var
// 3. REMOTE_CONTAINERS env var
func IsContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if os.Getenv("CODESPACES") == "true" {
		return true
	}

	if os.Getenv("REMOTE_CONTAINERS") == "true" {
		return true
	}

	return false
}

// IsWSL は現在のプロセスがWSL (Windows Subsystem for Linux) 内で実行されているかどうかを判定します。
// Checks content of /proc/version for "microsoft" and "wsl".
func IsWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "microsoft") && strings.Contains(s, "wsl")
}

// GetRecommendedManagers は現在の環境に基づいて推奨されるパッケージマネージャのリストを返します。
func GetRecommendedManagers() []string {
	managers := []string{}

	// Common
	managers = append(managers, "go", "npm")

	if IsContainer() {
		// Container environment usually relies on apt/apk but user might not want to update system packages directly.
		// Use apt if it's Debian/Ubuntu based.
		if isDebianLike() {
			managers = append(managers, "apt")
		}
	} else if IsWSL() {
		// WSL environment
		if isDebianLike() {
			managers = append(managers, "apt")
		}
		managers = append(managers, "brew") // Linuxbrew is common in WSL
	} else {
		// Host Linux/macOS
		if isDebianLike() {
			managers = append(managers, "apt")
			managers = append(managers, "snap")
		}
		// Mac/Linux common
		managers = append(managers, "brew")
	}

	return managers
}

// isDebianLike returns true if apt-get is available.
func isDebianLike() bool {
	_, err := os.Stat("/usr/bin/apt-get")
	return err == nil
}
