package secret

import (
	"bytes"
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

// bwLookPathFunc はテストで差し替え可能な bw コマンド存在確認の関数変数です。
var bwLookPathFunc = func() error {
	_, err := exec.LookPath("bw")
	return err
}

// statusFunc はテストで差し替え可能な bw status 実行の関数変数です。
var statusFunc = getBitwardenStatus

// listEnvItemsFunc はテストで差し替え可能な env: 項目取得の関数変数です。
var listEnvItemsFunc = listBitwardenEnvItems

// stdinIsTerminalFunc はテストで差し替え可能な標準入力TTY判定です。
var stdinIsTerminalFunc = func() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

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

	// アップデート通知等が stdout に混入する場合があるため、
	// 最後の非空行のみをセッショントークンとして使用する（PowerShell 側と同様）
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line, nil
		}
	}

	return "", nil
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

// listBitwardenEnvItems はBitwardenからenv:プレフィックス付きの項目を取得します。
func listBitwardenEnvItems() ([]BitwardenItem, error) {
	defer debugTimerStart("bw list items --search env:")()

	cmd := exec.CommandContext(context.Background(), "bw", "list", "items", "--search", "env:")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items が失敗しました: %w", err)
	}

	// アップデート通知等が stdout に混入する場合を考慮して、
	// '[' 以降の JSON 配列部分のみを解析する
	if idx := bytes.IndexByte(output, '['); idx > 0 {
		output = output[idx:]
	}

	var items []BitwardenItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("JSON のパースに失敗しました: %w", err)
	}

	return items, nil
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

// EnsureBitwardenSession は bw CLI のセッションを利用可能な状態にします。
// BW_SESSION が未設定またはロック済みの場合は対話的に再アンロックし、
// 現在のプロセス環境へ新しい BW_SESSION を設定します。
func EnsureBitwardenSession() (string, error) {
	if err := bwLookPathFunc(); err != nil {
		return "", fmt.Errorf("bw コマンドが見つかりません。Bitwarden CLI をインストールしてください")
	}

	existing := os.Getenv("BW_SESSION")
	if existing != "" {
		status, err := statusFunc()
		if err == nil && status == statusUnlocked {
			debugLog("既にアンロック済み → 既存トークンを返す")

			return existing, nil
		}

		fmt.Fprintln(os.Stderr, "BW_SESSION が設定されていますがロックされています。再アンロックします...")
	}

	if !stdinIsTerminalFunc() {
		if existing == "" {
			return "", fmt.Errorf("BW_SESSION が設定されていません。非対話環境では事前に Bitwarden をアンロックし、BW_SESSION を設定してください")
		}

		return "", fmt.Errorf("BW_SESSION がロックされています。非対話環境では対話的に再アンロックできません。事前に BW_SESSION を更新してください")
	}

	token, err := unlockBitwardenToken()
	if err != nil {
		return "", err
	}

	if err := os.Setenv("BW_SESSION", token); err != nil {
		return "", fmt.Errorf("BW_SESSION の設定に失敗しました: %w", err)
	}

	return token, nil
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

	if _, err := EnsureBitwardenSession(); err != nil {
		return nil, err
	}

	// --sync 指定時のみサーバーと同期（デフォルトはローカルキャッシュを使用）
	if withSync {
		if syncErr := syncFunc(); syncErr != nil {
			return nil, syncErr
		}
	}

	items, err := listEnvItemsFunc()
	if err != nil {
		return nil, err
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

	// アップデート通知等が stdout に混入する場合を考慮して、
	// '{' 以降の JSON オブジェクト部分のみを解析する
	if idx := bytes.IndexByte(output, '{'); idx > 0 {
		output = output[idx:]
	}

	var status BitwardenStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return "", err
	}

	return status.Status, nil
}

// GetBitwardenSessionStatus は現在の Bitwarden セッション状態を返します。
func GetBitwardenSessionStatus() (string, error) {
	if err := bwLookPathFunc(); err != nil {
		return "", fmt.Errorf("bw コマンドが見つかりません")
	}

	if os.Getenv("BW_SESSION") == "" {
		return "missing", nil
	}

	status, err := statusFunc()
	if err != nil {
		return "", fmt.Errorf("bitwarden のステータス確認に失敗しました: %w", err)
	}

	return status, nil
}

// UnlockGetToken はBitwardenをアンロックしてセッショントークンを返します。
// 既にアンロック済みの場合は既存の BW_SESSION トークンをそのまま返します。
// 呼び出し元でトークンをシェルに設定する責務を持ちます（BW_SESSION の伝播）。
func UnlockGetToken() (string, error) {
	defer debugTimerStart("UnlockGetToken 全体")()

	// bwコマンドの存在確認
	if err := bwLookPathFunc(); err != nil {
		return "", fmt.Errorf("bw コマンドが見つかりません。Bitwarden CLI をインストールしてください")
	}

	// 既にアンロック済みの場合は既存トークンを返す
	if existing := os.Getenv("BW_SESSION"); existing != "" {
		status, err := statusFunc()
		if err == nil && status == statusUnlocked {
			debugLog("既にアンロック済み → 既存トークンを返す")
			fmt.Fprintln(os.Stderr, "✅ Bitwarden は既にアンロックされています。")

			return existing, nil
		}

		fmt.Fprintln(os.Stderr, "BW_SESSION が設定されていますがロックされています。再アンロックします...")
	}

	return unlockBitwardenToken()
}

func unlockBitwardenToken() (string, error) {
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

	if !isValidSessionToken(token) {
		return "", fmt.Errorf("bw unlock --raw の出力形式が認識できません")
	}

	return token, nil
}

func isValidSessionToken(token string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9+/=._-]+$`).MatchString(token)
}
