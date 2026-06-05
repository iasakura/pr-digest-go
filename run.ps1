# run.ps1 — daily-digest を実行する Windows (PowerShell) 向けラッパー。
# このファイルをコピーして秘密情報を埋め、タスクスケジューラ / 手動から呼び出してください。
#   Copy-Item run.ps1 run.local.ps1   # run.local.ps1 は .gitignore 推奨
#   .\run.local.ps1
$ErrorActionPreference = 'Stop'

# === Obsidian 出力先 ===
if (-not $env:OBSIDIAN_VAULT) { $env:OBSIDIAN_VAULT = "$HOME\Obsidian\MyVault" }
if (-not $env:DIGEST_SUBDIR)  { $env:DIGEST_SUBDIR  = "DailyDigest" }

# === タイムゾーン（既定: Asia/Tokyo）===
if (-not $env:DIGEST_TZ) { $env:DIGEST_TZ = "Asia/Tokyo" }

# === アカウント1（個人）===
$env:GH_LABEL_1 = "personal"
$env:GH_LOGIN_1 = "your-personal-login"
$env:GH_TOKEN_1 = "ghp_xxxxxxxxxxxxxxxxxxxx"

# === アカウント2（もう一つの個人）===
$env:GH_LABEL_2 = "side"
$env:GH_LOGIN_2 = "your-second-login"
$env:GH_TOKEN_2 = "ghp_yyyyyyyyyyyyyyyyyyyy"

# === Claude CLI のパス（PATH に無い場合のみ指定）===
# $env:CLAUDE_BIN = "C:\Users\you\.local\bin\claude.exe"

# 特定日を対象にする場合（省略時は本日）:
# $env:DIGEST_DATE = "2026-06-03"

& (Join-Path $PSScriptRoot "daily-digest.exe")
