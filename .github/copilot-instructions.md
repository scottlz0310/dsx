# Copilot Instructions

## 言語規定

- **すべての応答・コメント・ドキュメント・UI メッセージは日本語**で記述すること
- CLI/TUI の出力（ログ、エラー、プロンプト）も日本語で実装する
- コミットメッセージ・PR 説明も日本語

## ビルド・テスト・リント

[Task](https://taskfile.dev/) (go-task) をタスクランナーとして使用。

```bash
task check          # 標準品質チェック（fmt → vet → test → lint）
task build          # dist/ にバイナリを出力
task test           # 全テスト実行（shuffle=on）
task lint           # golangci-lint
task fmt            # gofmt -s -w .
```

単一テストの実行:

```bash
go test ./internal/updater/... -run TestAptUpdate -v
go test ./cmd/dsx/... -run TestDoctor -v
```

## アーキテクチャ

### ディレクトリ構成

```
cmd/dsx/       CLI エントリポイント（Cobra コマンド定義・ジョブ実行）
internal/
  config/          YAML 設定の読み込み・保存・バリデーション（Viper）
  updater/         パッケージマネージャ更新（Updater インターフェース + レジストリ）
  repo/            Git リポジトリ操作（update / cleanup / list）
  secret/          Bitwarden 連携・環境変数注入
  tui/             Bubble Tea ベースの進捗 UI
  env/             環境変数ユーティリティ
  runner/          並列ジョブ実行エンジン
  testutil/        テスト用ヘルパー
```

### Updater レジストリパターン

各パッケージマネージャ（apt, brew, cargo 等）は `Updater` インターフェースを実装し、`init()` で `Register()` を呼んで自動登録する。新しいマネージャを追加する場合:

1. `internal/updater/<name>.go` に `Updater` インターフェースを実装
2. `init()` 内で `Register(NewXxxUpdater())` を呼ぶ
3. （推奨）`var _ Updater = (*XxxUpdater)(nil)` を追加してインターフェース準拠を明示する
4. エラーは `buildCommandOutputErr(err, output)` でラップ
5. 出力解析が必要なコマンドは `runCommandOutputWithLocaleC()` で `LANG=C` 固定実行

### GitHub API 呼び出し

`gh` コマンドは `runGhOutputWithRetry()` 経由で実行する。同時実行数 1 のセマフォ + レート制限/一時障害時のエクスポネンシャルバックオフが組み込まれている。

### TUI / 非 TUI 分岐

`ui.tui` 設定と `--tui` / `--no-tui` フラグで切り替え。TUI 有効時は `progressui.RunJobProgress()` でジョブを実行し、無効時は直接 `runner.Execute()` を使う。TUI 開始前の stdout メッセージは `if !useTUI` で抑制する。

## コーディング規約

### エラーハンドリング

- コマンド実行エラーは `buildCommandOutputErr(err, output)` でラップし、出力を診断に含める
- stdout/stderr 両方ある場合は `combineCommandOutputs(stdout, stderr)` で結合

### テスト

- **Table-driven tests** を基本とし、テストケース名は日本語
- **境界値・エラー系を重視** — 正常系のみのテストは禁止
- モックより fake / in-memory を優先
- `testutil.SetTestHome(t, path)` で HOME を分離（Windows 互換）

### ブランチ運用

- `main` ブランチへの直接コミット禁止 — 必ずブランチを切って PR を作成
- `.gitattributes` で Go ファイルは LF 改行を強制

## 開発ドキュメント

設計判断時は以下を参照:

- `docs/Implementation_Plan.md` — ロードマップ・アーキテクチャ設計
- `docs/Legacy_Migration_Analysis.md` — 旧ツール（sysup, Setup-Repository）からの移行要件
- `tasks.md` — タスク進捗管理（完了時にチェックを更新）
- `CHANGELOG.md` — 変更履歴（機能追加・修正時に更新）
