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
- **Robots.txt準拠**: robots.txtルールとクロール遅延を尊重
- **SQLiteストレージ**: クエリ可能なSQLiteデータベースに全データを保存
- **再開可能**: 中断されたセッション用の永続キュー
- **柔軟な設定**: CLIフラグ、環境変数、設定ファイル対応

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
./linktadoru --config mysite.yaml https://httpbin.org
```

## ドキュメント

- 📖 **[基本的な使用法](docs/basic-usage.ja.md)** - コマンドライン使用法と例
- 🔧 **[設定](docs/configuration.md)** - すべての設定オプション（英語）
- 🏗️ **[技術詳細](docs/technical-specification.ja.md)** - アーキテクチャと内部構造
- 🚀 **[開発](docs/development.md)** - ビルドと貢献方法（英語）

## 設定

```yaml
# config.yaml
concurrency: 10
request_delay: 1s
user_agent: "MyBot/1.0"
respect_robots: true
database_path: "./crawl.db"
```

または環境変数を使用：
```bash
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
./linktadoru https://httpbin.org
```

## 貢献

ガイドラインについては[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

## ライセンス

Apache License 2.0 - 詳細は[LICENSE](LICENSE)ファイルを参照。