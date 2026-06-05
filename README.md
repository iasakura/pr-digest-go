# daily-digest

その日に出した PR（draft 含む）とコミットを複数の GitHub 個人アカウントから取得し、
ローカルの Claude Code CLI で要約して Obsidian の vault にデイリーノートとして保存します。

- 依存: Go 標準ライブラリのみ（外部 Go モジュールなし）
- 実行時に必要な外部バイナリ: `claude`（Claude Code CLI）
- Claude API への従量課金は発生しません。Max プランのセッションを CLI 経由で利用します。

## ビルド

```sh
go build -o daily-digest .
```

## 設定（環境変数）

| 変数 | 必須 | 説明 |
|------|------|------|
| `OBSIDIAN_VAULT` | ○ | Obsidian vault のルートパス |
| `DIGEST_SUBDIR` | | vault 内の保存先サブフォルダ（既定 `DailyDigest`） |
| `GH_LOGIN_1` / `GH_TOKEN_1` | ○ | アカウント1のユーザー名とトークン |
| `GH_LABEL_1` | | アカウント1の表示ラベル（既定はログイン名） |
| `GH_LOGIN_2` / `GH_TOKEN_2` | | アカウント2（同様に `_3`〜`_9` まで対応） |
| `DIGEST_TZ` | | タイムゾーン（既定 `Asia/Tokyo`） |
| `DIGEST_DATE` | | 対象日 `YYYY-MM-DD`（既定は本日） |
| `CLAUDE_BIN` | | `claude` の場所（PATH に無い場合のみ） |

### GitHub トークン

各アカウントで Personal Access Token を発行してください。
- Classic: `repo` スコープ（private リポジトリの draft PR 取得に必要）
- Fine-grained: 対象リポジトリへの `Pull requests: read` / `Contents: read`

draft PR や private リポジトリの内容は認証なしでは取得できないため、トークンは必須です。

## 実行

### macOS / Linux

`run.sh` をコピーして秘密情報を記入し、実行してください。

```sh
cp run.sh run.local.sh   # run.local.sh は .gitignore 推奨
# 値を編集
./run.local.sh
```

### Windows (PowerShell)

`run.ps1` をコピーして秘密情報を記入し、実行してください。

```powershell
Copy-Item run.ps1 run.local.ps1   # run.local.ps1 は .gitignore 推奨
# 値を編集
.\run.local.ps1
```

パスはクォート付きの `~`（例 `"~/Documents/vault"`）も自動で展開されます。

## 定期実行（例）

### cron（毎日 23:30）

```cron
30 23 * * * /path/to/daily-digest/run.local.sh >> /tmp/daily-digest.log 2>&1
```

### macOS launchd

`~/Library/LaunchAgents/com.user.daily-digest.plist` を作成し、`run.local.sh` を
`ProgramArguments` に指定して `StartCalendarInterval` で時刻を設定してください。

## 出力

`<vault>/<DIGEST_SUBDIR>/YYYY-MM-DD.md` に以下を書き出します。

- YAML frontmatter（date, tags, generated）
- Claude による日本語要約（ハイライト＋アカウント／リポジトリ別の整理）
- 折りたたみ callout 内に生データ（要約失敗時はこちらが本文になります）

## 動作の補足

- PR は GitHub Search API の `author:<login> type:pr updated:<日付>` で取得し、
  draft / open / merged を区別します。
- コミットは `author:<login> author-date:<日付>` で取得します。
- Claude が失敗した場合も生データをノートに保存するため、データは失われません。
- ノート書き込みは一時ファイル経由のアトミック書き込みです。
