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
`gh auth login` 済みの環境では、`config init` の GitHub オーナー名入力が自動補完されます（必要に応じて組織名へ変更可能）。
既存の `config.yaml` がある場合、`config init` は現在値を初期値として再編集できます。

```bash
devsync config init
devsync doctor
```

### シェル連携（自動設定）

`devsync config init` では、シェル起動時に `~/.config/devsync/init.bash`（zshは `init.zsh`）を読み込む設定を
`~/.bashrc` / `~/.zshrc` に自動追記できます。

PowerShell の場合は、`~/.config/devsync/init.ps1` を `$PROFILE`（例: `Microsoft.PowerShell_profile.ps1`）に自動追記できます。
反映するには `. $PROFILE` を実行してください。

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

## 🗑 アンインストール

`devsync config uninstall` は **シェル設定から devsync のマーカーブロックを削除するだけ** です（バイナリや設定ファイルは削除しません）。

### 1. シェル連携の解除（推奨）

`config init` が追記した `# >>> devsync >>>` / `# <<< devsync <<<` のブロックを削除します。

```bash
devsync config uninstall
```

注意:
- `devsync config uninstall` は「いま実行しているシェル」を自動判定して削除します。bash/zsh/PowerShell それぞれで解除したい場合は、そのシェルから実行してください。
- 解除後はシェルを再起動するか、設定を再読み込みしてください（例: bash/zsh は `source ~/.bashrc`、PowerShell は `. $PROFILE`）。

### 2. 設定ファイル・生成ファイルの削除（任意）

- 設定ファイル: `~/.config/devsync/config.yaml`
- シェル連携スクリプト: `~/.config/devsync/init.bash` / `init.zsh` / `init.ps1`

完全に削除する場合は、`~/.config/devsync` ディレクトリごと削除してください。

```bash
rm -rf ~/.config/devsync
```

**PowerShell:**
```powershell
Remove-Item -Recurse -Force (Join-Path $HOME '.config/devsync') -ErrorAction SilentlyContinue
```

### 3. devsync バイナリの削除

#### go install でインストールした場合

`devsync` の実体パスを確認して削除します。

```bash
command -v devsync
rm -f "$(command -v devsync)"
```

**PowerShell:**
```powershell
(Get-Command devsync).Source
Remove-Item -Force (Get-Command devsync).Source
```

補足:
- `devsync` が PATH 上で見つからない場合は、通常 `$(go env GOPATH)/bin`（または `go env GOBIN` が設定されている場合はそのディレクトリ）にあります。

#### ローカルビルドで使っていた場合

`dist/devsync`（または配置先のバイナリ）を削除してください。

## 📋 コマンド一覧

### メインコマンド
```
devsync --version      # バージョン表示（現在: v0.1.0-alpha）
devsync run           # 日次の統合タスクを実行（Bitwarden解錠→環境変数読込→更新処理）
devsync doctor        # 依存ツール（git, bw等）と環境設定の診断
```

### システム更新 (`sys`)
```
devsync sys update    # パッケージマネージャで一括更新
devsync sys update -n # ドライラン（計画のみ表示）
devsync sys update -j 4 # 4並列で更新
devsync sys update --tui # Bubble Teaで進捗を表示
devsync sys update --no-tui # TUIを無効化（設定より優先）
devsync sys list      # 利用可能なパッケージマネージャを一覧表示
```

**対応パッケージマネージャ**: apt, brew, go, npm, pnpm, nvm, snap, flatpak, fwupdmgr, pipx, cargo, uv, rustup, gem

`sys update` は `--jobs / -j` で並列数を指定できます（未指定時は `config.yaml` の `control.concurrency` を使用）。
`apt` はパッケージロック競合を避けるため、依存関係ルールとして単独実行されます。
`ui.tui=true` を設定すると、`--tui` なしでも Bubble Tea ベースの進捗UI（マルチ進捗バー・リアルタイムログ・失敗ハイライト）を既定で有効化できます。
コマンド単位で上書きしたい場合は `--tui` / `--no-tui` を使用します。
`apt` / `snap` など sudo が必要な更新は、単独フェーズ・並列フェーズの開始前に `sudo -v` で事前認証を確認します。
`snapd unavailable` の環境では `snap` を利用不可として自動スキップします。
`sys.enable` に未インストールのマネージャが含まれている場合は、警告を表示してスキップし、利用可能なマネージャのみ継続実行します。

### リポジトリ管理 (`repo`)
```
devsync repo update       # 管理下リポジトリを更新（fetch + pull --rebase）
devsync repo update -j 4  # 4並列で更新
devsync repo update -n    # ドライラン（計画のみ表示）
devsync repo update --tui # Bubble Teaで進捗を表示
devsync repo update --no-tui # TUIを無効化（設定より優先）
devsync repo update --submodule      # submodule更新を強制有効化（設定値を上書き）
devsync repo update --no-submodule   # submodule更新を強制無効化（設定値を上書き）
devsync repo list         # 管理下リポジトリの一覧と状態を表示
devsync repo list --root ~/src # ルートを上書きして一覧表示
devsync repo cleanup      # マージ済みローカルブランチを整理
devsync repo cleanup -n   # DryRun（削除計画のみ表示）
```

`repo list` は `config.yaml` の `repo.root` 配下をスキャンし、状態を表示します。
状態は `クリーン` / `ダーティ` / `未プッシュ` / `追跡なし` です。
`repo update` は `fetch --all`、`pull --rebase`、必要に応じて `submodule update` を実行します。
`repo.github.owner` が設定されている場合は、GitHub 一覧との差分を確認し、
`repo.root` 配下で不足しているリポジトリを `git clone` してから更新を継続します（`-n/--dry-run` 時は clone 計画のみ表示）。
submodule 更新の既定値は `config.yaml` の `repo.sync.submodule_update` で制御し、
CLI では `--submodule` / `--no-submodule` で明示的に上書きできます。
`ui.tui=true` の場合は `--tui` なしでも、更新の進捗・ログ・失敗状態をインタラクティブに表示します。
コマンド単位で上書きしたい場合は `--tui` / `--no-tui` を使用します。

`repo cleanup` はマージ済みローカルブランチの削除を行います（安全側優先）。
`repo.cleanup.target` に `merged` / `squashed` を設定できます。
`merged` は git のマージ判定（`--merged`）に基づき、通常削除（`git branch -d`）します。
`squashed` は GitHub の PR 情報に基づき「PR は merged だが git 的には未マージ」なブランチを強制削除（`git branch -D`）します。
このとき **PR の head commit とローカルブランチ先頭コミットが一致する場合のみ** 削除対象にします（安全側のため）。
削除計画の精度を担保するため、DryRun（`-n/--dry-run`）でも `git fetch --all` は実行します（ただし DryRun 時は `--prune` を無効化します）。

### 環境変数 (`env`)
```
devsync env export    # Bitwardenから環境変数をシェル形式でエクスポート
devsync env run       # 環境変数を注入してコマンドを実行
```

### 設定管理 (`config`)
```
devsync config init       # 対話形式のウィザードで設定ファイルを生成
devsync config show       # 現在の設定を表示（YAML）
devsync config validate   # 設定内容を検証
devsync config uninstall  # シェル設定からdevsyncを削除
```

## 🚧 Alpha リリース方針（v0.1.0-alpha）

本バージョンは **運用検証向け Alpha** です。安定化期間中は `setup-repo` との併用を推奨します。

### 推奨運用（当面）

- リポジトリ同期は `devsync repo update` と `setup-repo` を並行利用し、差分がないことを確認しながら移行する
- `repo.root` は `~/workspace` または `~/src` のどちらか一方に固定して運用する
- `devsync run` は `env -> sys update -> repo update` の実処理フローを実行し、日常運用の確認に使う

### 既知の制約

- `repo` 系は安全側優先のため、運用状態によっては更新をスキップする（例: upstream 未設定など）
- `repo update` は未コミット変更/stash/detached HEAD/デフォルトブランチ以外の追跡を検出した場合は安全側にスキップする（必要に応じてデフォルトブランチをチェックアウトして再実行してください）
- `sys` パッケージマネージャ対応は `sysup` 同等以上へ拡張中（追加対応は `tasks.md` 管理）

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

非TTY環境（CIやリダイレクト実行）では、`--tui` / `ui.tui=true` による TUI 指定は通常表示へ自動フォールバックします。

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
`devsync run` では続けて `sys update` と `repo update` を順次実行します。

### 4. 本実行（通常運用）

```bash
devsync sys update --tui -j 4
devsync repo update --tui -j 4
```

### 5. 結果確認

- 失敗件数が 0 か
- スキップ理由が想定どおりか（キャンセル/タイムアウトなど）
- 必要に応じて `--verbose` で詳細ログを再確認

### 6. `setup-repo` との比較（移行期間の手動チェック）

`repo` 系は安全側優先のため、状態によっては `devsync repo update` がスキップすることがあります。
移行期間中は `setup-repo` でも同期を実行し、結果の差分がないことを確認してください。

- 実行順（例）:
  - `devsync repo update --tui -j 4`
  - `setup-repo sync`
- 確認観点:
  - スキップが出た場合、理由に応じた復旧ができること（例: デフォルトブランチへ戻す / stash を解消する / `git remote set-head <remote> -a` を実行する）
  - `devsync repo list` のステータスが想定どおりであること
  - `git status` が想定どおりであること
  - サブモジュールを利用している場合、`git submodule status` が想定どおりであること

## 🧯 トラブル時の復旧手順

問題が出た場合は、まず次の順で復旧してください。

1. 設定再生成: `devsync config init`
2. 設定確認: `devsync doctor`
3. `repo.root` 見直し: `~/.config/devsync/config.yaml` の `repo.root` が実在し、運用対象と一致しているか確認
4. Dry-run 再確認:
   - `devsync sys update -n --tui`
   - `devsync repo update -n --tui`
5. 必要に応じて `setup-repo` で整合を取り、再度 `devsync repo update` を実行
6. GitHub のレート制限（`429 Too Many Requests` / `secondary rate limit`）が出る場合:
   - 数十秒〜数分待ってから再実行
   - `repo cleanup` で頻発する場合は並列数を下げる（例: `devsync repo cleanup -j 1`）
   - どうしても復旧できない場合は `repo.cleanup.target` から `squashed` を外し、`merged` のみで運用（GitHub API 呼び出しを抑制）

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

`devsync-load-env` / `dev-sync` 利用時に `Cannot convert 'System.Object[]' to the type 'System.String'` が出る場合は、
旧版のシェル連携スクリプトが残っているため `devsync config init` を再実行して `init.ps1` を再生成してください。

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
task secrets:install # gitleaks をインストール（初回のみ）
task secrets     # gitleaks でシークレット混入をチェック
task clean       # ビルド成果物を削除
task tidy        # go mod tidy
```

### Windows で `task lint` が gofmt で落ちる場合

Windows で Git の `core.autocrlf=true` を使っていると、`.go` ファイルが CRLF でチェックアウトされることがあり、
`task lint`（gofmt チェック）が差分扱いになって失敗します。

このリポジトリは `.gitattributes` で Go 関連ファイルを LF に固定していますが、**既存のクローン**は一度再チェックアウトが必要な場合があります。

作業ツリーがクリーンな状態で、以下を実行してください：

```powershell
git status --porcelain
git checkout -f -- .
task lint
```

### 品質基準

- **カバレッジ閾値**: 30%（段階的に引き上げ予定）
- **リンター**: golangci-lint（`.golangci.yml` で設定）
- **静的解析**: go vet

## 📅 ステータス

現在 **v0.1.0-alpha（運用検証フェーズ）** です。
詳細なロードマップについては [docs/Implementation_Plan.md](docs/Implementation_Plan.md) を参照してください。

## 📄 ライセンス

[LICENSE](LICENSE) を参照してください。
