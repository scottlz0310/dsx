# コントリビューションガイド

dsx へのコントリビューションに興味を持っていただきありがとうございます！

## 開発環境のセットアップ

### 必要ツール

| ツール | 用途 | インストール |
|--------|------|--------------|
| Go 1.26 以上 | 言語ランタイム | [go.dev/dl](https://go.dev/dl/) |
| [Task](https://taskfile.dev/) | タスクランナー | `go install github.com/go-task/task/v3/cmd/task@latest` |
| [golangci-lint](https://golangci-lint.run/) | 静的解析 | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` |
| [lefthook](https://github.com/evilmartians/lefthook) | Git フック管理 | `go install github.com/evilmartians/lefthook@latest` |
| [gitleaks](https://github.com/gitleaks/gitleaks) | シークレット検出 | `task secrets:install` |

### 初期セットアップ

```bash
git clone https://github.com/scottlz0310/dsx.git
cd dsx

# 依存関係の解決
go mod tidy

# Git フックを有効化（コミット前 fmt チェック、プッシュ前 lint/test）
lefthook install

# 動作確認
task check
```

## 開発ワークフロー

### ブランチ作成

`main` への直接コミットは禁止です。必ずブランチを切って PR を作成してください。

```bash
git checkout -b feat/your-feature-name
# または
git checkout -b fix/your-bug-fix
```

### 日常の開発サイクル

```bash
task fmt      # コードフォーマット（gofmt -s -w .）
task test     # 全テスト実行（shuffle=on）
task build    # dist/ にバイナリを出力
task check    # CI 相当の一括チェック（fmt → vet → test → lint）
```

### コミット

[Conventional Commits](https://www.conventionalcommits.org/) に従ってください。

```bash
git commit -m "feat: repo list に Behind 列を追加"
git commit -m "fix: refs/remotes/origin/HEAD 未設定時のスキップを廃止"
git commit -m "docs: CONTRIBUTING.md を Go 実装に合わせて更新"
git commit -m "test: updater パッケージのテーブル駆動テストを追加"
git commit -m "chore(deps): golangci-lint を v2 に更新"
```

コミットメッセージ・PR 説明は**日本語**で記述してください（`.github/copilot-instructions.md` 参照）。

### PR の作成

```bash
git push -u origin your-branch-name
```

GitHub で PR を作成し、変更内容・動作確認結果を説明してください。

## コードスタイル

### フォーマット

```bash
task fmt          # gofmt -s -w . で自動整形
task fmt:check    # CI 用フォーマットチェック
task vet          # go vet ./cmd/... ./internal/...
task lint         # golangci-lint run
```

lefthook を有効化すると、コミット前に `gofmt` チェック、プッシュ前に `golangci-lint` と `go test` が自動実行されます。

### コーディング規約

- **エラーハンドリング**: コマンド実行エラーは `buildCommandOutputErr(err, output)` でラップして出力を診断に含める
- **言語**: CLI/TUI の出力（ログ、エラー、プロンプト）・コメント・ドキュメントはすべて日本語
- **インターフェース準拠**: `var _ Updater = (*XxxUpdater)(nil)` を追加してコンパイル時検証

## テスト

### テストの実行

```bash
# 全テスト
task test

# 詳細出力
task test:v

# 特定パッケージ
go test ./internal/updater/... -v

# 特定テスト関数
go test ./internal/updater/... -run TestAptUpdate -v

# カバレッジレポート生成（coverage.html に出力）
task coverage
```

### テストの書き方

- **テーブル駆動テスト**を基本とし、テストケース名は日本語
- **境界値・エラー系を重視** — 正常系のみのテストは禁止
- モックより fake / in-memory 実装を優先
- `testutil.SetTestHome(t, path)` で HOME ディレクトリを分離（Windows 互換）

```go
func TestProcessItems(t *testing.T) {
    tests := []struct {
        name    string
        input   []string
        want    []string
        wantErr bool
    }{
        {
            name:  "通常の入力を正しく処理できる",
            input: []string{"foo", "bar"},
            want:  []string{"foo", "bar"},
        },
        {
            name:    "空リストはエラーを返す",
            input:   []string{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ProcessItems(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ProcessItems() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr {
                require.Equal(t, tt.want, got)
            }
        })
    }
}
```

## アーキテクチャ

```
cmd/dsx/         CLI エントリポイント（Cobra コマンド定義・ジョブ実行）
internal/
  config/        YAML 設定の読み込み・保存・バリデーション（Viper）
  updater/       パッケージマネージャ更新（Updater インターフェース + レジストリ）
  repo/          Git リポジトリ操作（update / cleanup / list）
  secret/        Bitwarden 連携・環境変数注入
  tui/           Bubble Tea ベースの進捗 UI
  env/           環境変数ユーティリティ
  runner/        並列ジョブ実行エンジン
  testutil/      テスト用ヘルパー
```

### 新しい Updater の追加

1. `internal/updater/<name>.go` に `Updater` インターフェースを実装
2. `init()` 内で `Register(NewXxxUpdater())` を呼んで自動登録
3. `var _ Updater = (*XxxUpdater)(nil)` でインターフェース準拠をコンパイル時検証
4. エラーは `buildCommandOutputErr(err, output)` でラップ
5. 出力解析が必要なコマンドは `runCommandOutputWithLocaleC()` で `LANG=C` 固定実行

## セキュリティ

### シークレット検出

```bash
# gitleaks をインストール
task secrets:install

# シークレット混入チェック
task secrets
```

### セキュリティガイドライン

- API キー・パスワードなどの秘密情報を絶対にコミットしない
- 機密設定は環境変数または Bitwarden 経由で管理する
- セキュリティ関連の変更は PR で明記する

## バグ報告

Issue を作成する際は以下を含めてください。

- OS・Go バージョン（`go version`）
- `dsx --version` の出力
- 再現手順
- 期待動作 vs 実際の動作
- エラーメッセージ・ログ

## 機能リクエスト

Issue を作成する際は以下を説明してください。

- 解決したい問題
- なぜこの機能が有用か
- 利用イメージ・使用例
- プロジェクトのスコープへの適合性

## PR チェックリスト

PR 提出前に確認してください。

- [ ] `task check` がすべてパスする（fmt / vet / test / lint）
- [ ] 新機能にはテストを追加した
- [ ] `CHANGELOG.md` の `[Unreleased]` セクションを更新した
- [ ] ドキュメント・コメントを日本語で記述した
- [ ] コミットメッセージが Conventional Commits に従っている
- [ ] シークレット・個人情報が含まれていない

## コードレビュープロセス

1. **自動 CI**: すべての PR は GitHub Actions の CI（lint / test / build）を通過する必要があります
2. **レビュー**: メンテナー 1 名以上のレビューが必要です
3. **テスト**: 新機能にはテストが必要です

## リリースプロセス

1. `CHANGELOG.md` と `README.md` のバージョン記載を更新
2. `release/vX.Y.Z` ブランチを作成してリリース PR を開く
3. リリース PR を `main` にマージ
4. `vX.Y.Z` タグを作成・プッシュ
5. GitHub Actions + GoReleaser が自動でリリースアーティファクトを公開

## 参考ドキュメント

- `docs/Implementation_Plan.md` — ロードマップ・アーキテクチャ設計
- `docs/Dev_Tools.md` — 開発補助スクリプト（branch-chk 等）
- `docs/Legacy_Migration_Analysis.md` — 旧ツールからの移行要件
- `tasks.md` — タスク進捗管理
- `CHANGELOG.md` — 変更履歴

## 問い合わせ

- **GitHub Discussions**: 質問・一般的な議論
- **GitHub Issues**: バグ報告・機能リクエスト
- **SECURITY.md**: セキュリティ脆弱性の報告手順
