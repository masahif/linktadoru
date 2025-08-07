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
    SeedURLs        []string      
    Concurrency     int           
    RequestDelay    time.Duration 
    RequestTimeout  time.Duration 
    UserAgent       string        
    RespectRobots   bool          
    IncludePatterns []string      
    ExcludePatterns []string      
    DatabasePath    string        
    Limit           int
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
- URLは`status='queued'`で開始
- ワーカーがアトミックにアイテムを取得: `queued` → `processing`
- 完了時の更新: `processing` → `completed` または `error`

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
    WHERE status = 'queued' 
    ORDER BY added_at ASC 
    LIMIT 1
) AND status = 'queued'
RETURNING id, url
```

**主要な利点:**
- **重複URL防止**: `INSERT OR IGNORE`でキューの汚染を防止
- **競合状態の防止**: アトミック操作で排他アクセスを保証
- **プロセス間安全性**: SQLiteトランザクションベースの自動ロック
- **高性能**: 単一クエリでの取得と更新
- **状態追跡**: 明確な遷移: `queued` → `processing` → `completed`/`error`
- **再開可能性**: 永続的な状態でプロセス中断を生き延びる

### 3. HTTPクライアント

**パッケージ**: `internal/crawler/http_client.go`

機能：
- カスタムUser-Agentサポート
- 設定可能なタイムアウト
- 指数バックオフによる自動リトライ
- コネクションプーリング
- レスポンスサイズ制限
- パフォーマンスメトリクスの収集（TTFB、ダウンロード時間）

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

**統合Pagesテーブル（キュー＋結果）:**

```sql
-- Pagesテーブルはキューと結果ストレージを兼用
CREATE TABLE pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT UNIQUE NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'completed', 'error')),
    
    -- キュー関連フィールド
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processing_started_at DATETIME,
    
    -- クロール結果フィールド（クロール完了まではNULL）
    status_code INTEGER,
    title TEXT,
    meta_description TEXT,
    meta_robots TEXT,
    canonical_url TEXT,
    content_hash TEXT,
    ttfb_ms INTEGER,
    download_time_ms INTEGER,
    response_size_bytes INTEGER,
    content_type TEXT,
    content_length INTEGER,
    last_modified DATETIME,
    server TEXT,
    content_encoding TEXT,
    crawled_at DATETIME,
    
    -- エラー追跡
    retry_count INTEGER DEFAULT 0,
    last_error_type TEXT,
    last_error_message TEXT
);
```

**サポートテーブル:**

```sql
-- リンクテーブル  
CREATE TABLE links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_url TEXT NOT NULL,
    target_url TEXT NOT NULL,
    anchor_text TEXT,
    link_type TEXT,
    rel_attribute TEXT,
    crawled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_url, target_url)
);

-- 詳細エラー追跡用の別テーブル
CREATE TABLE crawl_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    error_type TEXT NOT NULL,
    error_message TEXT,
    occurred_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- メタデータテーブル
CREATE TABLE crawl_meta (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);
```

**最適化インデックス:**

```sql
-- キュー操作用の重要インデックス
CREATE INDEX idx_pages_status ON pages(status);
CREATE INDEX idx_pages_status_added ON pages(status, added_at);
CREATE INDEX idx_pages_url ON pages(url);

-- 完了したデータのみの条件付きインデックス
CREATE INDEX idx_pages_content_hash ON pages(content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX idx_pages_status_code ON pages(status_code) WHERE status = 'completed';
```

**分析ビュー:**

```sql
-- 完了ページのみのビュー（分析・レポート用）
CREATE VIEW completed_pages AS
SELECT id, url, status_code, title, meta_description, meta_robots,
       canonical_url, content_hash, ttfb_ms, download_time_ms,
       response_size_bytes, content_type, content_length,
       last_modified, server, content_encoding, crawled_at
FROM pages WHERE status = 'completed';

-- キュー管理ビュー
CREATE VIEW queue_status AS
SELECT status, COUNT(*) as count,
       MIN(added_at) as oldest_item,
       MAX(added_at) as newest_item
FROM pages GROUP BY status;
```

### 6. レートリミッター

**パッケージ**: `internal/crawler/rate_limiter.go`

実装：
- ドメイン別レート制限
- トークンバケットアルゴリズム
- 設定可能な遅延
- ノンブロッキング設計
