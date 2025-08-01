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

### プロジェクト設定（.actrc）

プロジェクトルートに`.actrc`ファイルが設定済みです:

```bash
# Ubuntu 22.04イメージを使用（互換性向上）
-P ubuntu-latest=catthehacker/ubuntu:act-22.04

# 環境変数設定
--env GO_VERSION=1.23

# gitignoreを使用しない
--use-gitignore=false

# 詳細出力
--verbose
```

### 基本的な使用法
```bash
# ワークフローの一覧表示
act --list

# 推奨: CIワークフローの実行（.actrcが自動適用される）
act -W .github/workflows/ci.yml

# 特定のジョブのみ実行
act -W .github/workflows/ci.yml -j test

# ドライラン（実行せずに確認）
act -W .github/workflows/ci.yml --dryrun

# PRイベントをシミュレート
act pull_request -W .github/workflows/ci.yml
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

## 4. 手動CI実行（workflow_dispatch）

CI戦略が変更され、mainブランチへの直接pushではCIが実行されなくなりました。必要に応じて手動でCIを実行できます。

### GitHub CLI使用
```bash
# mainブランチでCIを手動実行
gh workflow run CI --ref main

# 特定のブランチでCIを実行
gh workflow run CI --ref feature-branch

# 実行状況確認
gh run list --workflow=CI --limit 5
```

### GitHub Web UI使用
1. GitHubリポジトリページ → **Actions**タブ
2. 左サイドバーの**CI**をクリック
3. 右上の**Run workflow**ボタンをクリック
4. ブランチを選択して**Run workflow**

## 5. 推奨ワークフロー

現在のCI戦略（actプロジェクトを参考）:

1. **ローカルテスト**: actでbasic動作確認
2. **プルリクエスト**: mainブランチへのPRで自動CI実行
3. **手動実行**: 必要に応じてworkflow_dispatchでCI実行
4. **リリース**: タグプッシュでリリースワークフロー

### メリット
- **効率性**: 不要なCI実行を削減
- **柔軟性**: 必要な時だけ手動実行
- **品質保証**: PRで確実なレビュー

## 6. デバッグ

```bash
# 詳細ログ出力
act --verbose

# ドライラン（実際に実行せずに確認）
act --dryrun

# 特定のプラットフォームを指定
act -P ubuntu-latest=catthehacker/ubuntu:act-latest
```