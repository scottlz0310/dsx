FROM mcr.microsoft.com/devcontainers/go:2-1.24

# [Workaround] YarnのGPG鍵期限切れエラーにより apt-get update が失敗する問題を回避
# Yarnのリポジトリ定義を削除します（このプロジェクトはGo言語メインのため影響はありません）
RUN rm -f /etc/apt/sources.list.d/yarn.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
    # 必要であればここに追加のパッケージを記述できます
    && apt-get clean -y && rm -rf /var/lib/apt/lists/*
