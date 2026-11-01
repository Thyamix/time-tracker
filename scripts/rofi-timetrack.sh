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
INTENT="${2:-}"
WORKSPACE="${3:-}"

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
  local response
  if response=$(curl -sS --fail-with-body "$API/$1" 2>&1); then
    printf '%s\n' "$response"
  else
    show_error "GET $1 failed: $response"
    return 1
  fi
}

api_post() {
  local response
  if response=$(curl -sS --fail-with-body -X POST -H "Content-Type: application/json" -d "$2" "$API/$1" 2>&1); then
    printf '%s\n' "$response"
  else
    show_error "POST $1 failed: $response"
    return 1
  fi
}

show_error() {
  printf 'timetrack: %s\n' "$1" >&2
  notify-send "timetrack" "$1" 2>/dev/null || true
}

save_sway_config() {
  local operation="$1" workspace="${2:-}" value="${3:-}" config payload
  # Refresh before saving so a pause or unrelated assignment is not overwritten.
  config=$(api_get "tracking") || return 1
  if [[ $(jq -r '.mode' <<< "$config") != sway ]]; then
    show_error "Tracking mode changed. Reopen timetrack to use the current mode."
    return 1
  fi
  case "$operation" in
    resume)
      payload=$(jq -c '{mode, enabled: true, assignments}' <<< "$config")
      ;;
    assign)
      payload=$(jq -c --arg workspace "$workspace" --argjson pid "$value" '
        {mode, enabled, assignments: ([.assignments[] | select(.workspace != $workspace)]
          + [{workspace: $workspace, project_id: $pid}])}' <<< "$config")
      ;;
    unassign)
      payload=$(jq -c --arg workspace "$workspace" --arg name "$value" '
        {mode, enabled, assignments: [.assignments[] |
          select(.workspace != $workspace and .workspace != $name)]}' <<< "$config")
      ;;
  esac
  api_post "tracking" "$payload" >/dev/null
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
tracking_json=$(api_get "tracking")
mode=$(jq -r '.mode' <<< "$tracking_json")
if [[ "$mode" != global && "$mode" != sway ]]; then
  show_error "Unknown tracking mode; no tracking changes made."
  exit 1
fi

if [[ "$mode" == sway ]]; then
  focused_workspace=$(jq -r '.focused_workspace // empty' <<< "$tracking_json")
  workspace_key=$(jq -r 'if .focused_workspace_number >= 0 then
    .focused_workspace_number | tostring else .focused_workspace // empty end' <<< "$tracking_json")
  # Direct project invocations in Sway mode must assign, never manually start.
  if [[ -n "$CURRENT" ]]; then
    INTENT=assign
  fi
  if [[ "$INTENT" == assign && -z "$WORKSPACE" ]]; then
    WORKSPACE="$workspace_key"
  fi
  if [[ "$INTENT" == assign && -z "$WORKSPACE" ]]; then
    show_error "No focused Sway workspace. Check the connection in Dashboard settings."
    exit 1
  fi
elif [[ "$INTENT" == assign ]]; then
  show_error "Tracking mode changed to global. Reopen timetrack to start manually."
  exit 1
fi

active_id=$(echo "$status_json" | jq -r '.session.project_id // empty')
active_path=$(echo "$status_json" | jq -r '.path // empty')

# ── if tracking is active, just stop ────────────────────────────────────────────

if [[ "$mode" == global && -n "$active_id" && -z "$CURRENT" ]]; then
  if note=$(printf '\n' | rofi -dmenu -m primary -p "Note for ${active_path:-session} (optional):" -theme timetrack); then
    api_post "track/stop" "$(jq -nc --arg note "$note" '{note: $note}')" >/dev/null
  fi
  exit 0
fi

projects_json=$(api_get "projects")

# ── resolve current context ─────────────────────────────────────────────────────

if [[ -z "$CURRENT" ]]; then
  current_name="Track"
  current_id=""
  children_json=$(echo "$projects_json" | jq -c '. // []')
  parent_id=""
else
  current_node=$(find_node "$projects_json" "$CURRENT")
  if [[ -z "$current_node" || "$current_node" == "null" ]]; then
    exec "$SCRIPT" "" "$INTENT" "$WORKSPACE"
  fi
  current_name=$(echo "$current_node" | jq -r '.name')
  current_id="$CURRENT"
  children_json=$(echo "$current_node" | jq -c '.children // []')
  parent_id=$(echo "$current_node" | jq -r '.parent_id // empty')
fi

# ── build menu ──────────────────────────────────────────────────────────────────

items=()
actions=()

if [[ "$mode" == sway && "$INTENT" != assign ]]; then
  if [[ $(jq -r '.enabled' <<< "$tracking_json") == true ]]; then
    items+=("Pause all workspace tracking")
    actions+=("pause")
  else
    items+=("Resume workspace tracking")
    actions+=("resume")
  fi
  items+=("Assign current workspace (${focused_workspace:-unknown})"
    "Unassign current workspace (${focused_workspace:-unknown})"
    "Open Dashboard / tracking settings")
  actions+=("assign" "unassign" "settings")
else
  # 1. Track or assign at this level (only inside a project)
  if [[ -n "$current_id" ]]; then
    total=$(echo "$current_node" | jq -r '.total_seconds // 0')
    if [[ "$INTENT" == assign ]]; then
      items+=("* Assign to ${current_name} ($(fmt_dur "$total"))")
    else
      items+=("* ${current_name} ($(fmt_dur "$total"))")
    fi
    actions+=("track:$current_id")
  fi

  # 2. Children always drill down, even if leaf
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
  elif [[ "$INTENT" == assign ]]; then
    items+=("Back to Sway controls")
    actions+=("controls")
  fi
fi

# ── show rofi ───────────────────────────────────────────────────────────────────

prompt="▶ ${current_name}"
if [[ -n "$current_id" ]]; then
  bc=$(build_breadcrumb "$projects_json" "$current_id")
  prompt="▶ ${bc}"
fi
if [[ "$INTENT" == assign ]]; then
  prompt="Assign workspace ${WORKSPACE}: ${current_name}"
  if [[ -n "$current_id" ]]; then
    prompt="Assign workspace ${WORKSPACE}: ${bc}"
  fi
elif [[ "$mode" == sway ]]; then
  prompt="Sway (${focused_workspace:-disconnected})"
fi

if ! selection=$(printf '%s\n' "${items[@]}" | rofi -dmenu -m primary -p "$prompt" -i -selected-row 0 -theme timetrack); then
  exit 0
fi

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
  pause)
    api_post "track/stop" '{}' >/dev/null
    ;;

  resume)
    save_sway_config resume
    ;;

  assign|unassign)
    if [[ -z "$workspace_key" || $(jq -r '.connected' <<< "$tracking_json") != true ]]; then
      show_error "No focused Sway workspace. Check the connection in Dashboard settings."
      exit 1
    fi
    if [[ "$action" == assign ]]; then
      exec "$SCRIPT" "" assign "$workspace_key"
    fi
    # Remove the exact-name fallback as well as the higher-priority numeric key.
    save_sway_config unassign "$workspace_key" "$focused_workspace"
    ;;

  settings)
    if ! xdg-open "${API%/api}/"; then
      show_error "Could not open the browser. Open ${API%/api}/ for Dashboard settings."
      exit 1
    fi
    ;;

  controls)
    exec "$SCRIPT"
    ;;

  track:*)
    pid="${action#track:}"
    if [[ "$mode" == sway ]]; then
      save_sway_config assign "$WORKSPACE" "$pid"
    else
      api_post "track/start" "$(jq -nc --argjson pid "$pid" '{project_id: $pid}')" >/dev/null
    fi
    ;;

  drill:*)
    pid="${action#drill:}"
    exec "$SCRIPT" "$pid" "$INTENT" "$WORKSPACE"
    ;;

  new:root)
    if ! name=$(printf '\n' | rofi -dmenu -m primary -p "New project name:" -theme timetrack); then
      exit 0
    fi
    if [[ -n "$name" ]]; then
      api_post "projects" "$(jq -nc --arg name "$name" '{name: $name, parent_id: null}')" >/dev/null
      exec "$SCRIPT" "" "$INTENT" "$WORKSPACE"
    fi
    ;;

  new:*)
    pid="${action#new:}"
    if ! name=$(printf '\n' | rofi -dmenu -m primary -p "New child name:" -theme timetrack); then
      exit 0
    fi
    if [[ -n "$name" ]]; then
      api_post "projects" "$(jq -nc --arg name "$name" --argjson pid "$pid" '{name: $name, parent_id: $pid}')" >/dev/null
      exec "$SCRIPT" "$current_id" "$INTENT" "$WORKSPACE"
    fi
    ;;

  back:root)
    exec "$SCRIPT" "" "$INTENT" "$WORKSPACE"
    ;;

  back:*)
    parent="${action#back:}"
    exec "$SCRIPT" "$parent" "$INTENT" "$WORKSPACE"
    ;;
esac
