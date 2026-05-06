# timetrack

A tiny, local-only time tracker for developers who want to know where their hours go. I built this because I needed a simple way to track dev time across various projects and sub-projects without spinning up a database or dealing with SaaS subscriptions.

> **Warning:** This project was vibecoded. Use it at your own risk.

## What it does

- **Projects:** Organise work into a tree (e.g. `Project > Task > Sub Task`).
- **Sessions:** Start / stop tracking with optional notes.
- **Reports:** View stats by day, week, month or all time.
- **Import:** Paste JSON to bulk-import old sessions.
- **Waybar:** Live status in your bar.
- **Rofi:** Keyboard-driven start / stop without touching the browser.

## Stack

Go backend (SQLite), React frontend, single binary.

## Quick setup

```bash
./setup.sh
```

This builds the frontend and backend, installs the binary and helper scripts to `~/.local/bin`, enables the systemd user service, and opens your browser.

## Manual build

```bash
# 1. Build the frontend
cd web
npm install
npm run build
cd ..

# 2. Build the backend
go build -o timetrack ./cmd/server
```

Copy the binary somewhere in your `$PATH`:

```bash
mkdir -p ~/.local/bin
cp timetrack ~/.local/bin/
```

## Run

### Manual

```bash
timetrack
# listens on http://127.0.0.1:7332
```

### Systemd user service

```bash
mkdir -p ~/.config/systemd/user
cp scripts/timetrack.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now timetrack
```

Environment variables:

| Variable  | Default                                |
|-----------|----------------------------------------|
| `PORT`    | `7332`                                 |
| `DB_PATH` | `~/.local/share/timetrack/timetrack.db`|

## Sway / Hyprland integration

### Waybar module

Add to `~/.config/waybar/config`:

```json
"timetrack": {
    "exec": "~/.local/bin/waybar-timetrack.sh",
    "interval": 5,
    "return-type": "json",
    "on-click": "~/.local/bin/rofi-timetrack.sh",
    "format": "{}"
}
```

Add to `~/.config/waybar/style.css`:

```css
#timetrack {
    padding: 0 10px;
}
#timetrack.active {
    color: #3fb950;
}
#timetrack.inactive {
    color: #8b949e;
}
```

Then add `"timetrack"` to your waybar modules array.

### Rofi keybind

```bash
ln -s $(pwd)/scripts/waybar-timetrack.sh ~/.local/bin/waybar-timetrack
ln -s $(pwd)/scripts/rofi-timetrack.sh ~/.local/bin/rofi-timetrack
```

**Sway** (`~/.config/sway/config`):

```sway
bindsym $mod+Alt+t exec ~/.local/bin/rofi-timetrack
```

**Hyprland** (`~/.config/hypr/hyprland.conf`):

```hyprland
bind = SUPER ALT, T, exec, ~/.local/bin/rofi-timetrack
```

**Rofi behaviour:**

- If nothing is tracking, the menu opens to pick a project.
- Drill down through the tree; `*` tracks the current level.
- If a session is already active, the keybind jumps straight to a note prompt and stops tracking.

## License

MIT — see [LICENSE](LICENSE).
