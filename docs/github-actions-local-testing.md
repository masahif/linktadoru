# GitHub Actions ローカルテストガイド

## 1. actツールのインストール

`act`はGitHub Actionsをローカルで実行するためのツールです。

### Linux/macOS (Homebrew)
```bash
brew install act
```

### Linux (Binary)
```bash
curl -s https://api.github.com/repos/nektos/act/releases/latest \
  | grep "browser_download_url.*linux_amd64" \
  | cut -d '"' -f 4 \
  | wget -qi -
chmod +x act_*
sudo mv act_* /usr/local/bin/act
```

### Windows (Chocolatey)
```bash
choco install act-cli
```

## 2. ローカルでの実行方法

### 注意事項
⚠️ **Gitリポジトリが必要**: actはGitリポジトリ内で実行する必要があります。プロジェクトをgit initで初期化してください。

```bash
# 最初にGitリポジトリを初期化（必要な場合）
git init
git add .
git commit -m "Initial commit"
```

### 基本的な使用法
```bash
# ワークフローの一覧表示
act --list

# 特定のジョブだけ実行（ドライラン）
act -n -j test

# CIワークフローの実行
act -W .github/workflows/ci.yml

# プッシュイベントをシミュレート
act push

# 特定のジョブの実行
act -j test

# リリースワークフローのテスト（タグイベント）
act -e .github/workflows/release.yml
```

### 環境変数の設定
```bash
# .envファイルを作成
echo "GITHUB_TOKEN=your_token" > .env

# .envファイルを使用して実行
act --env-file .env
```

### シークレットの設定
```bash
# .secretsファイルを作成
echo "GITHUB_TOKEN=your_token" > .secrets

# シークレットファイルを使用
act --secret-file .secrets
```

## 3. 制限事項

- すべてのGitHub Actionsアクションが完全にサポートされているわけではない
- Dockerイメージが必要（初回実行時にダウンロード）
- 一部のGitHub固有の機能は動作しない場合がある

## 4. 推奨ワークフロー

1. **ローカルテスト**: actでbasicな動作確認
2. **ブランチプッシュ**: developブランチで実際のCI確認
3. **プルリクエスト**: mainブランチへのPRでフルテスト
4. **リリース**: タグプッシュでリリースワークフロー

## 5. デバッグ

```bash
# 詳細ログ出力
act --verbose

# ドライラン（実際に実行せずに確認）
act --dryrun

# 特定のプラットフォームを指定
act -P ubuntu-latest=catthehacker/ubuntu:act-latest
```