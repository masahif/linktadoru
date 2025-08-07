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
ignore_robots: false
database_path: "./linktadoru.db"
limit: 0                    # 0 = 無制限

# URL フィルタリング
include_patterns: []
exclude_patterns:
  - "\.pdf$"
  - "/admin/.*"

# 認証（いずれか一つの方法を選択）
auth:
  type: "basic"             # "basic"、"bearer"、または"api-key"
  basic:
    username: "user"
    password: "pass"

# カスタムHTTPヘッダー
headers:
  - "Accept: application/json"
  - "X-Custom-Header: value"
```

### 環境変数

すべての設定は `LT_` プレフィックス付きの環境変数で設定可能です：

```bash
# 基本設定
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.5
export LT_IGNORE_ROBOTS=true

# 階層設定（アンダースコアを使用）
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass

# HTTPヘッダー
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_X_CUSTOM="value"

./linktadoru https://httpbin.org
```

## 認証

LinkTadoruは保護されたリソースにアクセスするための複数の認証方式をサポートしています。

### Basic認証

```bash
# CLIフラグ
./linktadoru --auth-type basic --auth-username user --auth-password pass https://protected.httpbin.org

# 環境変数（推奨）
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass
./linktadoru https://protected.httpbin.org
```

### Bearerトークン認証

```bash
# CLIフラグ
./linktadoru --auth-type bearer --auth-token "your-bearer-token" https://api.example.com

# 環境変数（推奨）
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN=your-bearer-token-here
./linktadoru https://api.example.com
```

### APIキー認証

```bash
# CLIフラグ
./linktadoru --auth-type api-key --auth-header "X-API-Key" --auth-value "your-key" https://api.example.com

# 環境変数（推奨）
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER=X-API-Key
export LT_AUTH_APIKEY_VALUE=your-api-key-here
./linktadoru https://api.example.com
```

### 設定ファイル

```yaml
# linktadoru.yml
auth:
  type: "bearer"
  bearer:
    token: "your-token-here"
    # または環境変数を使用:
    # token_env: "MY_BEARER_TOKEN"
```

## カスタムHTTPヘッダー

すべてのリクエストにカスタムHTTPヘッダーを設定：

```bash
# CLIフラグ
./linktadoru -H "Accept: application/json" -H "X-Custom: value" https://api.example.com

# 環境変数
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_X_API_VERSION="v1"
./linktadoru https://api.example.com
```

**セキュリティ注意事項**: セキュリティ上の理由から、設定ファイルに認証情報を保存するのではなく、環境変数を使用することを推奨します。

## 貢献

ガイドラインについては[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

## ライセンス

Apache License 2.0 - 詳細は[LICENSE](LICENSE)ファイルを参照。