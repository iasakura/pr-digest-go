#!/usr/bin/env bash
# run.sh — daily-digest を実行するラッパー。
# このファイルをコピーして秘密情報を埋め、cron / launchd / 手動から呼び出してください。
set -euo pipefail

# === Obsidian 出力先 ===
export OBSIDIAN_VAULT="${OBSIDIAN_VAULT:-$HOME/Obsidian/MyVault}"
export DIGEST_SUBDIR="${DIGEST_SUBDIR:-DailyDigest}"

# === タイムゾーン（既定: Asia/Tokyo）===
export DIGEST_TZ="${DIGEST_TZ:-Asia/Tokyo}"

# === アカウント1（個人）===
export GH_LABEL_1="personal"
export GH_LOGIN_1="your-personal-login"
export GH_TOKEN_1="ghp_xxxxxxxxxxxxxxxxxxxx"

# === アカウント2（もう一つの個人）===
export GH_LABEL_2="side"
export GH_LOGIN_2="your-second-login"
export GH_TOKEN_2="ghp_yyyyyyyyyyyyyyyyyyyy"

# === Claude CLI のパス（PATH に無い場合のみ指定）===
# export CLAUDE_BIN="/usr/local/bin/claude"

# 特定日を対象にする場合（省略時は本日）:
# export DIGEST_DATE="2026-06-03"

exec "$(dirname "$0")/daily-digest"
