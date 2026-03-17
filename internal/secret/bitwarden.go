package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Bitwarden ステータス定数
const (
	statusUnlocked = "unlocked"
)

// syncFunc はテストで差し替え可能な同期処理の関数変数です。
var syncFunc = Sync

// unlockRawFunc はテストで差し替え可能な bw unlock --raw 実行の関数変数です。
var unlockRawFunc = runBWUnlockRaw

// loginCheckFunc はテストで差し替え可能な bw login --check 実行の関数変数です。
var loginCheckFunc = runBWLoginCheck

// runBWLoginCheck は bw login --check を実行してログイン状態を確認します。
func runBWLoginCheck() error {
	cmd := exec.CommandContext(context.Background(), "bw", "login", "--check")
	return cmd.Run()
}

// runBWUnlockRaw は bw unlock --raw を実行してトークンを返します。
func runBWUnlockRaw() (string, error) {
	defer debugTimerStart("bw unlock --raw")()

	cmd := exec.CommandContext(context.Background(), "bw", "unlock", "--raw")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw unlock が失敗しました: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// debugLog はデバッグログを出力します。DSX_DEBUG=1 で有効化されます。
func debugLog(format string, args ...interface{}) {
	if os.Getenv("DSX_DEBUG") != "1" {
		return
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[DEBUG %s] %s\n", time.Now().Format("15:04:05.000"), msg)
}

// debugTimerStart はデバッグ計測用タイマーを開始します。
func debugTimerStart(label string) func() {
	if os.Getenv("DSX_DEBUG") != "1" {
		return func() {}
	}

	start := time.Now()

	debugLog("%s: 開始", label)

	return func() {
		debugLog("%s: 完了 (%s)", label, time.Since(start).Round(time.Millisecond))
	}
}

// BitwardenItem は `bw list items` のJSON出力の構造体です。
type BitwardenItem struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Notes  string                 `json:"notes"`
	Fields []BitwardenCustomField `json:"fields"`
	Login  *BitwardenLogin        `json:"login,omitempty"`
}

// BitwardenCustomField はカスタムフィールドの構造体です。
type BitwardenCustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

// BitwardenLogin はログイン情報の構造体です。
type BitwardenLogin struct {
	Username string `json:"username"`
	Secret   string `json:"-"`
}

// UnmarshalJSON は login.password を Secret に取り込みます。
func (l *BitwardenLogin) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if username, ok := raw["username"]; ok {
		if err := json.Unmarshal(username, &l.Username); err != nil {
			return fmt.Errorf("login.username の解析に失敗: %w", err)
		}
	}

	if password, ok := raw["password"]; ok {
		if err := json.Unmarshal(password, &l.Secret); err != nil {
			return fmt.Errorf("login.password の解析に失敗: %w", err)
		}
	}

	return nil
}

// BitwardenStatus は `bw status` の出力構造体です。
type BitwardenStatus struct {
	Status string `json:"status"`
}

// LoadStats は環境変数読み込みの統計情報です。
type LoadStats struct {
	Loaded  int
	Missing int
	Invalid int
}

// Unlock はBitwardenのアンロックを行い、BW_SESSIONを設定します。
// 参考実装: bw-unlock 関数
func Unlock() error {
	defer debugTimerStart("Unlock 全体")()

	// bwコマンドの存在確認
	if _, err := exec.LookPath("bw"); err != nil {
		return fmt.Errorf("bw コマンドが見つかりません。Bitwarden CLI をインストールしてください")
	}

	// ログイン状態の確認
	done := debugTimerStart("bw login --check")

	cmd := exec.CommandContext(context.Background(), "bw", "login", "--check")
	if err := cmd.Run(); err != nil {
		done()

		return fmt.Errorf("bitwarden にログインしていません。'bw login' を実行してください")
	}

	done()

	// 既にセッションがある場合は状態を確認し、アンロック済みなら何もしない
	if os.Getenv("BW_SESSION") != "" {
		status, err := getBitwardenStatus()
		if err == nil && status == statusUnlocked {
			debugLog("BW_SESSION 設定済み＋unlocked → スキップ")
			fmt.Fprintln(os.Stderr, "このシェルでは既に BW_SESSION が設定されています。")
			return nil
		}

		fmt.Fprintln(os.Stderr, "BW_SESSION が設定されていますがロックされています。再アンロックします...")
	}

	// アンロック実行
	fmt.Fprintln(os.Stderr, "🔐 Bitwarden をアンロックしています...")

	done = debugTimerStart("bw unlock --raw")
	cmd = exec.CommandContext(context.Background(), "bw", "unlock", "--raw")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()

	done()

	if err != nil {
		return fmt.Errorf("bw unlock が失敗しました: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return fmt.Errorf("bw unlock --raw の出力が空です")
	}

	// トークン形式の検証（Base64文字セット）
	if !regexp.MustCompile(`^[A-Za-z0-9+/=._-]+$`).MatchString(token) {
		return fmt.Errorf("bw unlock --raw の出力形式が認識できません")
	}

	// BW_SESSIONを設定
	if err := os.Setenv("BW_SESSION", token); err != nil {
		return fmt.Errorf("BW_SESSION の設定に失敗しました: %w", err)
	}

	fmt.Fprintln(os.Stderr, "✅ このシェルで Bitwarden をアンロックしました。")

	return nil
}

// Sync はBitwardenのローカルキャッシュをサーバーと同期します。
// キャッシュが古い場合に最新データを取得するため、環境変数読み込み前に実行します。
func Sync() error {
	defer debugTimerStart("bw sync")()

	fmt.Fprintln(os.Stderr, "🔄 Bitwarden データを同期しています...")

	cmd := exec.CommandContext(context.Background(), "bw", "sync")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bw sync が失敗しました: %w: %s", err, strings.TrimSpace(string(output)))
	}

	fmt.Fprintln(os.Stderr, "✅ Bitwarden データを同期しました。")

	return nil
}

// LoadEnv はBitwardenから "env:" プレフィックス付きの項目を取得し、環境変数に設定します。
// 参考実装: bw-load-env 関数
func LoadEnv() (*LoadStats, error) {
	defer debugTimerStart("LoadEnv 全体")()

	stats := &LoadStats{}

	// 事前チェック
	if err := checkBitwardenPrerequisites(); err != nil {
		return stats, err
	}

	// サーバーと同期して最新データを取得（参照実装に合わせ、失敗時は中断）
	if err := syncFunc(); err != nil {
		return stats, err
	}

	// env: プレフィックス付きの項目を検索
	items, err := fetchBitwardenEnvItems()
	if err != nil {
		return stats, err
	}

	// 各項目を処理して環境変数に設定
	if err := processEnvItems(items, stats); err != nil {
		return stats, err
	}

	// 結果の表示
	if err := printLoadStats(stats); err != nil {
		return stats, err
	}

	return stats, nil
}

// checkBitwardenPrerequisites はBitwarden CLIの事前条件をチェックします。
func checkBitwardenPrerequisites() error {
	// bwコマンドの存在確認
	if _, err := exec.LookPath("bw"); err != nil {
		return fmt.Errorf("bw コマンドが見つかりません")
	}

	// jqコマンドの存在確認
	if _, err := exec.LookPath("jq"); err != nil {
		return fmt.Errorf("jq コマンドが見つかりません")
	}

	// BW_SESSIONが設定されていない場合はスキップ
	if os.Getenv("BW_SESSION") == "" {
		return fmt.Errorf("BW_SESSION が設定されていません。Bitwarden をアンロックしてください")
	}

	// ステータス確認
	status, err := getBitwardenStatus()
	if err != nil {
		return fmt.Errorf("bitwarden のステータス確認に失敗しました: %w", err)
	}

	if status != statusUnlocked {
		return fmt.Errorf("bitwarden がロックされています。'bw unlock' を実行してください")
	}

	return nil
}

// fetchBitwardenEnvItems はBitwardenからenv:プレフィックス付きの項目を取得します。
func fetchBitwardenEnvItems() ([]BitwardenItem, error) {
	fmt.Fprintln(os.Stderr, "🔑 環境変数を読み込んでいます...")

	defer debugTimerStart("bw list items --search env:")()

	cmd := exec.CommandContext(context.Background(), "bw", "list", "items", "--search", "env:")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items が失敗しました: %w", err)
	}

	var items []BitwardenItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("JSON のパースに失敗しました: %w", err)
	}

	return items, nil
}

// processEnvItems は各項目を処理して環境変数に設定します。
func processEnvItems(items []BitwardenItem, stats *LoadStats) error {
	for i := range items {
		if !strings.HasPrefix(items[i].Name, "env:") {
			continue
		}

		if err := processEnvItem(&items[i], stats); err != nil {
			return err
		}
	}

	return nil
}

// processEnvItem は単一の項目を処理して環境変数に設定します。
func processEnvItem(item *BitwardenItem, stats *LoadStats) error {
	// 変数名を抽出（env: プレフィックスを除去）
	varName := strings.TrimPrefix(item.Name, "env:")

	// 変数名の検証
	if !isValidEnvVarName(varName) {
		fmt.Fprintf(os.Stderr, "⚠️  項目名から無効な環境変数名をスキップ: %s\n", item.Name)

		stats.Invalid++

		return nil
	}

	// 値を取得
	value := getEnvValue(item)
	if value == "" {
		fmt.Fprintf(os.Stderr, "⚠️  項目 %s に 'value' カスタムフィールドがありません\n", item.Name)

		stats.Missing++

		return nil
	}

	// 環境変数に設定
	if err := os.Setenv(varName, value); err != nil {
		return fmt.Errorf("環境変数 %s の設定に失敗: %w", varName, err)
	}

	fmt.Fprintf(os.Stderr, "✅ %s を注入しました\n", varName)

	stats.Loaded++

	return nil
}

// getEnvValue は項目から環境変数の値を取得します。
func getEnvValue(item *BitwardenItem) string {
	// カスタムフィールド "value" から値を取得（大文字小文字を区別しない）
	value := getCustomFieldValue(item.Fields, "value")

	// フィールドがない場合は login.password をフォールバックとして利用
	if value == "" && item.Login != nil {
		value = strings.TrimSpace(item.Login.Secret)
		if value != "" {
			fmt.Fprintf(os.Stderr, "ℹ️  項目 %s は 'value' フィールドが無いので login.password を利用します\n", item.Name)
		}
	}

	return value
}

// printLoadStats は読み込み結果を表示します。
func printLoadStats(stats *LoadStats) error {
	if stats.Loaded == 0 && stats.Missing == 0 && stats.Invalid == 0 {
		return fmt.Errorf("bitwarden に env: 項目が見つかりません")
	}

	fmt.Fprintf(os.Stderr, "✅ %d 個の環境変数を読み込みました。\n", stats.Loaded)

	if stats.Missing > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  %d 個の項目で value フィールドが見つかりませんでした。\n", stats.Missing)
	}

	if stats.Invalid > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  %d 個の項目で無効な環境変数名がありました。\n", stats.Invalid)
	}

	return nil
}

// GetEnvVars はBitwardenから環境変数を取得し、map形式で返します（sync なし・高速）。
// dsx env export / dsx env run コマンドで使用します。
// トークンロール等で即時反映が必要な場合は GetEnvVarsWithSync を使用してください。
func GetEnvVars() (map[string]string, error) {
	return getEnvVarsInternal(false)
}

// GetEnvVarsWithSync はBitwardenと強制同期してから環境変数を取得します。
// トークンロールなど、シークレットが更新された直後に使用します。
func GetEnvVarsWithSync() (map[string]string, error) {
	return getEnvVarsInternal(true)
}

// checkBitwardenSession は bw CLI が利用可能かつセッションがアンロック済みかを確認します。
func checkBitwardenSession() error {
	if _, err := exec.LookPath("bw"); err != nil {
		return fmt.Errorf("bw コマンドが見つかりません")
	}

	if os.Getenv("BW_SESSION") == "" {
		return fmt.Errorf("BW_SESSION が設定されていません。bitwarden をアンロックしてください")
	}

	status, err := getBitwardenStatus()
	if err != nil {
		return fmt.Errorf("bitwarden のステータス確認に失敗しました: %w", err)
	}

	if status != statusUnlocked {
		return fmt.Errorf("bitwarden がロックされています。'bw unlock' を実行してください")
	}

	return nil
}

// buildEnvMapFromItems は BitwardenItem スライスから env: プレフィックス付き項目を
// 環境変数マップに変換します。
func buildEnvMapFromItems(items []BitwardenItem) map[string]string {
	envVars := make(map[string]string)

	for _, item := range items {
		if !strings.HasPrefix(item.Name, "env:") {
			continue
		}

		varName := strings.TrimPrefix(item.Name, "env:")
		if !isValidEnvVarName(varName) {
			continue
		}

		value := getCustomFieldValue(item.Fields, "value")
		if value == "" && item.Login != nil {
			value = strings.TrimSpace(item.Login.Secret)
		}

		if value == "" {
			continue
		}

		envVars[varName] = value
	}

	return envVars
}

// getEnvVarsInternal はBitwardenから環境変数を取得する内部実装です。
// withSync が true の場合、取得前に bw sync を実行します。
func getEnvVarsInternal(withSync bool) (map[string]string, error) {
	defer debugTimerStart("GetEnvVars 全体")()

	if err := checkBitwardenSession(); err != nil {
		return nil, err
	}

	// --sync 指定時のみサーバーと同期（デフォルトはローカルキャッシュを使用）
	if withSync {
		if syncErr := syncFunc(); syncErr != nil {
			return nil, syncErr
		}
	}

	// env: プレフィックス付きの項目を検索
	done := debugTimerStart("bw list items --search env: (GetEnvVars)")
	cmd := exec.CommandContext(context.Background(), "bw", "list", "items", "--search", "env:")

	output, err := cmd.Output()

	done()

	if err != nil {
		return nil, fmt.Errorf("bw list items が失敗しました: %w", err)
	}

	var items []BitwardenItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("JSON のパースに失敗しました: %w", err)
	}

	envVars := buildEnvMapFromItems(items)

	if len(envVars) == 0 {
		return nil, fmt.Errorf("bitwarden に env: 項目が見つかりません")
	}

	return envVars, nil
}

// isValidEnvVarName は環境変数名が有効かどうかを検証します。
// 英字またはアンダースコアで始まり、英数字とアンダースコアのみを含む必要があります。
// 注意: export.go の IsValidExportKey はより厳格で、大文字のみを要求します。
// これはBitwardenからの読み込み時の検証なので、小文字も許可します。
func isValidEnvVarName(name string) bool {
	return regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name)
}

// getCustomFieldValue はカスタムフィールドから指定された名前の値を取得します。
func getCustomFieldValue(fields []BitwardenCustomField, name string) string {
	for _, field := range fields {
		if strings.EqualFold(field.Name, name) {
			return field.Value
		}
	}

	return ""
}

// getBitwardenStatus は現在のBitwardenステータスを取得します。
func getBitwardenStatus() (string, error) {
	defer debugTimerStart("bw status")()

	cmd := exec.CommandContext(context.Background(), "bw", "status")

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var status BitwardenStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return "", err
	}

	return status.Status, nil
}

// UnlockGetToken はBitwardenをアンロックしてセッショントークンを返します。
// 既にアンロック済みの場合は既存の BW_SESSION トークンをそのまま返します。
// 呼び出し元でトークンをシェルに設定する責務を持ちます（BW_SESSION の伝播）。
func UnlockGetToken() (string, error) {
	defer debugTimerStart("UnlockGetToken 全体")()

	// bwコマンドの存在確認
	if _, err := exec.LookPath("bw"); err != nil {
		return "", fmt.Errorf("bw コマンドが見つかりません。Bitwarden CLI をインストールしてください")
	}

	// 既にアンロック済みの場合は既存トークンを返す
	if existing := os.Getenv("BW_SESSION"); existing != "" {
		status, err := getBitwardenStatus()
		if err == nil && status == statusUnlocked {
			debugLog("既にアンロック済み → 既存トークンを返す")
			fmt.Fprintln(os.Stderr, "✅ Bitwarden は既にアンロックされています。")

			return existing, nil
		}

		fmt.Fprintln(os.Stderr, "BW_SESSION が設定されていますがロックされています。再アンロックします...")
	}

	// ログイン状態の確認
	done := debugTimerStart("bw login --check")

	if err := loginCheckFunc(); err != nil {
		done()

		return "", fmt.Errorf("bitwarden にログインしていません。'bw login' を実行してください")
	}

	done()

	// アンロック実行（マスターパスワードを対話入力）
	fmt.Fprintln(os.Stderr, "🔐 Bitwarden をアンロックしています...")

	token, err := unlockRawFunc()
	if err != nil {
		return "", err
	}

	if token == "" {
		return "", fmt.Errorf("bw unlock --raw の出力が空です")
	}

	// トークン形式の検証
	if !regexp.MustCompile(`^[A-Za-z0-9+/=._-]+$`).MatchString(token) {
		return "", fmt.Errorf("bw unlock --raw の出力形式が認識できません")
	}

	return token, nil
}
