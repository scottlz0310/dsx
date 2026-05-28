# Changelog

このプロジェクトのすべての重要な変更はこのファイルに記録されます。

フォーマットは [Keep a Changelog](https://keepachangelog.com/ja/1.0.0/) に基づいています。

## [Unreleased]

### Performance

- `dsx sys update` の cargo 更新処理で `cargo install-update -a` の出力をパースして実際に更新されたパッケージ数を `UpdatedCount` に反映。「更新成功: N 件」の集計が正確になった（#74）
- `dsx sys update` の cargo 更新処理で `cargo install --list` の事前実行を省略。非 DryRun 時は `cargo install-update -a` を直接実行するよう変更し、起動時の余分な CLI 呼び出しを削減（#74）
- `cargo install-update -a` 実行時の `cmd.Stdin` 接続を削除。非対話コマンドへの不要な stdin 転送を除去（#74）

### Fixed

- `dsx sys update` の cargo 更新処理で `cargo-update v20+` の API 変更（`cargo-install-update -a` → `cargo-install-update install-update -a`）に対応。出力サマリ形式の変更（`Updated N...` → `Overall updated N...`）にも追従し、v19 との後方互換を維持
- `dsx sys update` の cargo 更新処理で `cargo-update` が未インストールの場合に全パッケージを `cargo install --force` で強制再ビルドしていた問題を修正。`cargo-update` がなければ自動インストールし、`cargo install-update -a`（更新必要分のみ再ビルド）を使用するよう変更（#74）

## [v0.6.3] - 2026-05-20

### Fixed

- `runBWUnlockRaw` (Go 側) が `bw unlock --raw` のマルチライン出力（アップデート通知等が stdout に混入する場合）を正しく処理できず、無効な `BW_SESSION` が設定される問題を修正（最後の非空行のみをセッショントークンとして使用するよう変更。PowerShell `dsx-unlock` の PR #70 修正と同様のパターンを Go 側にも適用）
- `getBitwardenStatus` が `bw status` の stdout にアップデート通知が混入した場合に JSON パースに失敗し、有効な `BW_SESSION` があるにもかかわらず「ロック済み」と誤判断して再アンロックを引き起こす問題を修正（`{` 以降の JSON 部分のみを解析）
- `listBitwardenEnvItems` が `bw list items` の stdout にアップデート通知が混入した場合に JSON パースに失敗する問題を修正（`[` 以降の JSON 部分のみを解析）

## [v0.6.2] - 2026-05-19

### Fixed

- `dsx env export` が `EnsureBitwardenSession()` を二重呼び出しし、パスワード入力を2回要求してしまう問題を修正（`getEnvVarsFunc()` が内部で `EnsureBitwardenSession()` を呼び出すため、`runEnvExport` からの直接呼び出しを排除）
- PowerShell `dsx-unlock` が `bw unlock --raw` のマルチライン出力（アップデート通知等）を正しく処理できず `$env:BW_SESSION` に無効な値が設定される問題を修正（最後の非空行のみをセッショントークンとして使用するよう変更）
- `dsx config init` が生成する `init.ps1` の `dsx-unlock` に同上の修正を適用
- インストール済み `~/.config/dsx/init.ps1` を即時修正（再実行不要）

## [v0.6.1] - 2026-05-19

### Fixed

- `dsx env export` / `dsx env run` が、未設定またはロック済みの `BW_SESSION` を検知した場合に Bitwarden を再アンロックして処理を継続するよう修正
- `dsx env export` が再アンロック後の `BW_SESSION` もシェル向け出力に含め、`Invoke-Expression` / `eval` 後の親シェルへ反映されるよう修正
- `dsx env status --quiet` を追加し、`dsx config init` が生成する `dsx-env` / `dsx-unlock` シェル関数の Bitwarden 状態判定を Go 側の JSON パースに集約
- 非対話環境では `dsx env export` / `dsx env run` が対話的な再アンロックを試みず、事前の `BW_SESSION` 設定を促すエラーを返すよう修正

## [v0.6.0] - 2026-05-15

### Added

- `dsx repo branch-clean` サブコマンドを追加（Issue #1）
  - 4カテゴリ（MERGED / UNMERGED / STALE-REF / NO-UPSTREAM）のブランチを検出
  - インタラクティブ（デフォルト）/ `--dry-run` / `--yes` の3モード対応
  - `survey/v2` による MultiSelect UI（MERGED・STALE-REF をデフォルト選択）
  - `--exclude` で除外ブランチを指定可能、`--no-fetch` で事前 fetch をスキップ可能
  - `--force` フラグを追加（指定時のみ UNMERGED/NO-UPSTREAM を `git branch -D` で強制削除）
  - インタラクティブモードで MultiSelect 確定後に `[y/N]` 最終確認プロンプトを表示（PR #66 レビュー対応）
  - 削除エラー時に終了コード非ゼロ（PR #66 レビュー対応）

### Changed

- `branch-clean` の UNMERGED 判定を `git branch --no-merged` ベースから `upstream:track` ベースに変更（upstream が gone のローカルブランチのみが UNMERGED 候補となり、未 push commit を含む通常の作業ブランチは候補に含まれない）（PR #66 レビュー対応）
- `branch-clean` のデフォルト動作を安全側に変更（`git branch -d` を使用し、未マージのコミットがあるブランチは保持として Skipped に記録）（PR #66 レビュー対応）
- `git remote prune --dry-run` の出力解析を LANG/LC_ALL=C に固定してロケール依存を排除（PR #66 レビュー対応）
- `branch-clean` の STALE_REF 削除を `git remote prune <remote>` の一括削除から `git update-ref -d refs/remotes/<remote>/<branch>` の候補単位削除に変更し、ユーザーが選択した STALE_REF だけが削除されるよう挙動を一致させた（PR #66 レビュー対応）
- `repo branch-clean` CLI ハンドラの到達不能だった分岐（`DeleteBranchCandidates` は常に非 nil の result を返すため `cleanErr != nil` の dead code が存在）を整理し、`cleanResult == nil` 防御パスのみを残して直線化した（PR #66 レビュー対応）
- `internal/repo` パッケージ内に共通定数 `defaultRemoteName = "origin"` を導入し、3 箇所に散在していたリテラルを置き換え（goconst 警告解消）
- ルートヘルプと README に `dsx repo branch-clean` の使用方法を追加し、リリース表記を v0.6.0 に更新

## [v0.5.0] - 2026-05-11

### Added

- Go updater で `go version -m` と `go list -m -json <module>@latest` を使った best-effort の installed/latest 比較を追加し、最新版の Go ツールは `go install` をスキップするよう改善

### Changed

- `dsx self-update` の更新判定ロジックを `internal/selfupdate` に分離し、Go updater からも同じ判定基準を再利用できるよう整理

### Fixed

- `go.targets` に `github.com/scottlz0310/dsx/cmd/dsx` が含まれる場合でも Go updater では `dsx` 本体を `go install` しないよう修正。最新版なら非エラースキップし、更新がある場合のみ `dsx self-update` を案内するエラーに変更

## [v0.4.1] - 2026-05-10

### Fixed

- pnpm v11 で `pnpm outdated -g --format json` の stdout に `[WARN]` 診断行が混入し JSON パースが失敗する問題を修正（`parseOutdatedJSON` でブラケットタグ行を除外してから JSON を抽出するよう変更）

### Changed

- Codecov アップロードに `CODECOV_TOKEN` を追加し、ブランチ保護下でのカバレッジ計測を有効化
- `codecov.yml` を追加し、プロジェクト・パッチカバレッジの閾値（各 1% 低下まで許容）と PR コメント表示を設定

## [v0.4.0] - 2026-05-09

### Added

- `dsx sys discover --apply` フラグを追加。検出した Go バイナリを `config.yaml` の `go.targets` へ自動書き込みする機能を実装
- `dsx sys discover --apply --dry-run` フラグを追加。書き込みは行わず変更内容をプレビュー表示する機能を実装
- `config.SaveAtomic()` を追加。既存 config.yaml のタイムスタンプ付きバックアップを作成した上でアトミックに書き込む
- `--dry-run` 単独指定時のエラー処理を追加（`--apply` との組み合わせが必要）
- `--apply` 時、`config.yaml` 未存在の場合は新規作成する

## [v0.3.0] - 2026-05-09

### Added

- `GoBinaryInfo` 構造体を `internal/updater` に追加（`BinaryPath`・`BinaryName`・`PackagePath`・`ModulePath`・`InstalledVersion`・`GoVersion` の 6 フィールド）
- `ParseGoBinaryInfo(binaryPath, output string)` を実装し、`go version -m` 出力の `path` 行と `mod` 行を個別フィールドに分離して解析
- `UpdateTarget()` メソッドをポインタレシーバで実装（nil ガード付き）
- `DiscoverResult` / `SkippedBinary` 型を `internal/updater` に追加
- `DiscoverGoBinariesInDir(ctx, binDir)` を実装（指定ディレクトリ内の Go バイナリをスキャンし、`go version -m` で情報を収集）
- `DiscoverGoBinaries(ctx)` を実装（`GOBIN` → `GOPATH/bin` → `~/go/bin` の順で自動解決してスキャン）
- `dsx sys discover` コマンドを追加（`GOBIN`/`GOPATH/bin`/`~/go/bin` 内の Go バイナリを検出し、モジュール情報と共に一覧表示）
- Go updater の `targets` 未設定時のメッセージを改善し、`dsx sys discover` コマンドへ誘導するヒントを追加

### Changed

- `ParseGoVersionOutput` を廃止。`path` 行と `mod` 行を同一フィールドに混在させるバグを根本解消

## [v0.2.5] - 2026-04-07

### Fixed

- `dsx self-update` が「完了」と表示しながら実際にバージョンが更新されない問題を修正
  - `go install @latest` は Go module proxy のインデックス遅延により旧バージョンをインストールする場合があった
  - GitHub Releases API で取得した明示バージョン（例: `@v0.2.5`）を指定して `go install` するよう変更

## [v0.2.4] - 2026-04-07

### Added

- `dsx repo list` に `Behind` 列を追加し、Ahead/Behind を一覧で確認できるよう改善
- `scripts/branch-chk.ps1` を追加し、`repo.root` 配下のリポジトリで `DIRTY` / `AHEAD` / `BEHIND` / `NO_UPSTREAM` などの異常状態を一括確認できるよう改善
- [docs/Dev_Tools.md](docs/Dev_Tools.md) に開発補助スクリプトの説明を追加

### Changed

- `repo update` / `dsx run` で発生した `pull` スキップ対象を集約し、完了サマリーに一覧表示するよう改善
- TUI モードの `repo update` / `dsx run` 完了後も、`pull` スキップ一覧をテキストで確認できるよう改善
- `repo update` の DryRun / 実行時に Ahead/Behind を 1 回の Git 呼び出しで取得するよう改善

### Fixed

- `repo update` で `refs/remotes/origin/HEAD` が未設定でも、`pull` を不必要にスキップせず継続できるよう修正
- `repo update` で `AutoStash=true` の tracked 変更ありリポジトリが常にスキップされる問題を修正し、`git pull --rebase --autostash` が有効に動作するよう改善
- `repo update` で `pull` 実行後も `Behind` が残る場合に警告を表示し、成功表示だけで見落とさないよう改善

## [v0.2.3] - 2026-03-27

### Added

- `dsx env unlock` サブコマンドを追加
  - `bw unlock --raw` でBitwardenをアンロックし、`BW_SESSION` 設定コマンドを標準出力に出力
  - `eval "$(dsx env unlock)"` / `dsx env unlock | Invoke-Expression` で親シェルへ反映できる（子プロセスの環境変数が消える問題を解決）
  - 既にアンロック済みの場合は即座に既存トークンを返す（高速）
  - `--sync` フラグでアンロック後にBitwardenサーバーと強制同期（トークンロール直後に使用）
- `dsx env run` に `--sync` / `--detach` フラグを追加
  - `--sync`: Bitwardenサーバーと強制同期してからコマンドを実行（トークンロール後の即時反映）
  - `--detach`: プロセスをデタッチ起動（完了を待たない）。GUIアプリ起動用
  - `--detach` + `--sync` の組み合わせ対応
  - ショートカットのターゲットに設定することでGUIアプリ（Claude Desktopなど）へ環境変数を注入可能
- `GetEnvVarsWithSync()` を `internal/secret` に追加（`--sync` フラグから使用）

### Changed

- `dsx env run` / `dsx env export` がデフォルトで `bw sync` をスキップするよう変更（高速化）
  - 変更前: 毎回 `bw sync`（2〜5秒のオーバーヘッド）
  - 変更後: ローカルキャッシュを使用（0.5秒以下）。即時反映が必要な場合は `--sync` を指定

### Fixed

- CI Lint ジョブの失敗を根本修正
  - `TestRemoveDsxBlock_ブロック操作` を `TestRemoveDsxBlock_マーカーなし` / `TestRemoveDsxBlock_マーカーあり` / `TestRemoveDsxBlock_パーミッション保持` の 3 関数に分割し、循環的複雑度を 16 から各 6 以下に削減（`gocyclo` 違反を解消）
  - `removeDsxBlock` 内の `os.WriteFile` 呼び出しに `//nolint:gosec` コメントを付加し、gosec のパストラバーサル警告（false positive）を抑制（`realPath` は `filepath.EvalSymlinks` で解決済みかつ `filepath.Rel` によるホームディレクトリ境界チェック済みのため安全）
- `dsx sys update` で `rustup check` の exit code 100 をエラーとして扱い更新が失敗する問題を修正
  - `rustup check` は更新がある場合に exit code 100 を返す仕様だが、これを異常終了と誤判定していた
  - `runCommandOutputWithLocaleCAllowExitCodes` ヘルパーを追加し、許容する exit code を指定できるよう共通化

## [v0.2.2-alpha] - 2026-02-26

### Changed

- `dsx --version` が `-ldflags` 未注入時でも `debug.ReadBuildInfo().Main.Version` からバージョン表示できるように改善（`go install ...@latest` 導線を改善）
- `self-update` の開発ビルド判定に `(devel)` を追加し、開発ビルドでは更新比較をスキップする挙動を安定化

## [v0.2.1-alpha] - 2026-02-26

### Added

- `dsx self-update` サブコマンドを追加（`--check` で更新確認のみ実行可能）

### Changed

- `dsx run` / `dsx sys update` の終了時に、`dsx` 本体の更新がある場合のみ最後に通知するよう改善
- `task lint` と CI の lint 実行を `go run .../golangci-lint/v2/...@latest` に統一し、Go バージョン不一致による失敗を回避

## [v0.2.0-alpha] - 2026-02-25

### Added

- `sys update` / `repo update` / `repo cleanup` / `dsx run` に `--log-file` フラグを追加（ジョブ実行ログをファイルに保存）
- GoReleaser によるクロスプラットフォームビルドとリリース自動化を追加（Linux/macOS/Windows）
- GitHub Actions リリースワークフロー（`v*` タグプッシュで自動リリース）を追加
- `task snapshot` / `task release:check` タスクを追加（ローカルでのリリースビルド検証）
- gitleaks によるシークレット混入チェックを追加（GitHub Actions / `task secrets`）
- `config.yaml` に `ui.tui` を追加し、`--tui` なしでも進捗TUIを既定で有効化できるように改善
- `sys update` / `repo update` に `--no-tui` を追加（設定より優先してTUIを無効化）
- `config show` / `config validate` を追加（設定の表示と妥当性チェック）
- `repo cleanup` を追加（マージ済みローカルブランチの整理、squashed 判定対応）
- `sys update` の対応マネージャに `flatpak` / `fwupdmgr` を追加
- `sys update` の対応マネージャに `pnpm` / `nvm` を追加
- `sys update` の対応マネージャに `uv` / `rustup` / `gem` を追加
- `sys update` の対応マネージャに `winget` / `scoop` を追加（Windows 環境対応）
- Windows 環境で `config init` 実行時に `winget` / `scoop` を推奨マネージャとして自動検出
- 環境変数読み込み前に `bw sync` を実行し、Bitwarden のキャッシュを最新化する機能を追加
- 全 Updater（apt/brew/cargo/flatpak/fwupdmgr/npm/pipx/scoop/snap/winget）に fake コマンド方式の Check/Update 統合テストを追加
- `sys update` / `repo update` の E2E テストを追加（`--tui` フォールバック、`--no-tui`、矛盾フラグエラー、終了コード検証）
- `sys update` / `repo update` の完了後に失敗ジョブのエラー詳細を表示する機能を追加（TUI/非TUI 両対応）
- `dsx run` に `--dry-run` / `--tui` / `--no-tui` / `--jobs` フラグを追加（sys/repo に伝播）
- テストカバレッジ改善: `internal/tui` ヘルパー関数テスト追加（32.7% → 56.9%）、`internal/secret` の `mergeEnv` テスト追加、`cmd/dsx` の gh_retry 関数群テスト追加

### Changed

- バージョン管理をハードコード (`const appVersion`) からビルド時 ldflags 注入 (`-X main.version`) 方式に変更
- `dsx run` の Bitwarden 重複呼び出しを削減：シェル関数側で既にアンロック済み・環境変数読み込み済みの場合、Go バイナリ側で `bw status` / `bw list items` の再実行をスキップ（`DSX_ENV_LOADED` マーカーにより判定）
- `dsx run` で `secrets.enabled` 設定を参照し、シークレット管理が無効な場合は bw 操作を完全にスキップするよう改善
- `dsx run` で Bitwarden アンロック失敗時に処理を中断せず、シークレット読み込みをスキップしてシステム更新・リポジトリ同期を続行するよう改善
- `dsx run` でシステム更新失敗時もリポジトリ同期を続行し、全フェーズ完了後にエラーをまとめて報告するよう改善
- `DSX_DEBUG=1` 環境変数で bw コマンドの実行時刻・所要時間をタイムスタンプ付きで出力するデバッグログを追加
- `repo update` のリポジトリ安全性チェック（isDirty/hasStash/isDetachedHEAD）を並列実行に変更し、リポジトリあたりの待ち時間を削減
- `repo update` の安全性チェックと upstream 確認を fetch 完了後に並列実行するよう改善
- `README.md` に Alpha の既知の制約、`setup-repo` 併用の推奨運用、復旧手順（`config init` 再実行 / `repo.root` 見直し）を追記
- `README.md` に `setup-repo` との比較手動チェック（移行期間の確認観点）を追記
- `README.md` にアンインストール手順を追記
- `docs/Implementation_Plan.md` を現状の実装進捗に合わせて更新
- `config init` のシステムマネージャ選択肢に `flatpak` / `fwupdmgr` を追加
- `config init` のシステムマネージャ選択肢に `pnpm` / `nvm` を追加
- `config init` のシステムマネージャ選択肢に `uv` / `rustup` / `gem` を追加
- CLI バージョン番号を `v0.1.0-alpha` に設定し、`dsx --version` で確認可能に変更
- PowerShell 連携スクリプトの `dsx-env` 関数で `dsx env export` の複数行出力を正しく連結して `Invoke-Expression` に渡すよう修正（`System.Object[]` 型エラーを解消）
- PowerShell 連携スクリプトの `dsx-run` 関数で `dsx-sys` / `dsx-repo` / `dsx-env` の成否判定を `$LASTEXITCODE` 依存から関数戻り値判定に変更し、失敗時に後続処理へ進まないよう修正
- `repo update` のジョブ表示名を Windows でも `/` 区切りで表示するよう統一
- `repo update` で未コミット変更（tracked/untracked）/stash 残存/detached HEAD を検出した場合、pull/submodule を行わず安全側にスキップして理由を表示するよう改善
- `repo update` でデフォルトブランチ以外を追跡している場合、pull/submodule を行わず安全側にスキップするよう改善
### Fixed

- `dsx env export` 実行時に読み込んだ環境変数の件数が表示されない問題を修正（stderr に統計情報を出力）
- Windows 環境で `go test ./...` が失敗する問題を修正（ホームディレクトリ環境変数のテスト分離、`gh` 実行モックの Windows 互換化など）
- Windows/PowerShell 環境で `config init` が PowerShell プロファイルパスを文字化けして誤ったフォルダを作成する問題を修正（Base64 経由で取得）
- Windows 環境で Git の `core.autocrlf` により Go ファイルが CRLF になり `task lint` の gofmt チェックが失敗する問題を回避（`.gitattributes` で LF 固定）
- Bitwarden CLI に未ログインの状態で `dev-sync` を実行すると、タイムアウトまで待って失敗する問題を修正（未ログインを即検知し、`bw login` を案内して終了）
- GitHub のレート制限（`429 Too Many Requests` / `secondary rate limit`）発生時に `gh` 呼び出しをリトライ/スロットリングし、`repo update` の GitHub 補完はレート制限時にスキップして処理を継続するよう改善
- Linux 環境で TUI 使用時に標準出力メッセージと Bubble Tea の画面制御が混在して表示が崩れる問題を修正（TUI 起動前の stdout 出力を抑制）
- `sys update --tui` / `repo update --tui` で TUI 完了後にテキストサマリー・完了メッセージが二重表示される問題を修正
- `sys update --tui` で DryRun 通知・sudo 認証メッセージ・TUI 有効通知が TUI 前に出力されて表示が崩れる問題を修正
- `pnpm` でグローバル manifest 不足時に JSON 解析エラーで失敗する問題を修正（通常更新時は自動初期化して1回再試行、DryRun時は案内のみ）

### Infrastructure

- GitHub Actions CI に Windows ランナーでのテスト実行を追加（`go test ./...`）

## [v0.1.0-alpha] - 2026-02-08

### Added

#### コアコマンド
- `dsx run` - 日次の統合タスク実行（Bitwarden解錠→環境変数読込→更新処理）
- `dsx doctor` - 依存ツール（git, gh, bw）と環境設定の診断機能

#### システム更新機能 (`sys`)
- `dsx sys update` - パッケージマネージャによる一括更新
  - `--dry-run` / `-n` フラグでドライラン対応
  - `--verbose` / `-v` フラグで詳細ログ出力
  - `--jobs` / `-j` フラグで並列実行数の指定に対応
  - `--timeout` / `-t` フラグでタイムアウト設定
  - `--tui` フラグで Bubble Tea による進捗表示（マルチ進捗バー・リアルタイムログ・失敗ハイライト）に対応
  - `apt` を単独実行に分離し、他マネージャは並列実行可能に改善
- `dsx sys list` - 利用可能なパッケージマネージャの一覧表示
- 対応パッケージマネージャ:
  - `apt` (Debian/Ubuntu)
  - `brew` (macOS/Linux Homebrew)
  - `go` (Go ツール)
  - `npm` (Node.js グローバルパッケージ)
  - `pipx` (Python CLI ツール)
  - `cargo` (Rust ツール)
  - `snap` (Snap パッケージ)
- 拡張可能な Updater インターフェースとレジストリパターンの採用

#### リポジトリ管理機能 (`repo`)
- `dsx repo update` - 管理下リポジトリを一括更新
  - `git fetch --all` / `git pull --rebase` を実行
  - `--jobs` / `-j` フラグで並列更新に対応
  - `--dry-run` / `-n` フラグで更新計画の確認に対応
  - `--tui` フラグで Bubble Tea による進捗表示（マルチ進捗バー・リアルタイムログ・失敗ハイライト）に対応
  - `--submodule` / `--no-submodule` フラグで submodule 更新設定を明示上書き可能
  - DryRun 時も upstream 有無を確認し、`pull` 計画表示を実挙動と一致させるよう改善
- `dsx repo list` - 管理下リポジトリの一覧表示
  - `config.yaml` の `repo.root` 配下をスキャン
  - `--root` フラグでスキャンルートを上書き可能
  - ステータス表示（クリーン / ダーティ / 未プッシュ / 追跡なし）
- `internal/repo` パッケージを追加（検出・状態取得ロジック）
  - `.git` 判定を厳格化（ディレクトリまたは `gitdir:` 形式ファイルのみをリポジトリとして検出）
- `internal/tui` パッケージを追加（Bubble Tea ベースの進捗UI）
- `repo.sync.submodule_update` 設定を追加（デフォルト: true）
- `internal/runner` を追加（`errgroup + semaphore` による並列実行・結果集計）

#### 環境変数機能 (`env`)
- `dsx env export` - Bitwardenから環境変数をシェル形式でエクスポート
  - bash/zsh: `eval "$(dsx env export)"`
  - PowerShell: `& dsx env export | Invoke-Expression`
  - 安全なクオート/エスケープ処理
- `dsx env run` - 環境変数を注入してサブプロセスでコマンド実行
  - `eval` を使わない安全な実行方式
  - 終了コードの正確な伝播

#### 設定管理機能 (`config`)
- `dsx config init` - 対話形式（survey）による設定ファイル生成ウィザード
  - リポジトリルートディレクトリ
  - GitHubオーナー名
  - 並列実行数
  - 有効化するパッケージマネージャの選択
  - シェル初期化スクリプトの自動設定
- `dsx config uninstall` - シェル設定からdsxを削除
- YAML形式の設定ファイル (`~/.config/dsx/config.yaml`)
- 環境変数 (`dsx_*`) によるオーバーライド対応

#### Bitwarden連携 (`internal/secret`)
- Bitwarden CLI (`bw`) ラッパーの実装
- セッショントークン管理（Unlock/Lock）
- 環境変数アイテムの取得と解析
- シェル形式でのエクスポートフォーマッター

#### 環境認識 (`internal/env`)
- コンテナ内実行の自動検出 (`IsContainer`)
- OS/環境に応じた推奨パッケージマネージャのリコメンド

### Changed

- 初回運用時の導線を改善
  - `repo list` / `repo update` で `repo.root` が未存在かつ設定未初期化の場合、`dsx config init` を明示的に案内
  - `doctor` で「設定ファイルあり / なし（デフォルト値運用）」を区別して表示
- コマンドエラーの二重表示を解消（`rootCmd` のエラー出力ポリシーを整理）
- `sys list` の「有効」列を `✅/❌` 表示に変更
- `sys update --tui` / `repo update --tui` で対象0件時に「TUI未起動」の理由を明示
- `sys.enable` に未インストールのマネージャが含まれる場合、警告を表示してスキップし、処理を継続するよう改善
- `sys update` で sudo が必要なマネージャ（`apt` / `snap`）を実行する前に、単独フェーズ・並列フェーズごとに `sudo -v` で事前認証するよう改善
- `apt` / `snap` の manager 設定で `use_sudo` と旧キー `sudo` の両方を受け付けるよう改善
- `snap` の可用性判定を強化し、`snapd unavailable` の環境では利用不可として自動スキップするよう改善
- `config init` が生成するシェル連携スクリプトを改善
  - `dsx-load-env` が `dsx env export` の失敗時に正しく終了コードを返すよう修正
  - `dev-sync` 互換関数を `Bitwarden 解錠 → 環境変数を親シェルへ読み込み → dsx run` の順で実行するよう改善
  - 設定された実行パスが無効な場合に `command -v dsx`（PowerShell は `Get-Command`）へフォールバック
- `config init` で指定した `repo.root` が未存在の場合に作成確認を追加（拒否時は保存せず終了）
- `config init` の GitHub オーナー入力で `gh auth` のログインユーザーを自動補完するよう改善（手動入力は上書き可能）
- `config init` 実行時に既存 `config.yaml` があれば現在値を初期値として再編集できるよう改善
- `dsx run` の `sys` / `repo` セクションをプレースホルダーから実処理に置き換え、`sys update` → `repo update` を順次実行するよう改善
- `repo update` で `repo.github.owner` を参照し、`repo.root` 配下で不足しているリポジトリを clone したうえで更新を継続するよう改善（dry-run は計画表示のみ）
- README の初回セットアップ手順に `dsx config init` 必須を明記

### Infrastructure

- Go 1.25 による開発環境
- Cobra CLI フレームワークの採用
- Viper による設定管理
- Taskfile.yml によるタスクランナー（Windows互換）
- `task daily` を追加し、日常運用の標準コマンドを `task check` に統一
- golangci-lint による静的解析
  - 循環的複雑度 (gocyclo)
  - 認知複雑度 (gocognit)
  - 重複コード検出 (dupl)
  - エラーハンドリング (errorlint)
  - その他品質チェック
- `wsl` から `wsl_v5` へ移行（非推奨警告を解消）
- GitHub Actions CI/CD（予定）
- DevContainer 対応

### Documentation

- README.md - プロジェクト概要と使用方法
- README.md に利用者向けインストール手順と日常運用（マニュアルテスト）手順を追記
- Implementation_Plan.md - 設計ドキュメント
- Legacy_Migration_Analysis.md - 旧ツールからの移行分析
- AGENTS.md - AIエージェント運用ガイドライン
- CONTRIBUTING.md - コントリビューションガイド
- SECURITY.md - セキュリティポリシー

---

[Unreleased]: https://github.com/scottlz0310/dsx/compare/v0.6.0...HEAD
[v0.6.0]: https://github.com/scottlz0310/dsx/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/scottlz0310/dsx/compare/v0.4.1...v0.5.0
[v0.4.1]: https://github.com/scottlz0310/dsx/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/scottlz0310/dsx/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/scottlz0310/dsx/compare/v0.2.5...v0.3.0
[v0.2.5]: https://github.com/scottlz0310/dsx/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/scottlz0310/dsx/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/scottlz0310/dsx/compare/v0.2.2-alpha...v0.2.3
[v0.2.2-alpha]: https://github.com/scottlz0310/dsx/compare/v0.2.1-alpha...v0.2.2-alpha
[v0.2.1-alpha]: https://github.com/scottlz0310/dsx/compare/v0.2.0-alpha...v0.2.1-alpha
[v0.2.0-alpha]: https://github.com/scottlz0310/dsx/compare/v0.1.0-alpha...v0.2.0-alpha
[v0.1.0-alpha]: https://github.com/scottlz0310/dsx/releases/tag/v0.1.0-alpha
