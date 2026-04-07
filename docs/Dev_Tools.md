# 開発ツール・補助スクリプト

`dsx` の開発・検証で利用する補助スクリプトと使用方法をまとめます。

---

## scripts/branch-chk.ps1

### 目的

`dsx repo update` 実行後に、リポジトリが実際に同期されているかを検証するための PowerShell スクリプトです。

`dsx` のサマリーが「全件成功」と表示した場合でも、実際に BEHIND 状態のリポジトリが残っていないかをこのスクリプトで確認できます（Issue [#29](https://github.com/scottlz0310/dsx/issues/29) 参照）。

### 前提条件

- Windows PowerShell 5.1 以上 または PowerShell Core 7.x 以上
- `git` コマンドが PATH に通っていること
- 対象リポジトリのリモートに `origin` が設定されていること

> **対象範囲**: スクリプトは `$Root` 直下のディレクトリのみを走査します（再帰なし）。
> これは `dsx` の `repo.root` 直下を対象とする仕様に対応しています。

### 使い方

```powershell
# デフォルト（$HOME/src 以下の直下ディレクトリを対象）
.\scripts\branch-chk.ps1

# 任意のルートを指定する場合
.\scripts\branch-chk.ps1 -Root "D:\work\repos"
```

### 出力例

問題があるリポジトリのみ出力されます。問題がなければ出力はありません。

```
C:\Users\jojob\src\my-app : main [DIRTY, BEHIND:3]
C:\Users\jojob\src\tools  : main [BEHIND:1]
C:\Users\jojob\src\infra  : develop [AHEAD:2, BEHIND:5, DIVERGED]
C:\Users\jojob\src\old    : (detached) [DETACHED]
C:\Users\jojob\src\fresh  : main [NO_UPSTREAM]
```

| フラグ | 意味 |
|--------|------|
| `DIRTY` | 未コミット変更あり（tracked/untracked を含む） |
| `AHEAD:N` | ローカルがリモートより N コミット先行（未プッシュ） |
| `BEHIND:N` | リモートがローカルより N コミット先行（未取得） |
| `DIVERGED` | AHEAD と BEHIND が両方ある（分岐状態） |
| `DETACHED` | detached HEAD 状態（AHEAD/BEHIND 判定はスキップ） |
| `NO_UPSTREAM` | upstream（追跡ブランチ）が未設定 |
| `SYNC_CHECK_FAILED` | `git rev-list` が失敗（upstream は存在するが参照解決に失敗） |

### dsx repo update との対比

| 状態 | dsx の動作 | branch-chk での確認 |
|------|-----------|---------------------|
| クリーン + upstream あり | pull 実行 | 正常時は何も出力されない |
| DIRTY | スキップ（安全機構） | `DIRTY` フラグで検出可能 |
| `refs/remotes/origin/HEAD` 未設定 | pull スキップ（Issue #29 で修正予定） | `BEHIND:N` で検出可能 |
| upstream 未設定 | スキップ | `NO_UPSTREAM` フラグで検出可能 |
| detached HEAD | スキップ | `DETACHED` フラグで検出可能 |

### 注意事項

- スクリプトは内部で `git fetch --quiet` を実行します。ネットワーク疎通が必要です。
- 各リポジトリへの fetch はシリアルで実行されるため、リポジトリ数が多い場合は時間がかかります。
- `refs/remotes/origin/HEAD` が未設定のリポジトリで `git rev-list` が失敗した場合は `SYNC_CHECK_FAILED` が表示されます。`git remote set-head origin -a` を実行してから再試行してください。

---

## 関連ドキュメント

- [実装計画書](Implementation_Plan.md)
- [旧ツール移行分析](Legacy_Migration_Analysis.md)
