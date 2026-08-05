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

### 3. ファイルまたは標準入力からのシードURL

URLリストが実行ごとに生成される場合、コマンドラインに収まらないほど長い場合、
またはリポジトリの外で管理されている場合は `--seed-file` を使います。

```bash
./linktadoru --seed-file urls.txt

# '-' は標準入力を読むため、パイプでリストを渡せます
fetch-target-list | ./linktadoru --config prod.yml --seed-file -
```

ファイルは1行に1つのURLを書きます。前後の空白は除去され、空行と `#` で始まる行は
無視されます。CRLF改行と先頭のUTF-8 BOMも扱えます。

```text
# 夜間クロール対象
https://example.com
https://docs.example.com/guide
```

`--seed-file` とURL引数は同時に指定できません。どちらか一方でシードを渡してください。
いずれも設定ファイルの `seed_urls` より優先されるため、設定ファイルを固定したまま
リストだけを差し替えられます。

ファイルが存在しない・読めない場合、および64KiBを超える行がある場合は、
リストの一部だけをクロールせずエラーで停止します。URLが1件も無いファイルは
シード無しの実行と同じ扱いになり、既存データベースのキューから再開するか、
クロール対象が無い旨を報告します。

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

キューが空になった後、`last_error_type`が`network_error`のページのみが再キューされ、1回の実行につき1リトライパスが行われます。URLごとの試行回数は`retry_count`で管理され、合計3回までです。決定的な失敗はリトライされません。

### robots.txtのCrawl-delay

robots.txtの`Crawl-delay`は、設定した`request_delay`より遅い場合にのみ適用されます。クロールを遅くする方向にしか働かず（速くなることはありません）、上限は60秒です。

### 中断と再開

Ctrl-C（SIGINT/SIGTERM）で安全に停止できます。処理中の状態は永続化され、データベースは正常にクローズされます。同じ`--database`を指定して再実行すれば再開でき、`processing`のまま残った行は次回起動時に自動的に再キューされます。

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
