# LinkTadoru (リンクたどる)

[![Build Status](https://github.com/masahif/linktadoru/actions/workflows/ci.yml/badge.svg)](https://github.com/masahif/linktadoru/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/masahif/linktadoru)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/github/license/masahif/linktadoru)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/masahif/linktadoru)](https://github.com/masahif/linktadoru/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/masahif/linktadoru)](https://goreportcard.com/report/github.com/masahif/linktadoru)

Go言語で構築された高性能Webクローラーおよびリンク解析ツール。

## 特徴

- **高速並行クロール**: 設定可能なワーカープールによる並列処理
- **リンク解析**: 内部・外部リンクの関係性をマッピング
- **複数の認証方式**: Basic認証、Bearerトークン、APIキーに対応
- **カスタムHTTPヘッダー**: リクエスト用カスタムヘッダーの設定
- **Robots.txt準拠**: robots.txtルールとクロール遅延を尊重
- **安全なデフォルト**: シードホスト内に留まり（`follow_external_hosts: false`）、レスポンスボディを10 MiBに制限（`max_response_size`）
- **SQLiteストレージ**: クエリ可能なSQLiteデータベースに全データを保存
- **再開可能**: 中断されたセッション用の永続キュー
- **柔軟な設定**: CLIフラグ、環境変数、または階層設定ファイル対応

## インストール

### バイナリのダウンロード

[リリースページ](https://github.com/masahif/linktadoru/releases)から事前ビルド済みバイナリをダウンロード。

### ソースからビルド

```bash
git clone https://github.com/masahif/linktadoru.git
cd linktadoru
make build
```

必要環境: Go 1.23以上

## クイックスタート

```bash
# Webサイトをクロール
./linktadoru https://httpbin.org

# オプション付き
./linktadoru --limit 100 --concurrency 5 https://httpbin.org

# 設定ファイルを使用
./linktadoru --config linktadoru.yml https://httpbin.org

# 生成したシード一覧の各URLと、その直接リンクをクロール
./linktadoru --seed-file urls.txt --max-depth 1 --limit 0

# 現在の設定を表示
./linktadoru --show-config

# カスタムヘッダーを使用
./linktadoru -H "Accept: application/json" -H "X-Custom: value" https://api.example.com
```

## ドキュメント

- 📖 **[基本的な使用法](docs/basic-usage.ja.md)** - コマンドライン使用法と例
- 🔧 **[設定](docs/configuration.md)** - すべての設定オプション（英語）
- 🏗️ **[技術詳細](docs/technical-specification.ja.md)** - アーキテクチャと内部構造
- 🚀 **[開発](docs/development.md)** - ビルドと貢献方法（英語）

## 設定

LinkTadoruは以下の階層設定優先順位に従います：
1. コマンドライン引数（最高優先度）
2. 環境変数
3. 設定ファイル
4. デフォルト値（最低優先度）

### 設定ファイル

```yaml
# linktadoru.yml
concurrency: 2
request_delay: 0.1           # 秒
user_agent: "LinkTadoru/1.0"
ignore_robots_txt: false
database_path: "./linktadoru.db"
limit: 0                     # 0 = 無制限
max_depth: 0                 # 0 = 無制限、1 = シードと直接リンク

# URL フィルタリング
include_patterns: []
exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"

# カスタムHTTPヘッダー
headers:
  - "Accept: application/json"
  - "X-Custom-Header: value"
```

コメント付きの完全な設定例は [linktadoru.yml.example](linktadoru.yml.example) を、全オプションの一覧は[設定リファレンス](docs/configuration.md)（英語）を参照してください。

### 環境変数

CLIフラグを持つオプションは `LT_` プレフィックス付きの環境変数でも設定できます。ロギング設定（`log_*`）と `allowed_schemes` にはフラグがないため、設定ファイルでのみ指定可能です。

```bash
# 基本設定
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.5
export LT_IGNORE_ROBOTS_TXT=true

# HTTPヘッダー（LT_HEADER_* パターン）
export LT_HEADER_ACCEPT="application/json"

./linktadoru https://httpbin.org
```

## 認証とカスタムヘッダー

Basic認証、Bearerトークン、APIキーの各認証方式と、全リクエストへのカスタムHTTPヘッダー設定に対応しています。例：

```bash
# 環境変数（認証情報には推奨）
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass
./linktadoru -H "Accept: application/json" https://protected.example.com
```

すべての認証方式（CLIフラグ・環境変数・設定ファイル）とヘッダーオプションについては、[設定リファレンス — Authentication](docs/configuration.md#authentication)（英語）を参照してください。

**セキュリティ注意事項**: セキュリティ上の理由から、設定ファイルに認証情報を保存するのではなく、環境変数を使用することを推奨します。

## 貢献

ガイドラインについては[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

## ライセンス

Apache License 2.0 - 詳細は[LICENSE](LICENSE)ファイルを参照。
