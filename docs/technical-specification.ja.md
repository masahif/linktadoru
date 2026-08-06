# 技術仕様書

## 概要

LinkTadoruは、SEO分析用に設計された高性能で並行処理可能なWebクローラーです。Go言語で構築され、レート制限とrobots.txt準拠による礼儀正しさを維持しながら、ゴルーチンを活用した並列処理を実現しています。

## アーキテクチャ

### コア設計原則

1. **モジュラーアーキテクチャ**: 明確に定義されたインターフェースによる関心の分離
2. **並行処理**: スケーラブルなクロールのためのワーカープールパターン
3. **メモリ効率**: ストリーミング処理と制限付きキュー
4. **フォールトトレランス**: リトライメカニズムと優雅なエラーハンドリング
5. **拡張性**: 簡単なコンポーネント置換のためのインターフェースベース設計

### コンポーネント概要

```
┌─────────────────┐     ┌──────────────┐     ┌──────────────┐
│   CLI/設定      │────▶│  クローラー  │────▶│  ストレージ  │
└─────────────────┘     └──────────────┘     └──────────────┘
                               │
                    ┌──────────┴──────────┐
                    │                     │
              ┌─────▼──────┐        ┌─────▼─────┐
              │   HTTP     │        │  キュー   │
              │クライアント│        │マネージャ │
              └─────┬──────┘        └───────────┘
                    │
              ┌─────▼─────┐
              │  ページ   │
              │プロセッサ │
              └─────┬─────┘
                    │
              ┌─────▼─────┐
              │   HTML    │
              │ パーサー  │
              └───────────┘
```

## 実装詳細

### 1. 設定管理

**パッケージ**: `internal/config`

設定システムは階層的な優先順位に従います：
1. CLIフラグ（最優先）
2. 環境変数（LT_*）
3. 設定ファイル（linktadoru.yml）
4. デフォルト値（最低優先）

```go
type CrawlConfig struct {
    SeedURLs            []string
    Concurrency         int
    RequestDelay        float64       // 秒
    RequestTimeout      time.Duration
    UserAgent           string
    IgnoreRobotsTxt     bool
    FollowExternalHosts bool
    Limit               int
    MaxResponseSize     int64         // バイト
    Auth                *Auth
    IncludePatterns     []string
    ExcludePatterns     []string
    AllowedSchemes      []string
    Headers             []string
    DatabasePath        string
    // ... ロギング設定 (LogLevel, LogFile, ...)
}
```

### 2. クローラーエンジン

**パッケージ**: `internal/crawler`

クローラーは統合SQLiteベースのキューシステムを持つワーカープールパターンを実装：

- **統合Pagesテーブル**: 単一テーブルがキューと結果ストレージを兼用
- **ワーカープール**: 設定可能な数の並行ワーカー
- **ステータスベース管理**: ステータスカラムによる包括的なライフサイクル追跡
- **レート制限**: ドメイン別のトークンバケットアルゴリズム
- **排他制御**: アトミックなSQLクエリによるマルチプロセス安全性
- **重複防止**: データベースレベルでのURL一意性保証

#### ワーカーライフサイクル

1. 統合pagesテーブルからアトミックにURLを取得
2. robots.txt準拠を確認
3. レート制限を適用
4. ページをフェッチして処理
5. ページレコードをクロール結果で更新
6. リンクを抽出して新しいURLをキューに追加
7. ページを完了としてマーク

#### 統合キューアーキテクチャ

pagesテーブルは二重の目的を果たします：

**キュー管理:**
- クロール済みページから発見されたリンクは`status='discovered'`で記録される（グラフのノードであり、クロール対象ではない）
- クロール対象に選ばれたURL（シード、またはinclude/excludeフィルタを通過した発見リンク）は`status='pending'`に昇格
- ワーカーがアトミックにアイテムを取得: `pending` → `processing`
- 完了時の更新: `processing` → `completed`、`skipped`（robots.txt）、または `error`
- 注: `completed` は「取得が完了した」ことを意味する。404などのHTTPエラーも`completed`となり、結果は`status_code`カラムに記録される

**結果ストレージ:**
- クロール結果フィールドは処理されるまで`NULL`
- アトミックな更新でデータ一貫性を保証
- ビューが分析用のクリーンなインターフェースを提供

#### 排他制御メカニズム

キューの排他制御は単一のアトミックSQLクエリで実現：

```sql
UPDATE pages 
SET status = 'processing', processing_started_at = ? 
WHERE id = (
    SELECT id FROM pages 
    WHERE status = 'pending' 
    ORDER BY added_at ASC 
    LIMIT 1
) AND status = 'pending'
RETURNING id, url
```

**主要な利点:**
- **重複URL防止**: `INSERT OR IGNORE`でキューの汚染を防止
- **競合状態の防止**: アトミック操作で排他アクセスを保証
- **プロセス間安全性**: SQLiteトランザクションベースの自動ロック
- **高性能**: 単一クエリでの取得と更新
- **状態追跡**: 明確な遷移: `discovered` → `pending` → `processing` → `completed`/`skipped`/`error`
- **再開可能性**: 永続的な状態でプロセス中断を生き延びる

#### リトライ処理

リトライはHTTPクライアントではなく、クロールループとストレージレイヤーが担います：

- キューが空になった後、一時的な通信障害とHTTP 408、429、500、502、503、504の応答を試行上限まで再キューする
- 試行回数は`retry_count`カラムで追跡され、実行をまたいでURLごとに合計3回まで
- 追加のバックオフはなく`Retry-After`も解釈しないが、通常のドメイン別request delayとrobots.txtの`Crawl-delay`は適用される
- 決定的な失敗（不正なURLなど）は意図的にリトライしない

### 3. HTTPクライアント

**パッケージ**: `internal/crawler/http_client.go`

機能：
- カスタムUser-Agentサポート
- 設定可能なタイムアウト
- コネクションプーリング
- レスポンスサイズ制限
- パフォーマンスメトリクスの収集（TTFB、ダウンロード時間）

HTTPクライアント自体はリクエストごとに1回だけフェッチを行います。リトライはクロールレベルで処理されます（上記「リトライ処理」参照）。

### 4. HTMLパーサー

**パッケージ**: `internal/parser`

抽出内容：
- タイトルタグ
- メタ説明
- メタロボットディレクティブ
- 正規URL
- すべてのリンク（href属性）
- 重複検出用のコンテンツ

堅牢なHTMLパースのために`golang.org/x/net/html`を使用。

### 5. ストレージレイヤー

**パッケージ**: `internal/storage`

SQLiteベースのストレージ：
- コネクションプーリング
- プリペアードステートメント
- トランザクションサポート
- 並行アクセス処理
- インデックス最適化

#### データベーススキーマ

正式なスキーマ定義は [`internal/storage/schema.go`](../internal/storage/schema.go) にあり、初回実行時に自動的に作成されます。SQLをここに複製する代わりに、その背後にある設計判断を示します：

- **統合`pages`テーブル**: URLごとに1行がキューエントリとクロール結果を兼ねる。`status`カラムが上述のライフサイクルを駆動し、クロール結果カラムは取得完了まで`NULL`のまま。
- **HTTPヘッダーのJSON格納と生成カラム**: レスポンスヘッダー全体を`response_http_headers`（JSON）に一度だけ保存。頻繁に照会されるヘッダー — `content_type`、`content_length`、`last_modified`、`server`、`content_encoding`、`x_cache` — は`GENERATED ALWAYS ... STORED`カラムとして公開され、書き込みロジックを複製せずに通常のカラム同様にインデックス・照会できる。
- **正規化されたリンクグラフ**: `link_relations`はエッジをページIDのペアとして保存し、`UNIQUE(source_page_id, target_page_id)`制約を持つ（同じリンクが異なるアンカーテキストで複数回見つかった場合、最初のもののみ保持）。`links`ビューがエッジをURLペアとして再公開し、分析を容易にする。
- **分析ビュー**: `completed_pages`（取得済みページのみ）と`queue_status`（ステータス別件数と最古/最新タイムスタンプ）が統合テーブルへの安定した照会インターフェースを提供。
- **サポートテーブル**: `crawl_errors`は診断用にすべてのエラー発生を記録し、`crawl_meta`はキー・バリュー形式のクロールメタデータを保存。
- **インデックス戦略**: キュー操作は`status`および`(status, added_at)`のインデックスで支え、分析用カラムには部分インデックス（例: `WHERE content_hash IS NOT NULL`）を用いてキューへの書き込みを軽量に保つ。

### 6. レートリミッター

**パッケージ**: `internal/crawler/rate_limiter.go`

実装：
- ドメイン別レート制限
- トークンバケットアルゴリズム
- 設定可能な遅延
- ノンブロッキング設計

robots.txtの`Crawl-delay`ディレクティブは、設定されたリクエスト遅延より遅い場合に適用され、上限は60秒です。
