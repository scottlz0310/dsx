// Package updater はシステムパッケージの更新機能を提供します。
// 各パッケージマネージャは Updater インターフェースを実装し、
// レジストリに登録することで拡張可能な設計になっています。
package updater

import (
	"context"
	"fmt"
	"sync"

	"github.com/scottlz0310/devsync/internal/config"
)

// Updater はパッケージマネージャの共通インターフェースです。
// 新しいパッケージマネージャをサポートする場合は、このインターフェースを実装してください。
type Updater interface {
	// Name はマネージャの識別名を返します（例: "apt", "brew", "go"）
	Name() string

	// DisplayName は表示用の名前を返します（例: "APT (Debian/Ubuntu)"）
	DisplayName() string

	// IsAvailable はこのマネージャが現在の環境で利用可能かを判定します。
	// 例: apt は Linux 環境でのみ利用可能
	IsAvailable() bool

	// Check は更新可能なパッケージを確認します。
	// DryRun モードでの事前確認に使用されます。
	Check(ctx context.Context) (*CheckResult, error)

	// Update はパッケージの更新を実行します。
	Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error)

	// Configure はマネージャ固有の設定を適用します。
	// config.ManagerConfig から必要な設定を読み取ります。
	Configure(cfg config.ManagerConfig) error
}

// CheckResult は更新確認の結果を保持します。
type CheckResult struct {
	// AvailableUpdates は更新可能なパッケージ数
	AvailableUpdates int
	// Packages は更新可能なパッケージの詳細リスト
	Packages []PackageInfo
	// Message は追加情報（任意）
	Message string
}

// PackageInfo はパッケージの情報を保持します。
type PackageInfo struct {
	Name           string // パッケージ名
	CurrentVersion string // 現在のバージョン
	NewVersion     string // 新しいバージョン
}

// UpdateOptions は更新実行時のオプションです。
type UpdateOptions struct {
	// DryRun が true の場合、実際の更新は行わず計画のみ表示
	DryRun bool
	// Verbose が true の場合、詳細なログを出力
	Verbose bool
}

// UpdateResult は更新実行の結果を保持します。
type UpdateResult struct {
	// UpdatedCount は更新されたパッケージ数
	UpdatedCount int
	// FailedCount は失敗したパッケージ数
	FailedCount int
	// Packages は更新されたパッケージの詳細リスト
	Packages []PackageInfo
	// Errors は発生したエラーのリスト
	Errors []error
	// Message は追加情報（任意）
	Message string
}

// Registry はUpdaterの登録・取得を管理します。
// グローバルなレジストリを通じて、利用可能なマネージャを管理します。
type Registry struct {
	mu       sync.RWMutex
	updaters map[string]Updater
}

// グローバルレジストリのインスタンス
var globalRegistry = &Registry{
	updaters: make(map[string]Updater),
}

// Register は新しいUpdaterをレジストリに登録します。
// 通常、各マネージャパッケージの init() 関数から呼び出されます。
func Register(u Updater) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.updaters[u.Name()] = u
}

// Get は指定された名前のUpdaterを取得します。
func Get(name string) (Updater, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	u, ok := globalRegistry.updaters[name]
	return u, ok
}

// All は登録されている全てのUpdaterを返します。
func All() []Updater {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	result := make([]Updater, 0, len(globalRegistry.updaters))
	for _, u := range globalRegistry.updaters {
		result = append(result, u)
	}
	return result
}

// Available は現在の環境で利用可能なUpdaterのみを返します。
func Available() []Updater {
	all := All()
	result := make([]Updater, 0, len(all))
	for _, u := range all {
		if u.IsAvailable() {
			result = append(result, u)
		}
	}
	return result
}

// GetEnabled は設定で有効化されているUpdaterを返します。
// 設定の enable リストに含まれ、かつ利用可能なマネージャのみを返します。
func GetEnabled(cfg *config.SysConfig) ([]Updater, error) {
	if cfg == nil || len(cfg.Enable) == 0 {
		return nil, nil
	}

	result := make([]Updater, 0, len(cfg.Enable))
	var notFound []string

	for _, name := range cfg.Enable {
		u, ok := Get(name)
		if !ok {
			notFound = append(notFound, name)
			continue
		}
		if !u.IsAvailable() {
			// 利用不可のマネージャは警告のみでスキップ
			continue
		}
		// マネージャ固有の設定を適用
		if managerCfg, ok := cfg.Managers[name]; ok {
			if err := u.Configure(managerCfg); err != nil {
				return nil, fmt.Errorf("%s の設定適用に失敗: %w", name, err)
			}
		}
		result = append(result, u)
	}

	if len(notFound) > 0 {
		return result, fmt.Errorf("未知のマネージャが指定されています: %v", notFound)
	}

	return result, nil
}
