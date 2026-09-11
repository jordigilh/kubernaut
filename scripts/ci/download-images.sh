#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  GITHUB_TOKEN=... download-images.sh [run-id] [output-dir] [--load] [--arch amd64|arm64]

  NOTE: The filename is historical; the script downloads images for the
  host architecture (amd64 or arm64), inferred via `uname -m`.

Environment:
  REPOSITORY  GitHub repository, default: jordigilh/kubernaut
  GH_TOKEN    Alternative to GITHUB_TOKEN
  ARCH / IMAGE_ARCH
              Override host architecture detection (amd64 or arm64).
              `--arch` takes precedence over these.

  On arm64, db-migrate and mock-llm have no arm64 artifacts (amd64-only
  builds) and are skipped; all other service images should be available.

Example:
  GITHUB_TOKEN=ghp_... ./scripts/ci/download-images.sh 34062457634 /root/ci-images --load
EOF
}

RUN_ID=""
OUTPUT_DIR=""
ARCH_OVERRIDE=""
LOAD_IMAGES=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --load)
      LOAD_IMAGES=true
      shift
      ;;
    --arch=*)
      ARCH_OVERRIDE="${1#--arch=}"
      shift
      ;;
    --arch)
      if [[ $# -lt 2 ]]; then
        echo "error: --arch requires a value (amd64 or arm64)" >&2
        exit 1
      fi
      ARCH_OVERRIDE="$2"
      shift 2
      ;;
    --*)
      echo "error: unknown option $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -z "$RUN_ID" ]]; then
        RUN_ID="$1"
      elif [[ -z "$OUTPUT_DIR" ]]; then
        OUTPUT_DIR="$1"
      else
        echo "error: unexpected argument $1" >&2
        usage >&2
        exit 1
      fi
      shift
      ;;
  esac
done

RUN_ID="${RUN_ID:-34062457634}"
OUTPUT_DIR="${OUTPUT_DIR:-$PWD/ci-images-${RUN_ID}}"

normalize_arch() {
  case "$1" in
    amd64|x86_64|x86-64)
      echo "amd64"
      ;;
    arm64|aarch64)
      echo "arm64"
      ;;
    *)
      echo "error: unsupported architecture '$1' (expected amd64 or arm64)" >&2
      return 1
      ;;
  esac
}

detect_host_arch() {
  local override="${ARCH_OVERRIDE:-${ARCH:-${IMAGE_ARCH:-}}}"
  if [[ -n "$override" ]]; then
    normalize_arch "$override"
    return
  fi
  normalize_arch "$(uname -m)"
}

ARCH="$(detect_host_arch)"
ARCH_UPPER="$(printf '%s' "$ARCH" | tr '[:lower:]' '[:upper:]')"

if [[ "$ARCH" == "arm64" ]]; then
  echo "Note: db-migrate and mock-llm are amd64-only; no ${ARCH} artifacts expected for them." >&2
fi

REPOSITORY="${REPOSITORY:-jordigilh/kubernaut}"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

if [[ -z "$TOKEN" ]]; then
  echo "error: set GITHUB_TOKEN or GH_TOKEN" >&2
  usage >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 is required to parse GitHub API responses" >&2
  exit 1
fi

if [[ "$LOAD_IMAGES" == true ]] && ! command -v podman >/dev/null 2>&1; then
  echo "error: podman is required when --load is used" >&2
  exit 1
fi

API_ROOT="https://api.github.com/repos/${REPOSITORY}/actions/runs/${RUN_ID}/artifacts"
DOWNLOAD_DIR="${OUTPUT_DIR}/.downloads"
mkdir -p "$DOWNLOAD_DIR"

cleanup() {
  rm -rf "$DOWNLOAD_DIR"
}
trap cleanup EXIT

artifact_count=0
page=1
while :; do
  response_file="${DOWNLOAD_DIR}/artifacts-${page}.json"
  curl --fail --silent --show-error \
    --header "Accept: application/vnd.github+json" \
    --header "Authorization: Bearer ${TOKEN}" \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    "${API_ROOT}?per_page=100&page=${page}" >"$response_file"

  mapfile -t artifacts < <(ARCH="$ARCH" python3 - "$response_file" <<'PY'
import json
import os
import re
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    payload = json.load(stream)

arch = os.environ.get("ARCH", "amd64")
pattern = r"image-.+-" + re.escape(arch)

for artifact in payload.get("artifacts", []):
    name = artifact.get("name", "")
    if re.fullmatch(pattern, name) and not artifact.get("expired", False):
        print(f"{name}\t{artifact['archive_download_url']}")
PY
  )

  if [[ "${#artifacts[@]}" -eq 0 ]]; then
    break
  fi

  for artifact in "${artifacts[@]}"; do
    IFS=$'\t' read -r name archive_url <<<"$artifact"
    archive_file="${DOWNLOAD_DIR}/${name}.zip"
    artifact_dir="${OUTPUT_DIR}/${name#image-}"
    temporary_archive="${archive_file}.tmp"
    rm -f "$archive_file" "$temporary_archive"
    rm -rf "$artifact_dir"
    mkdir -p "$artifact_dir"

    echo "Downloading ${name}"
    curl --fail --silent --show-error --location --retry 5 --retry-all-errors \
      --header "Accept: application/vnd.github+json" \
      --header "Authorization: Bearer ${TOKEN}" \
      --output "$temporary_archive" \
      "$archive_url"
    unzip -tq "$temporary_archive"
    mv "$temporary_archive" "$archive_file"

    unzip -q -o "$archive_file" -d "$artifact_dir"
    shopt -s nullglob
    image_tars=("${artifact_dir}"/*.tar)
    if [[ "${#image_tars[@]}" -ne 1 ]]; then
      echo "error: ${name} did not contain an image tar" >&2
      exit 1
    fi
    image_tar="${image_tars[0]}"

    if [[ "$LOAD_IMAGES" == true ]]; then
      echo "Loading ${image_tar}"
      podman load --input "$image_tar"
    fi

    artifact_count=$((artifact_count + 1))
  done

  page=$((page + 1))
done

if [[ "$artifact_count" -eq 0 ]]; then
  echo "error: no unexpired image-*-${ARCH} artifacts found for run ${RUN_ID}" >&2
  exit 1
fi

echo "Downloaded ${artifact_count} ${ARCH_UPPER} image artifacts to ${OUTPUT_DIR}"
if [[ "$LOAD_IMAGES" != true ]]; then
  echo "Run with --load to import the image tars into Podman."
fi
