# 機械可読なイベント出力（`--events-file`）

`dsx run` / `dsx sys update` / `dsx repo update` は、`--events-file <パス>` を指定すると、実行の経過と結果を **JSON Lines**（1 行 1 イベントの JSON）でファイルに書き出します。Arbor など外部のツールが、警告・スキップ・失敗を、標準出力のテキストを解析せずに取得するための契約です。

- 人が読む標準出力・標準エラーは変わりません（`--events-file` は追加の出力先です）。
- 出力先は、ファイルです（標準出力には混ぜません）。存在すれば上書きします。作成できないパスを指定すると、更新を始める前にエラーで終了します。
- 各行は、書き込みのたびに書き出されるため、実行中にファイルを追いかけて読むこともできます。
- `dsx run` では、sys と repo が 1 つのファイルを共有し、`start` と `summary` は 1 回だけです（`phase` で区別します）。
- 書き込みに失敗しても、更新そのものは止めません。失敗は終了時に標準エラーへ警告として出ます。

## スキーマの版

すべてのイベントに `schema`（整数）があります。現在は **`1`** です。

- フィールドの追加は、版を上げません。受け取る側は、**未知のフィールドと未知の `type` を無視**してください。
- フィールドの削除・改名・意味の変更、列挙値（`status` / `code` / `kind` / `category`）の意味の変更は、版を上げます。列挙値の追加は版を上げないため、未知の値は「その他」として扱ってください。
- スキーマは `internal/event` のテスト（golden）で固定されています。

## 共通のフィールド

| フィールド | 型 | 説明 |
|---|---|---|
| `schema` | 整数 | スキーマの版 |
| `type` | 文字列 | イベントの種別 |
| `time` | 文字列 | UTC の RFC 3339（ナノ秒つき） |

## イベント

### `start`（最初の 1 行）

| フィールド | 説明 |
|---|---|
| `command` | `run` / `sys update` / `repo update` |
| `version` | dsx のバージョン |

### `job_started`

マネージャまたはリポジトリの更新を始めたとき。

| フィールド | 説明 |
|---|---|
| `phase` | `sys` / `repo` |
| `kind` | `manager`（パッケージマネージャの更新）/ `manager_self_update`（マネージャ本体の更新）/ `repo`（リポジトリの更新） |
| `name` | マネージャ名（`go`・`npm` など）またはリポジトリの表示名 |

### `job_finished`

| フィールド | 説明 |
|---|---|
| `phase` / `kind` / `name` | `job_started` と同じ |
| `status` | `success` / `failed` / `skipped` |
| `code` | `failed` / `skipped` の理由の識別子（`success` では省略） |
| `reason` | 人が読む理由（`success` では省略） |
| `updated` | 更新したパッケージ数 |
| `duration_ms` | 所要時間（ミリ秒）。`job_started` が無いジョブ（実行しなかったもの）は `0` |

`job_started` が無く `job_finished` だけのジョブがあります（未インストールのマネージャ、キャンセルで開始しなかったジョブ）。

`status` と `code` の組み合わせ:

| status | code | 意味 | 終了コード |
|---|---|---|---|
| `success` | — | 成功 | 0 |
| `failed` | `error` | 更新に失敗した（一部のパッケージの失敗を含む） | **1** |
| `skipped` | `unavailable` | マネージャが未インストールまたは利用不可で、実行しなかった | 0 |
| `skipped` | `pull_skipped` | pull を実行しなかった（upstream なし等） | 0 |
| `skipped` | `cancelled` | キャンセルまたはタイムアウトで、完了しなかった・開始しなかった | **1** |

### `warning`

| フィールド | 説明 |
|---|---|
| `phase` | `sys` / `repo`（`run` の開始前など、段階が決まる前は空） |
| `category` | 分類（下表） |
| `subject` | 対象（マネージャ名・リポジトリ名など。無ければ空） |
| `message` | 人が読むメッセージ |

| category | 意味 |
|---|---|
| `manager_unknown` | 設定の `sys.enable` に、dsx に存在しないマネージャ名がある |
| `manager_config` | マネージャ固有の設定を適用できず、有効化できなかった |
| `repo_warning` | pull 後の警告（BEHIND の残存など） |
| `config_fallback` | 設定ファイルを読めず、既定の設定で続行した |
| `github_rate_limit` | GitHub のレート制限で、リポジトリ一覧の補完を省略した |

警告は終了コードに影響しません。

### `summary`（最後の 1 行）

| フィールド | 説明 |
|---|---|
| `command` | `start` と同じ |
| `ok` | 終了コードが 0 なら `true` |
| `error` | `ok` が `false` のときの、エラーメッセージ |
| `jobs` | `{success, failed, skipped}` のジョブ件数 |
| `updated` | 更新したパッケージの合計 |
| `warnings` | 警告の件数 |
| `issues` | `failed` / `skipped` のジョブの一覧（`phase`・`kind`・`name`・`status`・`code`・`reason`）。無ければ `[]` |
| `warning_items` | 警告の一覧（`warning` イベントと同じ内容）。無ければ `[]` |

`summary` は、実行中に書き出したイベントから組み立てるため、`job_finished` と `warning` の内容は、**漏れなく** `jobs` / `issues` / `warning_items` に現れます。

## 終了コード

| 終了コード | 条件 |
|---|---|
| `0` | 失敗がない。警告と、`unavailable` / `pull_skipped` のスキップは `0` |
| `1` | `failed` のジョブがある。`cancelled` のスキップがある（完了していない）。設定・root・イベントファイルの作成などの致命的なエラー |

「警告だけ」「スキップだけ」の実行は `0` です。気づくべきことは、終了コードではなく、`summary` の件数と理由で判断してください。

## 例

```jsonl
{"schema":1,"type":"start","time":"2026-10-20T20:16:29.43Z","command":"sys update","version":"v0.13.0"}
{"schema":1,"type":"job_finished","time":"2026-10-20T20:16:29.53Z","phase":"sys","kind":"manager","name":"gem","status":"skipped","code":"unavailable","reason":"未インストールまたは利用不可のためスキップ","updated":0,"duration_ms":0}
{"schema":1,"type":"job_started","time":"2026-10-20T20:16:29.53Z","phase":"sys","kind":"manager","name":"go"}
{"schema":1,"type":"job_finished","time":"2026-10-20T20:16:43.79Z","phase":"sys","kind":"manager","name":"go","status":"success","updated":2,"duration_ms":14260}
{"schema":1,"type":"summary","time":"2026-10-20T20:16:44.07Z","command":"sys update","ok":true,"jobs":{"success":1,"failed":0,"skipped":1},"updated":2,"warnings":0,"issues":[{"phase":"sys","kind":"manager","name":"gem","status":"skipped","code":"unavailable","reason":"未インストールまたは利用不可のためスキップ"}],"warning_items":[]}
```
