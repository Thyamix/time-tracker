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
- **Sway (optional):** Automatically track projects assigned to the focused workspace.

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
| `SWAYSOCK` | Auto-discover the sole Sway IPC socket in the runtime directory |

## Sway / Hyprland integration

### Optional Sway workspace tracking

Global mode keeps manual project start / stop behavior. To use automatic tracking in Sway, open the browser Dashboard, find **Tracking settings**, select **Sway workspaces**, and save your assignments. For example:

| Project | Workspaces |
|---------|------------|
| A | `8,9` |
| B | `6,7` |

Leave workspaces `1` through `5` unassigned. Time on `8` or `9` then goes to A, time on `6` or `7` goes to B, and time on `1` through `5` is not tracked. Save settings to apply the mode and mappings. If tracking is paused, use **Resume tracking** to enable it. Assignments stay saved across pauses, restarts, and mode changes.

Numeric keys match Sway's workspace number, so `8` also matches `8:code`. You can also map an exact workspace name, including a purely named workspace. If both a numeric key and an exact name match, the numeric assignment wins.

Only the focused workspace counts, including on multi-monitor setups. Visible workspaces on other outputs do not track in parallel. Switching to another project's workspace closes the current session segment and opens one for that project. An unassigned workspace closes the current segment without starting another. Returning starts a new segment; reports and project totals add those segments together. Moving between workspaces assigned to the same project keeps the current segment running.

**Pause all tracking** stops workspace tracking everywhere, even when the focused workspace is unassigned. **Resume tracking** keeps the mappings and starts tracking only if the focused workspace is assigned. Manual project starts are unavailable in Sway mode. Switching back to **Global / manual** closes the current Sway segment but does not start a global session. Start a project manually when ready.

The backend connects directly to Sway IPC, listens for workspace events, and checks the focused workspace every second. It closes tracking at the last checkpoint on disconnect and excludes suspended time and server downtime. It reconnects when Sway becomes available. This is workspace-based tracking, not idle or lock detection; an assigned workspace can keep tracking while you are away or the screen is locked.

The server uses `SWAYSOCK` when set, otherwise it automatically discovers a single Sway socket in the runtime directory. If multiple Sway sessions leave multiple sockets, explicitly pass the intended socket to the systemd user service. Run this from the intended Sway session:

```bash
systemctl --user import-environment SWAYSOCK
systemctl --user restart timetrack
```

If needed, add the same commands to your Sway startup configuration:

```sway
exec sh -c 'systemctl --user import-environment SWAYSOCK && systemctl --user restart timetrack'
```

Hyprland can use the manual Rofi and Waybar controls below, but automatic workspace tracking requires Sway.

### Waybar module

Add to `~/.config/waybar/config`:

```json
"custom/timetrack": {
    "exec": "~/.local/bin/waybar-timetrack",
    "interval": 1,
    "return-type": "json",
    "on-click": "~/.local/bin/rofi-timetrack",
    "format": "{}"
}
```

Add to `~/.config/waybar/style.css`:

```css
#custom-timetrack {
    padding: 0 10px;
}
#custom-timetrack.active {
    color: #3fb950;
}
#custom-timetrack.inactive {
    color: #8b949e;
}
```

Then add `"custom/timetrack"` to your Waybar modules array. The one-second interval lets the bar follow tracking changes within about a second.

While tracking, the module shows the current project's total for today, followed by its all-time total in parentheses. Today starts at the server's local midnight; sessions crossing midnight count only time spent today. Paused workspace tracking displays `Tracking paused`.

### Rofi keybind

The setup script installs the extensionless helper names used above. For a manual install, run from the repository root:

```bash
mkdir -p ~/.local/bin
ln -s "$(pwd)/scripts/waybar-timetrack.sh" ~/.local/bin/waybar-timetrack
ln -s "$(pwd)/scripts/rofi-timetrack.sh" ~/.local/bin/rofi-timetrack
```

Rofi controls require `rofi`, `curl`, and `jq`. Desktop error notifications use `notify-send`, and opening settings uses `xdg-open`. Set `TIMETRACK_API` for a different API address, such as `http://localhost:7332/api`.

**Sway** (`~/.config/sway/config`):

```sway
bindsym $mod+Alt+t exec ~/.local/bin/rofi-timetrack
```

**Hyprland** (`~/.config/hypr/hyprland.conf`):

```hyprland
bind = SUPER ALT, T, exec, ~/.local/bin/rofi-timetrack
```

**Rofi behaviour:**

- In global mode, if nothing is tracking, the menu opens to pick a project. Drill down through the tree; `*` tracks the current level.
- If a global session is active, the keybind opens an optional note prompt. Enter stops tracking, including with an empty note. Escape cancels without stopping.
- In Sway mode, the root menu offers pause / resume for all workspace tracking, assign the focused workspace, unassign it, and open Dashboard settings in the browser. It never starts a manual global session.
- Assignment uses the same project tree. Select `* Assign to ...` at any level to save that project for the workspace captured when the picker opened. Numbered workspaces use their numeric key; purely named workspaces use their exact name. Assigning does not resume paused tracking.
- Unassign removes both the numeric mapping and any exact-name fallback for the focused workspace. Other assignments stay unchanged.
- Canceling a menu makes no changes. Failed saves report an error rather than appearing to succeed.

## License

MIT — see [LICENSE](LICENSE).
