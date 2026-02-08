# DevSync

DevSync は、開発環境の運用作業を統合・一元化するためのクロスプラットフォーム CLI ツールです。
既存の `sysup` および `Setup-Repository` を置き換え、Bitwarden を利用した環境変数注入の自動化を目指します。

## 🚀 目的

- **運用の統合**: リポジトリ管理、システム更新、セキュアな環境変数注入を単一の CLI に集約します。
- **技術スタックの刷新**: Go 言語を採用し、安定性、配布の容易さ、信頼性の高い並列実行を実現します。
- **体験の向上**: 日本語によるインタラクティブな設定ウィザードや、安定した並列制御を提供します。

## 🛠 技術スタック

- **言語**: Go
- **CLI フレームワーク**: Cobra
- **設定**: YAML + Viper (または独自実装)
- **インタラクティブ UI**: Survey (ウィザード) / Bubble Tea (TUI)
- **並列制御**: errgroup + semaphore

## 📦 インストール

### 前提ツール

- Go 1.25 以上
- `git`
- `gh` (GitHub CLI, `repo` 系運用時に推奨)
- `bw` (Bitwarden CLI, `env` / `run` 運用時に推奨)

### インストール方法（推奨: go install）

```bash
go install github.com/scottlz0310/devsync/cmd/devsync@latest
```

`$GOPATH/bin`（通常は `~/go/bin`）に `devsync` が配置されます。  
PATH未設定の場合は、シェルの設定ファイルに追加してください。

```bash
export PATH="$HOME/go/bin:$PATH"
```

### ローカルビルドで使う場合

```bash
git clone https://github.com/scottlz0310/devsync.git
cd devsync
go build -o dist/devsync ./cmd/devsync
./dist/devsync --help
```

### 初期設定

初回セットアップでは、まず設定ファイルを生成してください。
`config init` で指定した `repo.root` が未存在の場合は、作成確認が表示されます（拒否時はそのまま終了します）。

```bash
devsync config init
devsync doctor
```

### シェル連携（自動設定）

`devsync config init` では、シェル起動時に `~/.config/devsync/init.bash`（zshは `init.zsh`）を読み込む設定を
`~/.bashrc` / `~/.zshrc` に自動追記できます。

```bash
# 反映確認（bash）
grep -n ">>> devsync >>>" ~/.bashrc
source ~/.bashrc

# 関数確認
type devsync-load-env
type dev-sync
```

- `devsync-load-env`: Bitwarden の `env:` 項目を現在のシェルへ読み込み
- `dev-sync`: Bitwarden 解錠 → 環境変数を親シェルへ読み込み → `devsync run` 実行（引数はそのまま渡されます）

`devsync` バイナリの配置先を変更した場合は、`devsync config init` を再実行してシェル連携スクリプトを再生成してください。

## 📋 コマンド一覧

### メインコマンド
```
devsync run           # 日次の統合タスクを実行（Bitwarden解錠→環境変数読込→更新処理）
devsync doctor        # 依存ツール（git, bw等）と環境設定の診断
```

### システム更新 (`sys`)
```
devsync sys update    # パッケージマネージャで一括更新
devsync sys update -n # ドライラン（計画のみ表示）
devsync sys update -j 4 # 4並列で更新
devsync sys update --tui # Bubble Teaで進捗を表示
devsync sys list      # 利用可能なパッケージマネージャを一覧表示
```

**対応パッケージマネージャ**: apt, brew, go, npm, snap, pipx, cargo

`sys update` は `--jobs / -j` で並列数を指定できます（未指定時は `config.yaml` の `control.concurrency` を使用）。
`apt` はパッケージロック競合を避けるため、依存関係ルールとして単独実行されます。
`--tui` を指定すると、Bubble Tea ベースの進捗UI（マルチ進捗バー・リアルタイムログ・失敗ハイライト）を表示します。
`apt` / `snap` など sudo が必要な更新は、単独フェーズ・並列フェーズの開始前に `sudo -v` で事前認証を確認します。
`snapd unavailable` の環境では `snap` を利用不可として自動スキップします。
`sys.enable` に未インストールのマネージャが含まれている場合は、警告を表示してスキップし、利用可能なマネージャのみ継続実行します。

### リポジトリ管理 (`repo`)
```
devsync repo update       # 管理下リポジトリを更新（fetch + pull --rebase）
devsync repo update -j 4  # 4並列で更新
devsync repo update -n    # ドライラン（計画のみ表示）
devsync repo update --tui # Bubble Teaで進捗を表示
devsync repo update --submodule      # submodule更新を強制有効化（設定値を上書き）
devsync repo update --no-submodule   # submodule更新を強制無効化（設定値を上書き）
devsync repo list         # 管理下リポジトリの一覧と状態を表示
devsync repo list --root ~/src # ルートを上書きして一覧表示
```

`repo list` は `config.yaml` の `repo.root` 配下をスキャンし、状態を表示します。
状態は `クリーン` / `ダーティ` / `未プッシュ` / `追跡なし` です。
`repo update` は `fetch --all`、`pull --rebase`、必要に応じて `submodule update` を実行します。
submodule 更新の既定値は `config.yaml` の `repo.sync.submodule_update` で制御し、
CLI では `--submodule` / `--no-submodule` で明示的に上書きできます。
`--tui` 指定時は、更新の進捗・ログ・失敗状態をインタラクティブに表示します。

### 環境変数 (`env`)
```
devsync env export    # Bitwardenから環境変数をシェル形式でエクスポート
devsync env run       # 環境変数を注入してコマンドを実行
```

### 設定管理 (`config`)
```
devsync config init       # 対話形式のウィザードで設定ファイルを生成
devsync config uninstall  # シェル設定からdevsyncを削除
```

### 予定機能
- `devsync repo cleanup`: マージ済みブランチの整理（未実装）

## 🧪 日常運用（マニュアルテスト手順）

日次運用を想定した、実行順の確認手順です。

### 0. 初回のみ: 設定ファイルを生成（必須）

```bash
devsync config init
```

`repo list` / `repo update` は `repo.root` 設定を利用するため、初回は先に `config init` を実行してください。

### 1. 依存関係と設定の確認

```bash
devsync doctor
devsync sys list
devsync repo list
```

### 2. Dry-run で計画確認（本実行前）

```bash
devsync sys update -n --tui
devsync repo update -n --tui
```

非TTY環境（CIやリダイレクト実行）では、`--tui` は通常表示へ自動フォールバックします。

### 3. `run` の環境変数注入セクション確認

```bash
# シェル連携が有効なことを確認
source ~/.bashrc
type dev-sync

# devsync run の先頭ジョブ（Bitwarden unlock / 環境変数注入）を確認
dev-sync
# もしくはサブプロセス実行のみを確認
devsync run
```

`dev-sync` は最初に Bitwarden のアンロックと環境変数注入を実行し、親シェルにも環境変数を反映したうえで `devsync run` を実行します。
`devsync run` 単体で実行した場合は、サブプロセス内のみ環境変数が注入されます。
この時点では `sys` / `repo` の統合処理は段階的実装中のため、プレースホルダー表示になる場合があります。

### 4. 本実行（通常運用）

```bash
devsync sys update --tui -j 4
devsync repo update --tui -j 4
```

### 5. 結果確認

- 失敗件数が 0 か
- スキップ理由が想定どおりか（キャンセル/タイムアウトなど）
- 必要に応じて `--verbose` で詳細ログを再確認

## 🔑 環境変数の使用

### 方法1: シェルに環境変数を読み込む（eval）

Bitwardenから環境変数を現在のシェルに読み込むには：

```bash
# Bitwardenから環境変数をエクスポート
eval "$(devsync env export)"

# 確認
echo $GPAT
```

**PowerShell:**
```powershell
& devsync env export | Invoke-Expression
```

### 方法2: サブプロセスに環境変数を注入する（推奨）

`eval` を使わずに安全にコマンドを実行できます：

```bash
# 環境変数を注入してコマンドを実行
devsync env run npm run build
devsync env run go test ./...
```

この方法は以下の利点があります：
- `eval` のリスクを回避
- コマンドの終了コードを保持
- 親シェルに影響を与えない

**注意**: `devsync run` 単体では親シェルに環境変数は反映されません。親シェルでも利用したい場合は `eval "$(devsync env export)"` または `devsync-load-env` / `dev-sync` を使用してください。
## 🛠 開発

### 前提条件

開発には [Task](https://taskfile.dev/) (go-task) を使用します。

**インストール:**
```bash
# Go
go install github.com/go-task/task/v3/cmd/task@latest

# Homebrew (macOS/Linux)
brew install go-task

# Scoop (Windows)
scoop install task

# Chocolatey (Windows)
choco install go-task
```

### 開発コマンド

```bash
task --list      # 利用可能なタスク一覧

# 日常運用（まずこれ）
task check       # 標準品質チェック（fmt → vet → test → lint）
task daily       # task check のエイリアス

# その他よく使うコマンド
task build       # バイナリをビルド（dist/に出力）
task test        # テスト実行
task lint        # リンター実行
task fmt         # コードフォーマット
task dev         # 開発サイクル（fmt → test → build）
task pre-commit  # コミット前チェック
task clean       # ビルド成果物を削除
task tidy        # go mod tidy
```

### 品質基準

- **カバレッジ閾値**: 30%（段階的に引き上げ予定）
- **リンター**: golangci-lint（`.golangci.yml` で設定）
- **静的解析**: go vet

## 📅 ステータス

現在 **v0.1 計画 / 初期開発** フェーズです。
詳細なロードマップについては [docs/Implementation_Plan.md](docs/Implementation_Plan.md) を参照してください。

## 📄 ライセンス

[LICENSE](LICENSE) を参照してください。
