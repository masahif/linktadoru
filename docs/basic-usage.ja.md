# 基本的な使用例

このドキュメントでは、LinkTadoruを使用したWebクローリングとリンク解析の実践的な例を提供します。

## クイックスタート

### 1. シンプルなサイトクロール

デフォルト設定で単一のWebサイトをクロール：

```bash
./linktadoru https://httpbin.org
```

### 2. カスタム設定での制限付きクロール

2つの並行ワーカー・2秒間隔で最大10ページをクロール（`--delay`は秒数を数値で指定）：

```bash
./linktadoru --limit 10 --concurrency 2 --delay 2 https://httpbin.org
```

### 3. シード一覧と1ホップのクロール

実行ごとに生成した対象はファイルから読みます。`-` は標準入力です。

```bash
./linktadoru --seed-file urls.txt --max-depth 1 --limit 0
fetch-target-list | ./linktadoru --seed-file - --max-depth 1 --limit 0
```

シードファイルは1行1URLです。空行、前後の空白、`#`で始まるコメントは無視します。
`--seed-file`とURL引数は同時に指定できません。

`max_depth: 0`は無制限です。正の値では、シードをdepth 0、そのリンクをdepth 1
として、指定した発見depthまでをキューへ入れます。キューは非同期のままなので、
遅いページが別ワーカーで既に投入された深い処理を止めません。URLポリシーの復元が
入るまで、bounded crawlには明示的なシードと新しい空のDBが必要です。

一時的な応答（408、429、500、502、503、504）は観測内容をDBへ残し、通常処理の
後に合計3回まで再試行します。

### 4. 設定ファイルの使用

設定ファイルを作成：

```yaml
# mysite-config.yml
concurrency: 3
request_delay: 1             # 秒（数値）
request_timeout: "15s"       # Go duration文字列
user_agent: "MyBot/1.0"
ignore_robots_txt: false
limit: 50
database_path: "./mysite-crawl.db"

include_patterns:
  - "^https?://[^/]*httpbin\\.org/.*"

exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"
  - ".*\\?print=1"
```

設定ファイルを使用して実行：

```bash
./linktadoru --config mysite-config.yml https://httpbin.org
```

## 高度な使用例

### 1. 複数サイトのクロール

関連する複数のサイトをクロール：

```bash
./linktadoru \
  --limit 100 \
  --include-patterns "^https?://[^/]*(site1|site2)\.com/.*" \
  https://site1.com \
  https://site2.com
```

### 2. 前回のクロールの再開

LinkTadoruは既存のデータベースから自動的に再開します：

```bash
# 最初の実行（中断される）
./linktadoru --database mycrawl.db --limit 1000 https://httpbin.org

# 中断した場所から再開
./linktadoru --database mycrawl.db
```

### 3. アグレッシブクロール（robots.txt無視）

```bash
./linktadoru \
  --ignore-robots-txt \
  --concurrency 20 \
  --delay 0.5 \
  https://httpbin.org
```

### 4. パターンを使った集中クロール

ブログ記事と記事のみをクロール：

```bash
./linktadoru \
  --include-patterns "^https?://[^/]*httpbin\.org/(blog|articles)/.*" \
  --exclude-patterns "\\.jpg$|\\.png$|\\.css$|\\.js$" \
  https://httpbin.org
```

## クロールの挙動

### ページステータスのライフサイクル

各URLは`pages`テーブルの1行に対応し、`status`カラムがライフサイクルを表します：

- `discovered` — クロール済みページ上でリンクとして発見された状態。リンク解析用に記録されるだけで、クロール対象にはならない
- `pending` — クロール待ち（シードURL、およびinclude/excludeフィルタを通過した発見リンク）
- `processing` — ワーカーが取得処理中
- `completed` — 取得が完了した状態。注意: 404などのHTTPエラーも`completed`になります。結果は`status_code`カラムで確認してください
- `skipped` — robots.txtによりブロック
- `error` — 取得に失敗した状態。トランスポートレベルの失敗（DNS、タイムアウト、接続リセット）のほか、`max_response_size`超過や不正なURLも含まれます

### リトライ

キューが空になった後、一時的な通信障害とHTTP 408、429、500、502、503、504の
応答を再キューし、URLごとに合計3回になるまで試行します。試行回数は
`retry_count`で管理します。決定的な失敗は再試行しません。
再試行にもドメイン単位のrequest delayとrobots.txtの`Crawl-delay`は適用されますが、
`Retry-After`は解釈しないため、サーバーが指定した時刻より早く再訪する場合があります。

### robots.txtのCrawl-delay

robots.txtの`Crawl-delay`は、設定した`request_delay`より遅い場合にのみ適用されます。クロールを遅くする方向にしか働かず（速くなることはありません）、上限は60秒です。

### 中断と再開

Ctrl-C（SIGINT/SIGTERM）で安全に停止できます。処理中の状態は永続化され、データベースは正常にクローズされます。同じ`--database`を指定して再実行すれば再開でき、`processing`のまま残った行は次回起動時に自動的に再キューされます。ただし、bounded `max_depth`実行は一時的に明示的なシードと新しい空のDBを必要とします。

## 出力の分析

### データベースクエリ

クロール後、SQLで結果を分析：

```sql
-- レスポンス時間順の上位ページ
SELECT url, ttfb_ms, download_time_ms 
FROM pages 
WHERE status = 'completed'
ORDER BY ttfb_ms DESC 
LIMIT 10;

-- リンク分析
SELECT 
    link_type,
    COUNT(*) as count
FROM links 
GROUP BY link_type;

-- 壊れたリンクを発見
SELECT url, last_error_message
FROM pages 
WHERE status = 'error';
```

### データエクスポート

```bash
# CSVにエクスポート
sqlite3 -header -csv linktadoru.db "SELECT * FROM pages WHERE status='completed';" > pages.csv
sqlite3 -header -csv linktadoru.db "SELECT * FROM links;" > links.csv
```

## パフォーマンスチューニング

サイト規模別の推奨設定と礼儀正しいクロールについては、[設定リファレンス — Performance Tuning](configuration.md#performance-tuning)（英語）を参照してください。

## トラブルシューティング

### よくある問題

1. **データベースロック**: 他のインスタンスを停止するか、異なるデータベースファイルを使用
2. **エラーが多すぎる**: タイムアウトを増やすか、並行性を減らす
3. **robots.txtによるブロック**: `--ignore-robots-txt`フラグを使用（責任を持って使用）
4. **メモリ使用量**: 大規模サイトでは並行性を減らす

### 進行状況の監視

```bash
# 実行中のキュー状況を確認
sqlite3 linktadoru.db "SELECT status, COUNT(*) FROM pages GROUP BY status;"

# 最近のエラーを表示
sqlite3 linktadoru.db "SELECT url, error_message FROM crawl_errors ORDER BY occurred_at DESC LIMIT 5;"
```
