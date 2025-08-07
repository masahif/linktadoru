# 基本的な使用例

このドキュメントでは、LinkTadoruを使用したWebクローリングとリンク解析の実践的な例を提供します。

## クイックスタート

### 1. シンプルなサイトクロール

デフォルト設定で単一のWebサイトをクロール：

```bash
./linktadoru https://httpbin.org
```

### 2. カスタム設定での制限付きクロール

2つの並行ワーカーで最大10ページをクロール：

```bash
./linktadoru --limit 10 --concurrency 2 --delay 2s https://httpbin.org
```

### 3. 設定ファイルの使用

設定ファイルを作成：

```yaml
# mysite-config.yml
concurrency: 3
request_delay: 1s
request_timeout: 15s
user_agent: "MyBot/1.0"
ignore_robots: false
limit: 50
database_path: "./mysite-crawl.db"

include_patterns:
  - "^https?://[^/]*httpbin\.org/.*"

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
  --ignore-robots \
  --concurrency 20 \
  --delay 500ms \
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

### 大規模サイト向け

```yaml
# high-performance.yaml
concurrency: 50
request_delay: 100ms
request_timeout: 10s
user_agent: "FastCrawler/1.0"
limit: 0  # 無制限
```

### 礼儀正しいクロール

```yaml
# respectful.yaml
concurrency: 2
request_delay: 5s
request_timeout: 30s
ignore_robots: false
user_agent: "PoliteBot/1.0"
```

## トラブルシューティング

### よくある問題

1. **データベースロック**: 他のインスタンスを停止するか、異なるデータベースファイルを使用
2. **エラーが多すぎる**: タイムアウトを増やすか、並行性を減らす
3. **robots.txtによるブロック**: `--ignore-robots`フラグを使用（責任を持って使用）
4. **メモリ使用量**: 大規模サイトでは並行性を減らす

### 進行状況の監視

```bash
# 実行中のキュー状況を確認
sqlite3 linktadoru.db "SELECT status, COUNT(*) FROM pages GROUP BY status;"

# 最近のエラーを表示
sqlite3 linktadoru.db "SELECT url, error_message FROM crawl_errors ORDER BY occurred_at DESC LIMIT 5;"
```