#!/usr/bin/env bash
set -euo pipefail

# PokePilot currently supports this exact Pokemon Red revision. Keep this in
# sync with red/sym.ROMSHA1; callers may override it when adding a new profile.
expected_sha1="${POKEPILOT_ROM_SHA1:-ea9bcae617fdf159b045185467ae58b2e4a48b9a}"
target="${1:-${POKEMON_RED_ROM:-$HOME/.config/pokepilot/pokemon_red.gb}}"
url="${POKEPILOT_ROM_URL:-}"

sha1_file() {
  if command -v sha1sum >/dev/null 2>&1; then
    sha1sum "$1" | awk '{print tolower($1)}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 1 "$1" | awk '{print tolower($1)}'
  else
    echo "ensure-rom: need sha1sum or shasum" >&2
    return 2
  fi
}

verify_rom() {
  local path="$1"
  [[ -f "$path" ]] || return 1

  local got
  got="$(sha1_file "$path")"
  if [[ "$got" == "$expected_sha1" ]]; then
    return 0
  fi

  echo "ensure-rom: SHA-1 mismatch for $path" >&2
  echo "ensure-rom: expected $expected_sha1" >&2
  echo "ensure-rom: got      $got" >&2
  return 1
}

if verify_rom "$target"; then
  echo "ensure-rom: using verified ROM at $target" >&2
  printf '%s\n' "$target"
  exit 0
fi

if [[ -z "$url" ]]; then
  echo "ensure-rom: no verified ROM at $target" >&2
  echo "ensure-rom: set POKEPILOT_ROM_URL to fetch the supported revision" >&2
  exit 1
fi

mkdir -p "$(dirname "$target")"
tmp="${target}.download.$$"
trap 'rm -f "$tmp"' EXIT

echo "ensure-rom: fetching ROM into private runner storage" >&2
# Do not use -v/-x here: POKEPILOT_ROM_URL may contain a private signed URL.
curl --fail --location --silent --show-error \
  --retry 3 --retry-all-errors --connect-timeout 15 --max-time 180 \
  --output "$tmp" "$url"

if ! verify_rom "$tmp"; then
  echo "ensure-rom: refusing unverified download" >&2
  exit 1
fi

chmod 0600 "$tmp"
mv -f "$tmp" "$target"
trap - EXIT

echo "ensure-rom: installed verified ROM at $target" >&2
printf '%s\n' "$target"
