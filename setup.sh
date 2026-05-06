#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> Building frontend..."
cd web
npm install
npm run build
cd ..

echo "==> Building backend..."
go build -o timetrack ./cmd/server

echo "==> Installing binary..."
mkdir -p ~/.local/bin
cp -f timetrack ~/.local/bin/

echo "==> Installing helper scripts..."
ln -sf "$SCRIPT_DIR/scripts/waybar-timetrack.sh" ~/.local/bin/waybar-timetrack
ln -sf "$SCRIPT_DIR/scripts/rofi-timetrack.sh" ~/.local/bin/rofi-timetrack

echo "==> Installing systemd service..."
mkdir -p ~/.config/systemd/user
cp -f "$SCRIPT_DIR/scripts/timetrack.service" ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now timetrack

echo "==> Opening browser..."
xdg-open http://localhost:7332 2>/dev/null || open http://localhost:7332 2>/dev/null || true

echo ""
echo "Done! timetrack is running on http://localhost:7332"
