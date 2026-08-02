#!/usr/bin/env bash
# Validate the task graph without requiring a YAML parser dependency.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tasks_file=${1:-"$repo_root/spec/tasks.yaml"}

if [[ ! -r $tasks_file ]]; then
  printf 'error: cannot read task file: %s\n' "$tasks_file" >&2
  exit 1
fi

declare -a task_ids=()
declare -A seen_ids=() statuses=() dependencies=() saw_status=() saw_dependencies=() visit_state=()
errors=0
current_id=

error() { printf 'error: %s\n' "$*" >&2; errors=$((errors + 1)); }
trim() {
  local value=$1
  value=${value#"${value%%[![:space:]]*}"}
  value=${value%"${value##*[![:space:]]}"}
  printf '%s' "$value"
}

while IFS= read -r line || [[ -n $line ]]; do
  if [[ $line =~ ^[[:space:]]{2}-[[:space:]]id:[[:space:]]*([^[:space:]#]+)[[:space:]]*$ ]]; then
    current_id=${BASH_REMATCH[1]}
    [[ $current_id =~ ^[A-Z][A-Z0-9-]*$ ]] || error "invalid task ID: $current_id"
    if [[ -v "seen_ids[$current_id]" ]]; then
      error "duplicate task ID: $current_id"; current_id=; continue
    fi
    seen_ids[$current_id]=1; task_ids+=("$current_id"); dependencies[$current_id]=
    continue
  fi
  [[ -n $current_id ]] || continue
  if [[ $line =~ ^[[:space:]]{4}status:[[:space:]]*([^[:space:]#]+)[[:space:]]*$ ]]; then
    statuses[$current_id]=${BASH_REMATCH[1]}; saw_status[$current_id]=1; continue
  fi
  if [[ $line =~ ^[[:space:]]{4}depends_on:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    dependency_list=${BASH_REMATCH[1]}; dependencies[$current_id]=; saw_dependencies[$current_id]=1
    if [[ -n $(trim "$dependency_list") ]]; then
      IFS=',' read -r -a parsed_dependencies <<< "$dependency_list"
      for dependency in "${parsed_dependencies[@]}"; do
        dependency=$(trim "$dependency")
        [[ -n $dependency ]] || { error "empty dependency in task $current_id"; continue; }
        dependencies[$current_id]+=" $dependency"
      done
    fi
  fi
done < "$tasks_file"

for task_id in "${task_ids[@]}"; do
  if [[ ! -v "saw_status[$task_id]" ]]; then
    error "task $task_id has no status"
  elif [[ ! ${statuses[$task_id]} =~ ^(blocked|ready|in_progress|done|superseded)$ ]]; then
    error "task $task_id has invalid status: ${statuses[$task_id]}"
  fi
  [[ -v "saw_dependencies[$task_id]" ]] || error "task $task_id has no depends_on list"
  for dependency in ${dependencies[$task_id]}; do
    [[ -v "seen_ids[$dependency]" ]] || error "task $task_id depends on unknown task: $dependency"
  done
done

in_progress=0
for task_id in "${task_ids[@]}"; do
  [[ ${statuses[$task_id]:-} == in_progress ]] && in_progress=$((in_progress + 1))
  if [[ ${statuses[$task_id]:-} == ready ]]; then
    for dependency in ${dependencies[$task_id]}; do
      [[ ${statuses[$dependency]:-} == done ]] || error "ready task $task_id has incomplete dependency: $dependency"
    done
  fi
done
(( in_progress <= 1 )) || error "more than one task is in_progress ($in_progress)"

visit() {
  local task_id=$1 dependency
  case ${visit_state[$task_id]:-unvisited} in
    visiting) error "dependency cycle includes task: $task_id"; return ;;
    visited) return ;;
  esac
  visit_state[$task_id]=visiting
  for dependency in ${dependencies[$task_id]}; do
    [[ -v "seen_ids[$dependency]" ]] && visit "$dependency"
  done
  visit_state[$task_id]=visited
}
for task_id in "${task_ids[@]}"; do visit "$task_id"; done

if (( errors > 0 )); then
  printf 'task graph validation failed with %d error(s)\n' "$errors" >&2
  exit 1
fi
printf 'task graph validation passed: %d task(s)\n' "${#task_ids[@]}"
