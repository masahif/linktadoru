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
│   CLI/設定      │────▶│  クローラー   │────▶│  ストレージ   │
└─────────────────┘     └──────────────┘     └──────────────┘
                               │
                    ┌──────────┴──────────┐
                    │                     │
              ┌─────▼─────┐        ┌─────▼─────┐
              │   HTTP    │        │  キュー   │
              │ クライアント│        │ マネージャ │
              └─────┬─────┘        └───────────┘
                    │
              ┌─────▼─────┐
              │  ページ   │
              │ プロセッサ │
              └─────┬─────┘
                    │
              ┌─────▼─────┐
              │   HTML    │
              │  パーサー  │
              └───────────┘
```

## 実装詳細

### 1. 設定管理

**パッケージ**: `internal/config`

設定システムは階層的な優先順位に従います：
1. CLIフラグ（最優先）
2. 環境変数（LT_*）
3. 設定ファイル（config.yaml）
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

### 7. Robots.txtパーサー

**パッケージ**: `internal/crawler/robots.go`

機能：
- RFC準拠のパース
- User-Agentマッチング
- Crawl-delayサポート
- パフォーマンスのためのキャッシング
- パースエラー時のフォールバック

## パフォーマンス特性

### 並行性モデル

- **ワーカー**: 設定可能 1-100（デフォルト: 10）
- **キューサイズ**: SQLiteベース（制限なし、ディスクベース）
- **メモリ使用量**: 基本約30MB、キューサイズに依存しない
- **排他制御**: アトミックSQLクエリによる効率的なロック

### スループット

期待されるパフォーマンス（サイトにより異なる）：
- 10ワーカー: 約100-200ページ/分
- 20ワーカー: 約200-400ページ/分
- 50ワーカー: 約500-1000ページ/分

### ボトルネック

1. **ネットワークI/O**: 主要な制限要因
2. **データベース書き込み**: 効率のためバッチ処理
3. **HTMLパース**: ストリーミングで最適化
4. **メモリ**: 制限付きキューで無制限な成長を防止

## エラーハンドリング

### リトライ戦略

- ネットワークエラー: 指数バックオフで3回リトライ
- HTTP 5xx: 遅延付きで2回リトライ
- HTTP 429: Retry-Afterヘッダーを尊重
- パースエラー: ログして続行

### 障害モード

1. **接続拒否**: エラーとしてマーク、続行
2. **タイムアウト**: より長いタイムアウトでリトライ
3. **無効なHTML**: 可能な限り抽出
4. **データベースエラー**: ログして回復を試行

## セキュリティの考慮事項

1. **User-Agent**: クローラーを正直に識別
2. **レート制限**: DoS的な動作を防止
3. **Robots.txt**: 除外ルールを尊重
4. **URL検証**: SSRF攻撃を防止
5. **サイズ制限**: メモリ枯渇を防止

## モニタリング

### メトリクス

- 秒あたりのクロールページ数
- タイプ別エラー率
- キューの深さ
- ワーカー使用率
- レスポンスタイムのパーセンタイル

### ロギング

- レベル付き構造化ログ
- ワーカー別識別
- エラーコンテキスト保存
- パフォーマンスメトリクス

## 将来の拡張

### 計画機能

1. **分散クロール**: 複数クローラーの協調
2. **JavaScriptレンダリング**: ヘッドレスブラウザ統合
3. **APIエンドポイント**: リモート制御用REST API
4. **リアルタイムモニタリング**: WebSocketステータス更新
5. **プラグインシステム**: カスタムプロセッサーと抽出器

### パフォーマンス改善

1. **ブルームフィルター**: メモリ効率的な重複検出
2. **圧縮**: ストレージ要件の削減
3. **並列DB書き込み**: スループット向上
4. **キューパーティショニング**: 超大規模サイト対応

## テスト戦略

### ユニットテスト

- モックサーバーでHTTPクライアント
- フィクスチャHTMLでパーサー
- レートリミッターのタイミング検証
- インメモリSQLiteでストレージ

### 統合テスト

- 完全なクロールシミュレーション
- 並行アクセスパターン
- エラー注入
- パフォーマンスベンチマーク

### 負荷テスト

- 高並行性シナリオ
- 大規模サイトクロール
- メモリリーク検出
- データベースパフォーマンス

## 依存関係

### 直接依存関係

- `github.com/spf13/cobra`: CLIフレームワーク
- `github.com/spf13/viper`: 設定管理
- `golang.org/x/net`: HTMLパース
- `github.com/mattn/go-sqlite3`: SQLiteドライバー
- `golang.org/x/time/rate`: レート制限

### 開発依存関係

- `golangci-lint`: コード品質
- `go test`: テストフレームワーク
- `pprof`: パフォーマンスプロファイリング

## デプロイメント

### システム要件

- **OS**: Linux、macOS、Windows
- **RAM**: 最小512MB、推奨2GB以上
- **ディスク**: 大規模クロール用に10GB以上
- **ネットワーク**: 安定したインターネット接続

### 設定チューニング

異なるシナリオ用：

#### 小規模サイト（<1000ページ）
- 並行性: 5
- 遅延: 1秒
- 最大ページ: 1000

#### 中規模サイト（1000-50000ページ）
- 並行性: 10-20
- 遅延: 500ms-1秒
- 最大ページ: 50000

#### 大規模サイト（>50000ページ）
- 並行性: 20-50
- 遅延: 200ms-500ms
- 最大ページ: 無制限
- バッチ処理を推奨