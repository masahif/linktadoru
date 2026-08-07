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

### 3. シード一覧と深さ制限付きクロール

実行ごとに生成した対象はファイルから読みます。`-` は標準入力です。

```bash
./linktadoru --seed-file urls.txt --max-depth 1 --limit 0
fetch-target-list | ./linktadoru --seed-file - --max-depth 1 --limit 0
```

シードファイルは1行1URLです。空行、前後の空白、`#`で始まるコメントは無視します。
`--seed-file`とURL引数は同時に指定できません。

`max_depth: N`は、深さ0のシードから深さNで登録された子までを取得します。0は
無制限です。DBへ保存する値は最初に発見・登録された経路の深さであり、最短距離の
保証ではありません。非同期キューは浅いURLを優先しますが、層全体の完了を待たない
ため、遅いページ1件が他のワーカーを止めません。そのためNが2以上の場合、応答順に
よって上限内に入る子孫が変わることがあります。独立して比較するスナップショットや、
上限変更後に全URLを判定し直す場合は新しいDBを使ってください。

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
  - '^https://search\.example/search(?:/.*)?(?:\?.*)?$'

exclude_patterns:
  - '\.pdf$'
  - '/admin/'
  - '[?&]print=1(?:&|$)'
```

設定ファイルを使用して実行：

```bash
./linktadoru --config mysite-config.yml https://httpbin.org
```

### URLパターンの規則

通常は、DBに保存された深さ0のシードと同じorigin（scheme・hostname・実効port）を
許可します。`include_patterns`は別の絶対URL範囲を追加し、`exclude_patterns`は
許可集合からURLを除きます。excludeは常に優先されます。同じ判定を、キュー済みURL、
発見リンク、redirect、retryに使います。

正規表現はGo標準の`regexp`（RE2）です。

- `/`は通常の文字なのでエスケープ不要です。JavaScriptの`/pattern/`記法ではありません。
- `.`は任意の1文字です。hostnameのドットは`\.`と書きます。
- includeは絶対URL文字列全体との一致です。読みやすさのため`^`と`$`を推奨します。
- `/products/`のような相対includeは、意味を黙って変えず起動時にエラーにします。
- excludeは部分一致を使えるため、`/ika/`やquery parameterの除外に向きます。
- 正規表現は大文字と小文字を区別し、保存された絶対URL文字列をそのまま照合します。
- URLをpercent-decodeしてから照合はしません。`/ika/`は`/%69ka/`に一致しません。
- YAMLではsingle quoteを使うとbackslashをそのまま読みやすく書けます。

```yaml
seed_urls:
  - https://example.com/

include_patterns:
  - '^https://hogehoge\.com/search(?:/.*)?(?:\?.*)?$'

exclude_patterns:
  - '/ika/'
  - '[?&]page=[0-9]+(?:&|$)'
```

この例では`https://example.com/news/1`と
`https://hogehoge.com/search/items?q=go`を許可し、
`https://hogehoge.com/account`と`/ika/`を含むURLを拒否します。
includeだけで許可されたoriginへは、認証情報や設定済みcustom headerを送りません。
`^https?://.*$`のような広いincludeは、`follow_external_hosts: true`と同じ到達リスクを
持ちます。`follow_external_hosts: true`の場合、`include_patterns`では許可範囲を
狭められません。制限には`exclude_patterns`を使用してください。

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

### 4. パターンで対象範囲を追加

関連する検索endpointを追加し、静的assetを除外：

```bash
./linktadoru \
  --include-patterns "^https://search\.example/search(?:/.*)?(?:\?.*)?$" \
  --exclude-patterns "\\.jpg$|\\.png$|\\.css$|\\.js$" \
  https://example.com
```

シードoriginは引き続き許可されます。includeは一致する`search.example`の範囲だけを
追加し、excludeは両方の範囲から一致するassetを除外します。

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

Ctrl-C（SIGINT/SIGTERM）で安全に停止できます。処理中の状態は永続化され、データベースは正常にクローズされます。同じ`--database`を指定して再実行すれば再開でき、`processing`のまま残った行は次回起動時に深さを維持したまま再キューされます。URLをシードとして再指定すると、深さ0として再取得します。再利用したDBでは深さ0の起点が加算され、後から渡したシード一覧は以前の起点やキューを置き換えません。起点集合を置き換える場合や独立して比較できるスナップショットを作る場合は、新しいDBを使ってください。認証またはカスタムヘッダーを使う場合、資格情報を渡してよい起点をDB履歴から推測しないため、seedなしでは再開せず、シード一覧の再指定を要求します。このとき資格情報を送るのは今回指定したシードのoriginだけで、過去から蓄積された起点には送りません。

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
