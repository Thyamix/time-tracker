#!/usr/bin/env bash
set -euo pipefail

API="${TIMETRACK_API:-http://localhost:7332/api}"

# Waybar expects a single line of JSON on stdout.
# If the server is down, emit a safe fallback.
curl -sSf "$API/status" 2>/dev/null || echo '{"text":"⏱ off","tooltip":"timetrack not running","class":"inactive"}'
