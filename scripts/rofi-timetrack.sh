#!/usr/bin/env bash
set -euo pipefail

API="${TIMETRACK_API:-http://localhost:7332/api}"
SCRIPT="$0"

# ── deps ────────────────────────────────────────────────────────────────────────

if ! command -v jq &>/dev/null; then
  notify-send "timetrack" "jq is required but not installed"
  exit 1
fi

# Current project ID (empty = root view)
CURRENT="${1:-}"

# ── helpers ─────────────────────────────────────────────────────────────────────

fmt_dur() {
  local t=$1
  local h=$(( t / 3600 ))
  local m=$(( (t % 3600) / 60 ))
  if (( h > 0 )); then
    printf "%dh %dm" "$h" "$m"
  else
    printf "%dm" "$m"
  fi
}

api_get() {
  curl -sSf "$API/$1" 2>/dev/null || echo 'null'
}

api_post() {
  curl -sSf -X POST -H "Content-Type: application/json" -d "$2" "$API/$1" 2>/dev/null || true
}

# Find a project node anywhere in the tree by ID
find_node() {
  local tree="$1"
  local target="$2"
  echo "$tree" | jq --argjson id "$target" -c '[.. | objects | select(has("id") and .id == $id)] | first // empty'
}

# Build breadcrumb path for prompt
build_breadcrumb() {
  local tree="$1"
  local target="$2"
  local parts=()
  local cur="$target"
  while [[ -n "$cur" && "$cur" != "null" ]]; do
    local node
    node=$(find_node "$tree" "$cur")
    [[ -z "$node" || "$node" == "null" ]] && break
    parts=("$(echo "$node" | jq -r '.name')" "${parts[@]}")
    cur=$(echo "$node" | jq -r '.parent_id // empty')
  done
  local IFS=' > '
  echo "${parts[*]}"
}

# ── fetch state ─────────────────────────────────────────────────────────────────

status_json=$(api_get "status")
projects_json=$(api_get "projects")

active_id=$(echo "$status_json" | jq -r '.session.project_id // empty')
active_path=$(echo "$status_json" | jq -r '.path // empty')

# ── if tracking is active, just stop ────────────────────────────────────────────

if [[ -n "$active_id" && -z "$CURRENT" ]]; then
  note=$(echo "" | rofi -dmenu -m primary -p "Note for ${active_path:-session} (optional):" || true)
  if [[ -n "$note" || $? -eq 0 ]]; then
    api_post "track/stop" "{\"note\":\"${note:-}\"}"
  fi
  exit 0
fi

# ── resolve current context ─────────────────────────────────────────────────────

if [[ -z "$CURRENT" ]]; then
  current_name="Track"
  current_id=""
  children_json=$(echo "$projects_json" | jq -c '. // []')
  parent_id=""
else
  current_node=$(find_node "$projects_json" "$CURRENT")
  if [[ -z "$current_node" || "$current_node" == "null" ]]; then
    exec "$SCRIPT"
  fi
  current_name=$(echo "$current_node" | jq -r '.name')
  current_id="$CURRENT"
  children_json=$(echo "$current_node" | jq -c '.children // []')
  parent_id=$(echo "$current_node" | jq -r '.parent_id // empty')
fi

# ── build menu ──────────────────────────────────────────────────────────────────

items=()
actions=()

# 1. Track at this level (only inside a project)
if [[ -n "$current_id" ]]; then
  total=$(echo "$current_node" | jq -r '.total_seconds // 0')
  items+=("* ${current_name} ($(fmt_dur "$total"))")
  actions+=("track:$current_id")
fi

# 2. Children — always drill down, even if leaf
child_count=$(echo "$children_json" | jq 'length')
for (( i=0; i<child_count; i++ )); do
  child=$(echo "$children_json" | jq -c ".[$i]")
  cid=$(echo "$child" | jq -r '.id')
  cname=$(echo "$child" | jq -r '.name')
  ctotal=$(echo "$child" | jq -r '.total_seconds // 0')
  items+=("- ${cname} ($(fmt_dur "$ctotal"))")
  actions+=("drill:$cid")
done

# 3. New project / child
if [[ -z "$current_id" ]]; then
  items+=("+ New project")
  actions+=("new:root")
else
  items+=("+ New child")
  actions+=("new:$current_id")
fi

# 4. Back
if [[ -n "$current_id" ]]; then
  items+=("← Back")
  if [[ -z "$parent_id" || "$parent_id" == "null" ]]; then
    actions+=("back:root")
  else
    actions+=("back:$parent_id")
  fi
fi

# ── show rofi ───────────────────────────────────────────────────────────────────

prompt="▶ ${current_name}"
if [[ -n "$current_id" ]]; then
  bc=$(build_breadcrumb "$projects_json" "$current_id")
  prompt="▶ ${bc}"
fi

selection=$(printf '%s\n' "${items[@]}" | rofi -dmenu -m primary -p "$prompt" -i -selected-row 0 || true)

if [[ -z "$selection" ]]; then
  exit 0
fi

# find matching action
idx=-1
for i in "${!items[@]}"; do
  if [[ "${items[$i]}" == "$selection" ]]; then
    idx=$i
    break
  fi
done

[[ "$idx" -lt 0 ]] && exit 0
action="${actions[$idx]}"

# ── act ─────────────────────────────────────────────────────────────────────────

case "$action" in
  track:*)
    pid="${action#track:}"
    api_post "track/start" "{\"project_id\":$pid}"
    ;;

  drill:*)
    pid="${action#drill:}"
    exec "$SCRIPT" "$pid"
    ;;

  new:root)
    name=$(echo "" | rofi -dmenu -p "New project name:" || true)
    if [[ -n "$name" ]]; then
      api_post "projects" "{\"name\":\"$name\",\"parent_id\":null}"
      exec "$SCRIPT"
    fi
    ;;

  new:*)
    pid="${action#new:}"
    name=$(echo "" | rofi -dmenu -p "New child name:" || true)
    if [[ -n "$name" ]]; then
      api_post "projects" "{\"name\":\"$name\",\"parent_id\":$pid}"
      exec "$SCRIPT" "$current_id"
    fi
    ;;

  back:root)
    exec "$SCRIPT"
    ;;

  back:*)
    parent="${action#back:}"
    exec "$SCRIPT" "$parent"
    ;;
esac
