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

### 4. クロールの深さを制限する

`--max-depth` は、シードから何ホップまで辿るかを指定します。シードは深さ0なので、
次の例は各サイトのトップページと、そこから1クリックで到達できるページだけをクロールします。

```bash
./linktadoru --seed-file urls.txt --max-depth 1
```

全シードで1つの予算を共有する `--limit` と違い、深さの制限は単独で使えばシードごとに
等しく効きます。リストが長くても各シードのホップ上限は同じです。ただし `--limit` を併用すると
共有予算が復活し、ページ数を使い切った時点で停止するため、後半のシードが不完全なまま
終わることがあります。上限を超えたリンクもリンク解析用に記録され、取得されないだけです。

スケジューリングは深さ制限の値によって変わります。

| 設定 | スケジューリング |
|---|---|
| `--max-depth 0`（既定） | 非同期。深さは記録しない |
| `--max-depth 1` | 非同期。浅い方を優先し、リトライはキューが空になった後 |
| `--max-depth 2` 以上 | 深さの層ごとに順番に処理し、リトライは各層の内側 |

`--max-depth 1` では、シードを優先はしますが待つことはしないため、応答の遅いシードが
占有するのは自分のワーカーだけです。`--max-depth 2` 以上では各深さを終えてから次に進みます。
後から見つかった短い経路によって、子リンクが上限内に入るかどうかが変わるためです。
その代わり、遅いページ1件がその層全体を待たせることになります。

`--max-depth` は、深さ情報のない未処理ページ（キューに残っているURL、および再試行の
余地が残っている失敗）があるデータベースには使えません。新しいデータベースを使うか、
その分を `--max-depth` なしで先に完了させてください。
詳細は [設定リファレンス](configuration.md#crawl-depth) を参照してください。

### 5. 設定ファイルの使用

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
- `error` — 受け入れられる回答が得られなかった状態。トランスポートレベルの失敗（DNS、タイムアウト、接続リセット）、回復しなかった一時的なHTTPレスポンス、`max_response_size`超過、不正なURLが含まれます

`error`の行にも観測した内容は残ります。これにより次の2つを区別できます。クロール同士を比較するときに効いてきます。

- `status_code`がある`error` — サーバーは応答したが一時的な内容で、リトライを使い切った。そのページの現在の内容は**不明**
- `status_code`がない`error` — HTTPレスポンスが一度も得られなかった

どちらもページが削除された証拠にはなりません。削除のシグナルは、`completed`として記録された明示的な`404`や`410`です。

### リトライ

リトライの対象は2種類です。トランスポートレベルの失敗（`network_error`）と、一時的なHTTPレスポンス（`408`、`429`、`500`、`502`、`503`、`504`）です。後者はサーバーが「今は答えられない」と言っている状態にあたります。それ以外のステータスはリソースについての実際の回答なのでリトライしません。`404`をリトライしても、有用なシグナルを不明に変えてしまうだけです。

URLごとの試行回数は`retry_count`で管理され、同じデータベースに対するクロール全体で合計3回までです。最後まで走った実行は戻る前に予算を使い切ります。中断した場合、残りの試行回数は同じデータベースへの次回の再開に引き継がれ、カウントが最初からやり直しになることはありません。リトライは通常のキューが空になった後、`--max-depth`指定時は次の深さ層を開く前に行われます。回復したページのリンクが正しい層に入るようにするためです。

試行の間隔は制御されます。トランスポート障害も対象です。タイムアウトするホストはすぐ再びタイムアウトするので、待機なしのリトライは予算をミリ秒で使い切ってしまいます。`Retry-After`ヘッダーは尊重しますが、誤設定や悪意ある値でクロールが停止しないよう60秒を上限とします。ヘッダーがない場合は1秒から始めて試行ごとに倍増し、同じ上限で頭打ちになります。待機時刻はデータベースに保存されるため中断した実行をまたいでも保たれ、再開直後にホストへ殺到することはありません。決定的な失敗（不正なURL、サイズ超過）はリトライしません。

失敗した試行はすべて`crawl_errors`テーブルにステータスコードと試行番号つきで記録されるため、3回目で回復したURLについても最初の2回が何を見たかを追えます。

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
