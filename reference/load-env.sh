#!/usr/bin/env bash
# Bitwarden env helpers. Source this file in .bashrc.

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  echo "This file must be sourced: . ~/load-env.sh" >&2
  exit 1
fi

bw-unlock() {
  local raw status

  if [ -n "${BW_SESSION-}" ]; then
    echo "BW_SESSION already set for this shell."
    return 0
  fi

  if ! command -v bw >/dev/null 2>&1; then
    echo "bw not found in PATH" >&2
    return 1
  fi

  if ! bw login --check >/dev/null 2>&1; then
    echo "Not logged in. Run: bw login" >&2
    return 1
  fi

  raw="$(bw unlock --raw)"
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "bw unlock failed (exit $status)." >&2
    return "$status"
  fi

  if [ -z "$raw" ]; then
    echo "bw unlock --raw returned empty output." >&2
    return 1
  fi

  case "$raw" in
    [A-Za-z0-9+/=._-]*)
      export BW_SESSION="$raw"
      ;;
    *)
      echo "bw unlock --raw output format not recognized; refusing to use it." >&2
      return 1
      ;;
  esac

  echo "Bitwarden unlocked for this shell."
}

bw-load-env() {
  local status list_json name value var loaded missing invalid

  if ! command -v bw >/dev/null 2>&1; then
    echo "bw not found in PATH" >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    echo "jq not found in PATH" >&2
    return 1
  fi

  if [ -z "${BW_SESSION-}" ]; then
    return 0
  fi

  status="$(bw status 2>/dev/null | jq -r '.status' 2>/dev/null)"
  if [ "$status" != "unlocked" ]; then
    return 0
  fi

  list_json="$(bw list items --search "env:")"
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "bw list items failed (exit $status)." >&2
    return "$status"
  fi

  loaded=0
  missing=0
  invalid=0

  while IFS=$'\t' read -r name value; do
    [ -z "$name" ] && continue
    case "$name" in
      env:*) ;;
      *) continue ;;
    esac

    var="${name#env:}"
    if [[ ! "$var" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "Skipping invalid env var name from item: $name" >&2
      invalid=$((invalid + 1))
      continue
    fi

    if [ -z "$value" ]; then
      echo "Missing custom field 'value' for item: $name" >&2
      missing=$((missing + 1))
      continue
    fi

    export "$var=$value"
    loaded=$((loaded + 1))
  done < <(
    printf '%s' "$list_json" | jq -r '
      .[] | select(.name|startswith("env:")) |
      .name as $name |
      ( .fields // [] | map(select(.name=="value") | .value) | .[0] // "" ) as $value |
      [$name, $value] | @tsv
    '
  )

  if [ "$loaded" -eq 0 ] && [ "$missing" -eq 0 ] && [ "$invalid" -eq 0 ]; then
    echo "No env: items found in Bitwarden." >&2
    return 1
  fi

  echo "Loaded $loaded variable(s)."
  if [ "$missing" -gt 0 ]; then
    echo "Missing value field for $missing item(s)." >&2
  fi
  if [ "$invalid" -gt 0 ]; then
    echo "Invalid env var name for $invalid item(s)." >&2
  fi
}
