#!/usr/bin/env bash
# Print ready task IDs compatible with the supplied environment capabilities.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if (( $# < 1 || $# > 2 )); then
  printf 'usage: %s <capability[,capability...]> [tasks-file]\n' "$0" >&2
  exit 2
fi

capabilities=$1
tasks_file=${2:-"$repo_root/spec/tasks.yaml"}
if [[ ! -r $tasks_file ]]; then
  printf 'error: cannot read task file: %s\n' "$tasks_file" >&2
  exit 1
fi
"$repo_root/scripts/spec-validate.sh" "$tasks_file" >/dev/null

trim() {
  local value=$1
  value=${value#"${value%%[![:space:]]*}"}
  value=${value%"${value##*[![:space:]]}"}
  printf '%s' "$value"
}

declare -A available=()
IFS=',' read -r -a supplied <<< "$capabilities"
for capability in "${supplied[@]}"; do
  capability=$(trim "$capability")
  if [[ -z $capability || ! $capability =~ ^[a-z][a-z0-9-]*$ ]]; then
    printf 'error: invalid capability: %s\n' "$capability" >&2
    exit 2
  fi
  available[$capability]=1
done

current_id=
current_status=
current_environment=

# Returns non-zero when the task is simply not selected, so every call site
# must tolerate that; the script's own exit status must not depend on it.
emit_current() {
  local requirement
  [[ ${current_status:-} == ready ]] || return
  for requirement in $current_environment; do
    [[ -v "available[$requirement]" ]] || return
  done
  printf '%s\n' "$current_id"
}

while IFS= read -r line || [[ -n $line ]]; do
  if [[ $line =~ ^[[:space:]]{2}-[[:space:]]id:[[:space:]]*([^[:space:]#]+)[[:space:]]*$ ]]; then
    [[ -n $current_id ]] && { emit_current || true; }
    current_id=${BASH_REMATCH[1]}
    current_status=
    current_environment=
    continue
  fi
  [[ -n $current_id ]] || continue
  if [[ $line =~ ^[[:space:]]{4}status:[[:space:]]*([^[:space:]#]+)[[:space:]]*$ ]]; then
    current_status=${BASH_REMATCH[1]}
  elif [[ $line =~ ^[[:space:]]{4}environment:[[:space:]]*\[(.*)\][[:space:]]*$ ]]; then
    environment_list=${BASH_REMATCH[1]}
    IFS=',' read -r -a requirements <<< "$environment_list"
    current_environment=
    for requirement in "${requirements[@]}"; do
      requirement=$(trim "$requirement")
      [[ -n $requirement ]] && current_environment+=" $requirement"
    done
  fi
done < "$tasks_file"
[[ -n $current_id ]] && { emit_current || true; }
exit 0
