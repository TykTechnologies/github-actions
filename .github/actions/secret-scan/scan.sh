#!/usr/bin/env bash
# Scans a build folder and/or a container image for secrets. Never prints
# secret values. Fails closed: a scanner that errors or reads nothing fails
# the scan.
#
#   Build folder: gitleaks (including inside archives) plus a check for
#                 credentials in copied .git/config files, which gitleaks skips.
#   Image:        trufflehog on every layer, the config and the history of
#                 every platform in a multi-platform image; then gitleaks on
#                 the archives in each platform's final filesystem, which
#                 trufflehog does not unpack.
#
# Environment:
#   SCAN_PATH        build folder to scan (optional)
#   SCAN_IMAGE       image to scan: registry ref, docker://<local>, or file://<tar> (optional)
#   SCAN_ARCHIVES    "true" exports the image and scans its archives; needs docker (default true)
#   GITLEAKS_CONFIG  .gitleaks.toml for gitleaks (optional)
#   ALLOWLIST        JSON allowlist for trufflehog findings (optional)
#   ENFORCE          "true" fails on findings; "false" reports only (default true)
#   OUT_DIR          where reports are written (default ./secret-scan-results).
#                    trufflehog's raw output there contains secret values: never upload it.
#
# Exit codes: 0 clean (or findings with ENFORCE=false), 1 findings, 2 scanner error or bad input.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SCAN_PATH=${SCAN_PATH:-}
SCAN_IMAGE=${SCAN_IMAGE:-}
SCAN_ARCHIVES=${SCAN_ARCHIVES:-true}
GITLEAKS_CONFIG=${GITLEAKS_CONFIG:-}
ALLOWLIST=${ALLOWLIST:-}
ENFORCE=${ENFORCE:-true}
OUT_DIR=${OUT_DIR:-secret-scan-results}

errors=0
error() { echo "::error title=secret-scan::$*" >&2; errors=$((errors + 1)); }

if [ -z "$SCAN_PATH" ] && [ -z "$SCAN_IMAGE" ]; then
  echo "::error title=secret-scan::set at least one of path or image" >&2
  exit 2
fi
for flag in "ENFORCE=$ENFORCE" "SCAN_ARCHIVES=$SCAN_ARCHIVES"; do
  case "${flag#*=}" in true|false) ;; *) echo "::error title=secret-scan::${flag%%=*} must be true or false" >&2; exit 2 ;; esac
done
for value in "$SCAN_PATH" "$SCAN_IMAGE" "${SCAN_IMAGE#docker://}" "${SCAN_IMAGE#file://}"; do
  case "$value" in
    -*) echo "::error title=secret-scan::path and image must not start with '-'" >&2; exit 2 ;;
  esac
done
if [ -n "$GITLEAKS_CONFIG" ] && [ ! -f "$GITLEAKS_CONFIG" ]; then
  echo "::error title=secret-scan::gitleaks config not found: $GITLEAKS_CONFIG" >&2
  exit 2
fi
if [ -n "$ALLOWLIST" ] && [ ! -f "$ALLOWLIST" ]; then
  echo "::error title=secret-scan::allowlist not found: $ALLOWLIST" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
OUT_DIR=$(cd "$OUT_DIR" && pwd)
FINDINGS="$OUT_DIR/findings.jsonl"
: > "$FINDINGS"
scanned=()

# Runs gitleaks on a directory and appends findings. $1 dir, $2 report name,
# $3 tool label, $4 path prefix to strip from reported files, $5 replacement,
# $6 config file (optional).
run_gitleaks() {
  local dir=$1 name=$2 tool=$3 strip=$4 prefix=$5 config=${6:-} rc=0
  local args=(dir "$dir" --no-banner --redact --max-archive-depth 3 --exit-code 3
    --report-format json --report-path "$OUT_DIR/$name.json")
  if [ -n "$config" ]; then
    args+=(--config "$config")
  fi
  gitleaks "${args[@]}" > "$OUT_DIR/$name.log" 2>&1 || rc=$?
  case "$rc" in
    0|3)
      jq -c --arg tool "$tool" --arg strip "$strip" --arg prefix "$prefix" '.[] | {
        tool: $tool, rule: .RuleID, platform: "", layer: "",
        file: ((if $strip != "" and (.File | startswith($strip)) then $prefix + (.File | ltrimstr($strip)) else .File end)
               | gsub("\\.secret-scan\\.(tar\\.gz|tar\\.bz2|tar\\.xz|zip)"; ""))
      }' "$OUT_DIR/$name.json" >> "$FINDINGS"
      ;;
    *)
      error "gitleaks failed with exit code $rc"
      sed -E 's/^/  gitleaks: /' "$OUT_DIR/$name.log" >&2
      ;;
  esac
}

scan_folder() {
  if [ ! -d "$SCAN_PATH" ]; then
    error "build folder not found: $SCAN_PATH"
    return
  fi
  run_gitleaks "$SCAN_PATH" gitleaks-folder gitleaks "" "" "$GITLEAKS_CONFIG"

  # gitleaks skips .git folders, but a copied .git/config can carry the
  # checkout token (http extraheader) or credentials in a remote URL.
  while IFS= read -r -d '' cfg; do
    if grep -qiE 'extraheader[[:space:]]*=.*authorization' "$cfg" ||
       grep -qE '^[[:space:]]*url[[:space:]]*=[[:space:]]*[a-z+]+://[^/@[:space:]]+:[^/@[:space:]]+@' "$cfg"; then
      jq -nc --arg f "$cfg" '{tool: "secret-scan", rule: "git-credentials", file: $f, platform: "", layer: ""}' >> "$FINDINGS"
    fi
  done < <(find "$SCAN_PATH" -type f -path '*/.git/config' -print0)
  scanned+=("build folder")
}

# Prints "<platform>|<ref>" for each image to scan. A registry reference to a
# multi-platform index expands to one digest reference per platform.
image_targets() {
  case "$SCAN_IMAGE" in
    docker://*|file://*) printf '|%s\n' "$SCAN_IMAGE"; return ;;
  esac
  local raw
  if ! raw=$(docker buildx imagetools inspect --raw "$SCAN_IMAGE" 2> "$OUT_DIR/inspect.log"); then
    error "cannot read image manifest for $SCAN_IMAGE: $(head -3 "$OUT_DIR/inspect.log" | tr '\n' ' ')"
    return 1
  fi
  if jq -e '.manifests' > /dev/null <<< "$raw"; then
    local repo=${SCAN_IMAGE%@*}
    case "${repo##*/}" in *:*) repo=${repo%:*} ;; esac
    jq -r --arg repo "$repo" '.manifests[]
      | select(.platform.os != "unknown")
      | "\(.platform.os)/\(.platform.architecture)\(if .platform.variant then "/" + .platform.variant else "" end)|\($repo)@\(.digest)"' <<< "$raw"
  else
    printf '|%s\n' "$SCAN_IMAGE"
  fi
}

# trufflehog on all layers, config and history of one image.
scan_layers() {
  local platform=$1 ref=$2 n=$3 rc=0
  trufflehog docker --image "$ref" --no-verification --json --no-update \
    > "$OUT_DIR/trufflehog-$n.json" 2> "$OUT_DIR/trufflehog-$n.log" || rc=$?
  if [ "$rc" -ne 0 ]; then
    error "trufflehog failed with exit code $rc on $ref"
    return
  fi

  # trufflehog exits 0 even when it cannot read the image, so check its log.
  local log_errors chunks
  log_errors=$(jq -Rr 'fromjson? | select(.level == "error") | "\(.msg): \(.error // "")"' "$OUT_DIR/trufflehog-$n.log")
  if [ -n "$log_errors" ]; then
    error "trufflehog could not scan $ref: $(head -3 <<< "$log_errors" | tr '\n' ' ')"
    return
  fi
  chunks=$(jq -Rr 'fromjson? | select(.msg == "finished scanning") | .chunks // 0' "$OUT_DIR/trufflehog-$n.log" | tail -1)
  if [ -z "$chunks" ] || [ "$chunks" -eq 0 ]; then
    error "trufflehog scanned nothing in $ref"
    return
  fi

  jq -c --slurpfile allow "$OUT_DIR/allowlist.json" --arg platform "$platform" '
    . as $f
    | ($f.SourceMetadata.Data.Docker.file // "") as $file
    | select(all($allow[0][]; . as $a | ($f.DetectorName != $a.detector) or ($file | test($a.path) | not)))
    | {tool: "trufflehog", rule: .DetectorName, file: $file, platform: $platform,
       layer: (.SourceMetadata.Data.Docker.layer // "")}
  ' "$OUT_DIR/trufflehog-$n.json" >> "$FINDINGS"
}

# gitleaks on the archives in one image's final filesystem.
scan_archives() {
  local platform=$1 ref=$2 n=$3 img cid
  case "$ref" in
    docker://*) img=${ref#docker://} ;;
    file://*)
      if ! img=$(docker load -i "${ref#file://}" 2> "$OUT_DIR/load-$n.log" | sed -nE 's/^Loaded image( ID)?: //p' | tail -1) || [ -z "$img" ]; then
        error "cannot load $ref for the archive scan"
        return
      fi ;;
    *)
      if ! docker pull -q "$ref" > /dev/null 2> "$OUT_DIR/pull-$n.log"; then
        error "cannot pull $ref for the archive scan: $(head -3 "$OUT_DIR/pull-$n.log" | tr '\n' ' ')"
        return
      fi
      img=$ref ;;
  esac

  # The command is never run; it only lets images without CMD be created.
  if ! cid=$(docker create "$img" secret-scan-placeholder 2> "$OUT_DIR/create-$n.log"); then
    error "cannot create a container from $img for the archive scan"
    return
  fi
  local export="$OUT_DIR/fs-$n.tar" archives="$OUT_DIR/archives-$n"
  mkdir -p "$archives"
  if ! docker export -o "$export" "$cid"; then
    docker rm "$cid" > /dev/null
    error "cannot export the filesystem of $img"
    return
  fi
  docker rm "$cid" > /dev/null

  # Copy out only the regular archive files: never links, devices, absolute
  # names or ".." paths, and never the rest of the image.
  if ! python3 -I "$HERE/extract-archives.py" "$export" "$archives" > "$OUT_DIR/archive-count-$n.txt" 2> "$OUT_DIR/extract-$n.log"; then
    error "cannot extract the archives from $img: $(head -3 "$OUT_DIR/extract-$n.log" | tr '\n' ' ')"
    rm -f "$export"
    return
  fi
  rm -f "$export"

  # gitleaks picks the archive format from the file extension and skips short
  # or zip-based ones, so rename those to an extension it recognises.
  (cd "$archives" && find . -type f -print0) |
    while IFS= read -r -d '' f; do
      local staged=""
      case "$(tr '[:upper:]' '[:lower:]' <<< "$f")" in
        *.tgz) staged="$f.secret-scan.tar.gz" ;;
        *.tbz|*.tbz2) staged="$f.secret-scan.tar.bz2" ;;
        *.txz) staged="$f.secret-scan.tar.xz" ;;
        *.jar|*.war|*.ear|*.whl|*.egg) staged="$f.secret-scan.zip" ;;
      esac
      if [ -n "$staged" ]; then
        mv "$archives/$f" "$archives/$staged"
      fi
    done

  if [ -n "$(find "$archives" -type f -print -quit)" ]; then
    run_gitleaks "$archives" "gitleaks-archives-$n" gitleaks-archive "$archives/" "/" "${GITLEAKS_CONFIG:-$HERE/gitleaks-archives.toml}"
    if [ -n "$platform" ]; then
      local tmp="$FINDINGS.tmp"
      jq -c --arg p "$platform" 'if .tool == "gitleaks-archive" and .platform == "" then .platform = $p else . end' "$FINDINGS" > "$tmp"
      mv "$tmp" "$FINDINGS"
    fi
  fi
  rm -rf "$archives"
}

scan_image() {
  local allow=("$HERE/default-allowlist.json")
  [ -n "$ALLOWLIST" ] && allow+=("$ALLOWLIST")
  jq -s 'add' "${allow[@]}" > "$OUT_DIR/allowlist.json"

  # image_targets runs in a subshell, so count its error here.
  local targets
  if ! targets=$(image_targets); then
    errors=$((errors + 1))
    return
  fi
  if [ -z "$targets" ]; then
    error "no platform images found in $SCAN_IMAGE"
    return
  fi

  local n=0 platform ref
  while IFS='|' read -r -u 3 platform ref; do
    n=$((n + 1))
    scan_layers "$platform" "$ref" "$n"
    [ "$SCAN_ARCHIVES" = "true" ] && scan_archives "$platform" "$ref" "$n"
    scanned+=("image${platform:+ $platform}")
  done 3<<< "$targets"
}

[ -n "$SCAN_PATH" ] && scan_folder
[ -n "$SCAN_IMAGE" ] && scan_image

count=$(wc -l < "$FINDINGS" | tr -d ' ')

while IFS= read -r f; do
  jq -r '"::error title=secret-scan (\(.tool) \(.rule))::\(.file)\(if .platform != "" then " [\(.platform)]" else "" end)\(if .layer != "" then " in layer \(.layer)" else "" end)"' <<< "$f"
done < "$FINDINGS"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "## Secret scan"
    echo
    [ -n "$SCAN_PATH" ] && echo "- Build folder: \`$SCAN_PATH\`"
    [ -n "$SCAN_IMAGE" ] && echo "- Image: \`$SCAN_IMAGE\`"
    if [ "${#scanned[@]}" -gt 0 ]; then
      joined=$(printf '%s, ' "${scanned[@]}")
      echo "- Scanned: ${joined%, }"
    fi
    echo
    if [ "$errors" -gt 0 ]; then
      echo ":x: The scan itself failed ($errors error(s)); see the job log."
    elif [ "$count" -eq 0 ]; then
      echo ":white_check_mark: No secrets found."
    else
      echo "| Tool | Rule | File | Platform | Layer |"
      echo "|------|------|------|----------|-------|"
      jq -r '"| \(.tool) | \(.rule) | `\(.file)` | \(.platform) | \(.layer[0:19]) |"' "$FINDINGS"
      echo
      echo "Rotate any real credential first, then remove it from the build. False positives go in the allowlist, reviewed by AppSec."
    fi
  } >> "$GITHUB_STEP_SUMMARY"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "findings=$count" >> "$GITHUB_OUTPUT"
fi

echo "secret-scan: $count finding(s), $errors scanner error(s); scanned: ${scanned[*]:-nothing}"
[ "$errors" -gt 0 ] && exit 2
[ "$count" -gt 0 ] && [ "$ENFORCE" = "true" ] && exit 1
exit 0
