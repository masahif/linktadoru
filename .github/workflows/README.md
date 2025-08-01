# GitHub Actions Workflows

このプロジェクトは以下のワークフローを使用しています：

## CI（ci.yml）
**トリガー**: 
- PRのmainブランチへのマージ
- developブランチへのpush
- 手動実行（workflow_dispatch）

**用途**: 日常開発での品質チェック
- テスト実行
- リンター
- セキュリティスキャン  
- ビルド確認

## Release（release.yml）
**トリガー**: 
- v*タグのpush（例：v1.0.0）

**用途**: リリース時の最終検証とバイナリビルド
- テスト実行（最終確認）
- マルチプラットフォームビルド（Linux, macOS, Windows）
- GitHub Releaseの自動作成
- リリースノート生成

## ローカルテスト（act）

actを使用してローカルでGitHub Actionsをテストできます：

```bash
# actのインストール（macOS）
brew install act

# CIワークフローをローカル実行
act -W .github/workflows/ci.yml

# 特定のジョブのみ実行
act -W .github/workflows/ci.yml -j test

# デバッグモード
act -W .github/workflows/ci.yml --verbose
```

## 推奨フロー

1. **開発時**: developブランチで作業 → CIが自動実行
2. **PR時**: main向けPR作成 → CIが自動実行  
3. **リリース時**: タグ作成 → Releaseワークフローが自動実行

これにより、mainブランチへの直接pushでは不要なCI実行を避け、リリース時のみ完全なテスト+ビルドを実行します。