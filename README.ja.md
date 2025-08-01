# LinkTadoru

Go言語で構築された高性能なWebクローラーおよびリンク分析ツールです。LinkTadoruはWebサイトの構造を発見・分析し、メタデータを抽出し、サイトアーキテクチャとコンテンツの関連性を理解するためにリンク関係をマッピングします。

## 機能

- **高速並行クロール**: 並列処理のための設定可能なワーカープール
- **包括的なデータ収集**: ページタイトル、メタデータ、正規URL、構造化データを抽出
- **リンク分析**: アンカーテキストと共に内部・外部リンクの関係をマッピング
- **パフォーマンスメトリクス**: TTFB、ダウンロード時間、レスポンスサイズを追跡
- **Robots.txt準拠**: robots.txtルールとクロール遅延を尊重
- **レート制限**: サーバー過負荷を防ぐための組み込みレートリミッター
- **SQLiteストレージ**: すべてのデータはクエリ可能なSQLiteデータベースに保存
- **永続キュー**: 中断・再開可能な永続化されたクロールキュー
- **排他制御**: マルチプロセス環境での安全な並行処理
- **重複検出**: コンテンツハッシュベースの重複ページ検出
- **柔軟な設定**: CLIフラグ、環境変数、または設定ファイル

## インストール

### ソースから

```bash
# リポジトリをクローン
git clone https://github.com/fukuda-deltax/linktadoru.git
cd linktadoru

# Makeを使用してビルド（推奨）
make build

# またはGoで直接ビルド
go build -o linktadoru ./cmd/crawler

# グローバルにインストール
make install
```

### クロスプラットフォームビルド

```bash
# すべてのプラットフォーム用にビルド
make build-all

# 特定のプラットフォーム用にビルド
make build-linux    # Linux (amd64, arm64)
make build-darwin   # macOS (Intel, Apple Silicon)  
make build-windows  # Windows (amd64)

# リリースアーカイブを作成
make release
```


### 要件

- Go 1.23以上
- SQLite3（Go SQLiteドライバーに含まれています）

## 使用方法

### 基本的な使用方法

```bash
# 単一のWebサイトをクロール
./linktadoru https://example.com

# 複数のシードURLをクロール
./linktadoru https://example.com https://blog.example.com

# カスタム設定で実行
./linktadoru --max-pages 5000 --concurrency 5 --delay 2s https://example.com
```

### 設定オプション

クローラーは以下の方法で設定できます：
1. コマンドラインフラグ（最優先）
2. 環境変数（プレフィックス: `LT_`）
3. 設定ファイル（`config.yaml`）
4. デフォルト値

#### コマンドラインフラグ

```bash
./linktadoru --help

フラグ:
  --include-patterns strings   含めるURLの正規表現パターン
  --batch-limit int           N個のページで停止（0=無制限）
  -c, --concurrency int       並行ワーカー数（デフォルト 10）
  --config string             設定ファイルパス（デフォルト "./config.yaml"）
  -d, --database string       SQLiteデータベースパス（デフォルト "./crawl.db"）
  -r, --delay duration        リクエスト間の遅延（デフォルト 1s）
  --exclude-patterns strings  除外するURLの正規表現パターン
  --ignore-robots             robots.txtルールを無視
  -d, --max-depth int         最大クロール深度（デフォルト 5）
  -p, --max-pages int         最大クロールページ数（デフォルト 10000）
  --max-retries int           最大リトライ回数（デフォルト 3）
  -o, --output string         出力ディレクトリ（デフォルト "./output"）
  --timeout duration          HTTPリクエストタイムアウト（デフォルト 30s）
  -u, --user-agent string     HTTP User-Agent（デフォルト "LinkTadoru/1.0"）
```

#### 設定ファイル

`config.yaml`ファイルを作成:

```yaml
# 基本的なクロールパラメータ
concurrency: 10
request_delay: 1s
request_timeout: 30s
user_agent: "LinkTadoru/1.0"
respect_robots: true
limit: 0

# URLフィルタリング
include_patterns:
  - "^https?://[^/]*example\\.com/.*"
  - "^https?://[^/]*subdomain\\.example\\.com/.*"

exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"
  - ".*#.*"

# ストレージ設定
database_path: "./crawl.db"
```

#### 環境変数

すべての設定オプションは環境変数で設定可能:

```bash
export LT_LIMIT=5000
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
./linktadoru https://example.com
```

## データベーススキーマ

クローラーは統合SQLiteデータベース設計を使用し、以下の構造を持ちます：

### 統合Pagesテーブル（キュー + 結果）
- **キュー管理**: ステータス追跡付きURL（`queued` → `processing` → `completed`/`error`）
- **クロール結果**: タイトル、メタ説明、パフォーマンスメトリクス、HTTPヘッダー
- **重複防止**: データベースレベルでのURL一意性保証
- **NULL処理**: 結果フィールドはクロールされるまでNULL

### Linksテーブル
- ソースとターゲットURLの関係
- アンカーテキストとリンクタイプ（内部/外部）
- rel属性と発見タイムスタンプ

### Crawl Errorsテーブル
- タイプとメッセージによる詳細なエラー追跡
- ページレベルのエラーステータスとは別管理

### 分析ビュー
- **completed_pages**: 成功してクロールされたページのクリーンビュー
- **queue_status**: ステータス別のリアルタイムキュー統計

**主要な利点:**
- キューに重複URLなし
- スレッドセーフなアトミック操作
- 再開可能なクロールのための永続状態
- 専用ビューによる効率的な分析

## 開発

### ソースからのビルド

```bash
# テストを実行
make test

# リンターを実行（golangci-lintが必要）
make lint

# コードをフォーマット
make fmt

# コミット前にすべてのチェックを実行
make check

# ビルド成果物をクリーン
make clean
```

### プロジェクト構造

```
.
├── cmd/crawler/          # メインアプリケーションエントリポイント
├── internal/
│   ├── cmd/             # CLIコマンドハンドリング
│   ├── config/          # 設定管理
│   ├── crawler/         # コアクロールロジック
│   ├── interfaces/      # インターフェース定義
│   ├── parser/          # HTMLパース
│   └── storage/         # データベース操作
├── config.yaml.example  # 設定例
└── go.mod              # Goモジュール定義
```

### テストの実行

```bash
# すべてのテストを実行
go test ./...

# カバレッジ付き
go test -cover ./...

# 特定のパッケージ
go test -v ./internal/crawler
```

### ビルド

```bash
# 現在のプラットフォーム用にビルド
go build -o linktadoru ./cmd/crawler

# Linux用クロスコンパイル
GOOS=linux GOARCH=amd64 go build -o crawler-linux ./cmd/crawler

# Windows用クロスコンパイル
GOOS=windows GOARCH=amd64 go build -o crawler.exe ./cmd/crawler
```

## パフォーマンスの考慮事項

- **並行性**: ターゲットサーバーの容量に基づいて`--concurrency`を調整
- **レート制限**: サーバーを圧倒しないように`--delay`を使用
- **メモリ使用量**: 大規模サイトでは増加したメモリ割り当てが必要な場合があります
- **データベースパフォーマンス**: SQLiteは数百万ページまで良好に動作

## 例

### URLパターンマッチングクロール

```bash
./linktadoru --include-patterns "^https?://[^/]*example\\.com/.*" https://example.com
```

### 特定パターンの除外

```bash
./linktadoru --exclude-patterns "\.pdf$" --exclude-patterns "/search\?" https://example.com
```

### 高パフォーマンスクロール

```bash
./linktadoru --concurrency 20 --delay 500ms --max-pages 50000 https://example.com
```

### Robots.txtを無視

```bash
./linktadoru --ignore-robots https://example.com
```

## トラブルシューティング

### 一般的な問題

1. **「開いているファイルが多すぎます」エラー**
   - システムのファイル記述子制限を増やす: `ulimit -n 4096`

2. **高メモリ使用量**
   - 並行性を減らすかバッチ処理を実装
   - チャンクで処理するために`--batch-limit`を使用

3. **遅いクロール**
   - （サーバーが許可する場合）並行性を増やす
   - リクエスト遅延を減らす
   - ネットワーク接続を確認

### デバッグモード

ログレベルを設定して詳細ログを有効化:

```bash
LOG_LEVEL=debug ./linktadoru https://example.com
```

## 貢献

貢献を歓迎します！プルリクエストをお気軽に送信してください。

1. リポジトリをフォーク
2. フィーチャーブランチを作成（`git checkout -b feature/amazing-feature`）
3. 変更をコミット（`git commit -m 'Add amazing feature'`）
4. ブランチにプッシュ（`git push origin feature/amazing-feature`）
5. プルリクエストを開く

## ライセンス

LinkTadoruはApache License, Version 2.0の下でライセンスされています - 詳細はLICENSEファイルを参照してください。

## 謝辞

- CLI用の[Cobra](https://github.com/spf13/cobra)で構築
- 設定管理に[Viper](https://github.com/spf13/viper)を使用
- [go-sqlite3](https://github.com/mattn/go-sqlite3)によるSQLiteストレージ

