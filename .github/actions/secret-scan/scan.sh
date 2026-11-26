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
#                 trufflehog does not unpack; then, for registry images, the
#                 provenance attestation, where BuildKit's mode=max records
#                 every build arg value.
#
# Environment:
#   SCAN_PATH        build folder to scan (optional)
#   SCAN_IMAGE       image to scan: registry ref, docker://<local>, or file://<tar> (optional)
#   SCAN_ARCHIVES    "true" exports the image and scans its archives; needs docker (default true)
#   SCAN_ATTESTATIONS "true" scans the provenance attestation of registry images (default true)
#   GITLEAKS_CONFIG  .gitleaks.toml for gitleaks (optional)
#   ALLOWLIST        JSON allowlist for trufflehog findings (optional)
#   MAX_ARCHIVE_MB   largest archive the archive scan copies out; a larger one fails the scan (default 1024)
#   ENFORCE          "true" fails on findings; "false" reports only (default true)
#   REPORT_DETAILS   "false" keeps finding details (files, rules) and scanner logs
#                    out of the job log and summary, for public repositories; the
#                    findings stay in $OUT_DIR/report/findings.jsonl (default true)
#   OUT_DIR          where reports are written (default ./secret-scan-results).
#                    trufflehog's raw output there contains secret values: never
#                    upload it. Only $OUT_DIR/report is safe to keep.
#
# Exit codes: 0 clean (or findings with ENFORCE=false), 1 findings, 2 scanner error or bad input.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SCAN_PATH=${SCAN_PATH:-}
SCAN_IMAGE=${SCAN_IMAGE:-}
SCAN_ARCHIVES=${SCAN_ARCHIVES:-true}
SCAN_ATTESTATIONS=${SCAN_ATTESTATIONS:-true}
GITLEAKS_CONFIG=${GITLEAKS_CONFIG:-}
ALLOWLIST=${ALLOWLIST:-}
ENFORCE=${ENFORCE:-true}
REPORT_DETAILS=${REPORT_DETAILS:-true}
OUT_DIR=${OUT_DIR:-secret-scan-results}
MAX_ARCHIVE_MB=${MAX_ARCHIVE_MB:-1024}

# Escapes text for a workflow command, so a file name or message from an
# image cannot end the annotation and start a command of its own.
escape_cmd() {
  local s=$*
  s=${s//'%'/%25}; s=${s//$'\r'/%0D}; s=${s//$'\n'/%0A}
  printf '%s' "$s"
}
errors=0
# Scanner logs can name files inside the scanned image, so they follow
# REPORT_DETAILS. Reads the log on stdin and prints its first lines.
log_excerpt() {
  if [ "$REPORT_DETAILS" = "true" ]; then
    head -3 | tr '\n' ' '
  else
    cat > /dev/null
    printf '(details withheld)'
  fi
}
error() { echo "::error title=secret-scan::$(escape_cmd "$*")" >&2; errors=$((errors + 1)); }

if [ -z "$SCAN_PATH" ] && [ -z "$SCAN_IMAGE" ]; then
  echo "::error title=secret-scan::set at least one of path or image" >&2
  exit 2
fi
for flag in "ENFORCE=$ENFORCE" "SCAN_ARCHIVES=$SCAN_ARCHIVES" "SCAN_ATTESTATIONS=$SCAN_ATTESTATIONS" "REPORT_DETAILS=$REPORT_DETAILS"; do
  case "${flag#*=}" in true|false) ;; *) echo "::error title=secret-scan::${flag%%=*} must be true or false" >&2; exit 2 ;; esac
done
for value in "$SCAN_PATH" "$SCAN_IMAGE" "${SCAN_IMAGE#docker://}" "${SCAN_IMAGE#file://}"; do
  case "$value" in
    -*) echo "::error title=secret-scan::path and image must not start with '-'" >&2; exit 2 ;;
  esac
done
case "$MAX_ARCHIVE_MB" in
  ''|*[!0-9]*) echo "::error title=secret-scan::MAX_ARCHIVE_MB must be a whole number" >&2; exit 2 ;;
esac
if [ -n "$GITLEAKS_CONFIG" ] && [ ! -f "$GITLEAKS_CONFIG" ]; then
  echo "::error title=secret-scan::gitleaks config not found: $GITLEAKS_CONFIG" >&2
  exit 2
fi
if [ -n "$ALLOWLIST" ] && [ ! -f "$ALLOWLIST" ]; then
  echo "::error title=secret-scan::allowlist not found: $ALLOWLIST" >&2
  exit 2
fi

if [ -n "$ALLOWLIST" ] && ! jq -e 'type == "array" and all(.[];
      type == "object" and (.detector | type) == "string" and (.path | type) == "string"
      and ((.path as $re | "" | test($re)) or true))' "$ALLOWLIST" > /dev/null 2>&1; then
  echo "::error title=secret-scan::allowlist must be a JSON list of {detector, path, reason} with path a valid regex: $ALLOWLIST" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
OUT_DIR=$(cd "$OUT_DIR" && pwd)
mkdir -p "$OUT_DIR/report"
FINDINGS="$OUT_DIR/report/findings.jsonl"
: > "$FINDINGS"
NO_IGNORE="$OUT_DIR/no-gitleaksignore"
mkdir -p "$NO_IGNORE"
scanned=()

# Runs gitleaks on a directory and appends findings. $1 dir, $2 report name,
# $3 tool label, $4 path prefix to strip from reported files, $5 replacement,
# $6 config file, $7 platform (optional).
#
# Only the action's inputs decide what is reported: the config is always
# passed (a .gitleaks.toml in the scanned folder would otherwise replace the
# rules), gitleaks:allow comments are ignored, and .gitleaksignore is read
# from an empty folder instead of the working directory.
run_gitleaks() {
  local dir=$1 name=$2 tool=$3 strip=$4 prefix=$5 config=$6 platform=${7:-} rc=0
  local args=(dir "$dir" --no-banner --redact --max-archive-depth 3 --exit-code 3
    --config "$config" --ignore-gitleaks-allow --gitleaks-ignore-path "$NO_IGNORE"
    --report-format json --report-path "$OUT_DIR/$name.json")
  gitleaks "${args[@]}" > "$OUT_DIR/$name.log" 2>&1 || rc=$?
  case "$rc" in
    0|3)
      jq -c --arg tool "$tool" --arg strip "$strip" --arg prefix "$prefix" --arg platform "$platform" '.[] | {
        tool: $tool, rule: .RuleID, platform: $platform, layer: "",
        file: ((if $strip != "" and (.File | startswith($strip)) then $prefix + (.File | ltrimstr($strip)) else .File end)
               | gsub("\\.secret-scan\\.(tar\\.gz|tar\\.bz2|tar\\.xz|zip)"; ""))
      }' "$OUT_DIR/$name.json" >> "$FINDINGS"
      ;;
    *)
      error "gitleaks failed with exit code $rc"
      if [ "$REPORT_DETAILS" = "true" ]; then
        sed -E 's/^/  gitleaks: /' "$OUT_DIR/$name.log" >&2
      fi
      ;;
  esac
}

scan_folder() {
  if [ ! -d "$SCAN_PATH" ]; then
    error "build folder not found: $SCAN_PATH"
    return
  fi
  run_gitleaks "$SCAN_PATH" gitleaks-folder gitleaks "" "" "${GITLEAKS_CONFIG:-$HERE/gitleaks-default.toml}"

  # gitleaks always applies a .gitleaksignore at the root of the scanned
  # folder and there is no option to stop it, so report the file itself:
  # suppressions belong in the reviewed gitleaks-config input.
  if [ -f "$SCAN_PATH/.gitleaksignore" ]; then
    jq -nc --arg f "$SCAN_PATH/.gitleaksignore" '{tool: "secret-scan", rule: "unreviewed-gitleaksignore", file: $f, platform: "", layer: ""}' >> "$FINDINGS"
  fi

  # gitleaks skips .git folders, but a copied .git/config can carry the
  # checkout token (http extraheader) or credentials in a remote URL.
  while IFS= read -r -d '' cfg; do
    if grep -qiE 'extraheader[[:space:]]*=.*authorization' "$cfg" ||
       grep -qE '^[[:space:]]*url[[:space:]]*=[[:space:]]*[a-z+]+://[^/@[:space:]]+:[^/@[:space:]]+@' "$cfg"; then
      jq -nc --arg f "$cfg" '{tool: "secret-scan", rule: "git-credentials", file: $f, platform: "", layer: ""}' >> "$FINDINGS"
    fi
  done < <(find "$SCAN_PATH" -type f -path '*/.git/config' -print0 2> /dev/null)
  scanned+=("build folder")
}

# Pins a registry image to the digest of its index or manifest, once, so the
# layer, archive and provenance scans all read the same image even if the tag
# moves. Sets IMAGE_REPO and IMAGE_PINNED (repo@digest).
IMAGE_REPO=""
IMAGE_PINNED=""
resolve_image() {
  IMAGE_REPO=${SCAN_IMAGE%@*}
  case "${IMAGE_REPO##*/}" in *:*) IMAGE_REPO=${IMAGE_REPO%:*} ;; esac
  local digest
  if ! digest=$(docker buildx imagetools inspect --format '{{json .Manifest}}' "$SCAN_IMAGE" 2> "$OUT_DIR/inspect.log" | jq -r '.digest // empty') ||
     [ -z "$digest" ]; then
    error "cannot read image manifest for $SCAN_IMAGE: $(log_excerpt < "$OUT_DIR/inspect.log")"
    return 1
  fi
  IMAGE_PINNED="$IMAGE_REPO@$digest"
}

# Prints "<platform>|<ref>" for each image to scan: an index expands to one
# digest per platform, and a single manifest is the pinned reference itself.
image_targets() {
  case "$SCAN_IMAGE" in
    docker://*|file://*) printf '|%s\n' "$SCAN_IMAGE"; return ;;
  esac
  local raw
  if ! raw=$(docker buildx imagetools inspect --raw "$IMAGE_PINNED" 2> "$OUT_DIR/inspect.log"); then
    error "cannot read image manifest for $IMAGE_PINNED: $(log_excerpt < "$OUT_DIR/inspect.log")"
    return 1
  fi
  if jq -e '.manifests' > /dev/null <<< "$raw"; then
    jq -r --arg repo "$IMAGE_REPO" '.manifests[]
      | select(.platform.os != "unknown")
      | "\(.platform.os)/\(.platform.architecture)\(if .platform.variant then "/" + .platform.variant else "" end)|\($repo)@\(.digest)"' <<< "$raw"
  else
    printf '|%s\n' "$IMAGE_PINNED"
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
    error "trufflehog could not scan $ref: $(log_excerpt <<< "$log_errors")"
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
  local platform=$1 ref=$2 n=$3 img cid pulled=false
  case "$ref" in
    docker://*) img=${ref#docker://} ;;
    file://*)
      if ! img=$(docker load -i "${ref#file://}" 2> "$OUT_DIR/load-$n.log" | sed -nE 's/^Loaded image( ID)?: //p' | tail -1) || [ -z "$img" ]; then
        error "cannot load $ref for the archive scan"
        return
      fi ;;
    *)
      # Pull only when the image is not already local (for example, the job
      # just built it), and remember it so only what this step pulled is removed.
      if ! docker image inspect "$ref" > /dev/null 2>&1; then
        if ! docker pull -q "$ref" > /dev/null 2> "$OUT_DIR/pull-$n.log"; then
          error "cannot pull $ref for the archive scan: $(log_excerpt < "$OUT_DIR/pull-$n.log")"
          return
        fi
        pulled=true
      fi
      img=$ref ;;
  esac

  # The command is never run; it only lets images without CMD be created.
  if ! cid=$(docker create "$img" secret-scan-placeholder 2> "$OUT_DIR/create-$n.log"); then
    error "cannot create a container from $img for the archive scan"
    return
  fi
  local archives="$OUT_DIR/archives-$n" rc=0
  mkdir -p "$archives"

  # Stream the filesystem and copy out only the regular archive files: never
  # links, devices, absolute names or ".." paths, and never the rest of the
  # image. An archive over MAX_ARCHIVE_MB fails the scan rather than being skipped.
  docker export "$cid" 2> "$OUT_DIR/export-$n.log" |
    python3 -I "$HERE/extract-archives.py" - "$archives" "$((MAX_ARCHIVE_MB * 1024 * 1024))" \
      > "$OUT_DIR/archive-count-$n.txt" 2> "$OUT_DIR/extract-$n.log" || rc=$?
  docker rm "$cid" > /dev/null
  if [ "$rc" -ne 0 ]; then
    error "cannot extract the archives from $img: $(cat "$OUT_DIR/export-$n.log" "$OUT_DIR/extract-$n.log" | log_excerpt)"
    return
  fi

  # gitleaks picks the archive format from the file extension and skips short
  # or zip-based ones, so rename those to an extension it recognises.
  # The loop runs in a subshell, so a failed rename is counted here. Its error
  # would name a file in the image, so it is not printed.
  if ! (cd "$archives" && find . -type f -print0) |
    while IFS= read -r -d '' f; do
      local staged=""
      case "$(tr '[:upper:]' '[:lower:]' <<< "$f")" in
        *.tgz) staged="$f.secret-scan.tar.gz" ;;
        *.tbz|*.tbz2) staged="$f.secret-scan.tar.bz2" ;;
        *.txz) staged="$f.secret-scan.tar.xz" ;;
        *.jar|*.war|*.ear|*.whl|*.egg) staged="$f.secret-scan.zip" ;;
      esac
      if [ -n "$staged" ]; then
        mv "$archives/$f" "$archives/$staged" 2> /dev/null || exit 1
      fi
    done; then
    error "cannot stage the archives from $img for gitleaks"
  fi

  if [ -n "$(find "$archives" -type f -print -quit)" ]; then
    run_gitleaks "$archives" "gitleaks-archives-$n" gitleaks-archive "$archives/" "/" \
      "${GITLEAKS_CONFIG:-$HERE/gitleaks-archives.toml}" "$platform"
  fi
  rm -rf "$archives"
  # Free the disk before the next platform, but never remove an image the
  # job had before this step ran.
  if [ "$pulled" = "true" ]; then
    docker rmi "$ref" > /dev/null 2>&1 || true
  fi
}

# Provenance attestations of a registry image. With provenance mode=max,
# BuildKit records every build arg value in them (and, on GitHub Actions, the
# triggering event payload), and they are published with the image, so a secret
# passed as a build arg leaks even when no layer holds it, for example one used
# only in a builder stage. Flags any non-empty build arg whose name says it is a
# secret, whatever its value (a password has no pattern gitleaks can match), and
# runs gitleaks on the rest. Findings go through the allowlist like image
# findings. Local and tarball images carry no attestations.
scan_provenance() {
  case "$SCAN_IMAGE" in docker://*|file://*) return ;; esac
  local raw="$OUT_DIR/provenance.json" by_platform="$OUT_DIR/provenance-by-platform.json"
  local dir="$OUT_DIR/provenance" found="$OUT_DIR/provenance-findings.jsonl"
  # These hold build arg values: never leave them in the results folder.
  cleanup_provenance() { rm -rf "$raw" "$by_platform" "$dir" "$found"; }

  if ! docker buildx imagetools inspect --format '{{json .Provenance}}' "$IMAGE_PINNED" > "$raw" 2> "$OUT_DIR/provenance.log" ||
     ! jq -e . "$raw" > /dev/null 2>&1; then
    error "cannot read the provenance of $IMAGE_PINNED: $(log_excerpt < "$OUT_DIR/provenance.log")"
    cleanup_provenance
    return
  fi

  # A single-platform image gives one statement, an index a map keyed by
  # platform, and an image without attestations an empty object.
  jq 'if . == null then {} elif type == "object" and has("SLSA") then {"": .} else . end' "$raw" > "$by_platform"
  if [ "$(jq 'length' "$by_platform")" -eq 0 ]; then
    echo "::warning title=secret-scan::no provenance attestation found for $(escape_cmd "$SCAN_IMAGE"); nothing to scan there" >&2
    scanned+=("provenance (none)")
    cleanup_provenance
    return
  fi

  : > "$found"
  jq -c 'to_entries[] | .key as $platform
    | [.value | .. | objects | to_entries[] | select((.key | startswith("build-arg:")) and .value != "") | .key] | unique[]
    | ltrimstr("build-arg:") as $name
    | select($name | test("(^|_)(token|secret|password|passwd|pwd|pass|key|apikey|credentials?|auth)(_|$)"; "i"))
    | select($name | test("_(id|url|uri|ttl|path|file|mode|type|name|host|port)$|(^|_)public_key($|_)"; "i") | not)
    | {tool: "provenance", rule: "secret-named-build-arg", file: ., platform: $platform, layer: ""}' \
    "$by_platform" >> "$found"

  # run_gitleaks appends to FINDINGS: point it at this scan's file so the
  # allowlist and de-duplication below apply before anything is reported.
  local n=0 platform
  mkdir -p "$dir"
  while IFS= read -r -u 3 platform; do
    n=$((n + 1))
    jq --arg p "$platform" '.[$p]' "$by_platform" > "$dir/$n.json"
    FINDINGS="$found" run_gitleaks "$dir/$n.json" "gitleaks-provenance-$n" gitleaks-provenance "$dir/$n.json" "provenance attestation" \
      "${GITLEAKS_CONFIG:-$HERE/gitleaks-default.toml}" "$platform"
  done 3< <(jq -r 'keys[]' "$by_platform")

  # A build arg is recorded several times in one statement: report it once.
  jq -sc --slurpfile allow "$OUT_DIR/allowlist.json" 'unique[]
    | . as $f | select(all($allow[0][]; . as $a | ($f.rule != $a.detector) or ($f.file | test($a.path) | not)))' \
    "$found" >> "$FINDINGS"
  cleanup_provenance
  scanned+=("provenance")
}

scan_image() {
  local allow=("$HERE/default-allowlist.json")
  [ -n "$ALLOWLIST" ] && allow+=("$ALLOWLIST")
  jq -s 'add' "${allow[@]}" > "$OUT_DIR/allowlist.json"

  case "$SCAN_IMAGE" in
    docker://*|file://*) ;;
    *) resolve_image || return 0 ;;
  esac

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
    echo "secret-scan: scanning $ref${platform:+ ($platform)}"
    scan_layers "$platform" "$ref" "$n"
    [ "$SCAN_ARCHIVES" = "true" ] && scan_archives "$platform" "$ref" "$n"
    scanned+=("image${platform:+ $platform}")
  done 3<<< "$targets"
  [ "$SCAN_ATTESTATIONS" = "true" ] && scan_provenance
  return 0
}

[ -n "$SCAN_PATH" ] && scan_folder
[ -n "$SCAN_IMAGE" ] && scan_image

count=$(wc -l < "$FINDINGS" | tr -d ' ')

if [ "$REPORT_DETAILS" = "false" ]; then
  [ "$count" -gt 0 ] && echo "::error title=secret-scan::$count finding(s); details are withheld from this log" >&2
else
  # File names come from the scanned folder or image: escape them.
  jq -r 'def cmd: gsub("%"; "%25") | gsub("\r"; "%0D") | gsub("\n"; "%0A");
    def prop: cmd | gsub(":"; "%3A") | gsub(","; "%2C");
    "::error title=\("secret-scan (\(.tool) \(.rule))" | prop)::\("\(.file)\(if .platform != "" then " [\(.platform)]" else "" end)\(if .layer != "" then " in layer \(.layer)" else "" end)" | cmd)"' "$FINDINGS"
fi

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
      if [ "$REPORT_DETAILS" = "true" ]; then
        echo ":x: The scan itself failed ($errors error(s)); see the job log."
      else
        echo ":x: The scan itself failed ($errors error(s)). Details are withheld; rerun privately with report-details true."
      fi
    elif [ "$count" -eq 0 ]; then
      echo ":white_check_mark: No secrets found."
    elif [ "$REPORT_DETAILS" = "false" ]; then
      echo ":x: $count finding(s). Details are withheld here; see the report output."
    else
      echo "| Tool | Rule | File | Platform | Layer |"
      echo "|------|------|------|----------|-------|"
      jq -r 'def cell: gsub("[\r\n]"; " ") | gsub("\\|"; "\\|") | gsub("`"; "'"'"'");
        "| \(.tool) | \(.rule) | `\(.file | cell)` | \(.platform) | \(.layer[0:19]) |"' "$FINDINGS"
      echo
      echo "Rotate any real credential first, then remove it from the build. False positives go in the allowlist, reviewed by the security team."
    fi
  } >> "$GITHUB_STEP_SUMMARY"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "findings=$count" >> "$GITHUB_OUTPUT"
  echo "report=$FINDINGS" >> "$GITHUB_OUTPUT"
fi

echo "secret-scan: $count finding(s), $errors scanner error(s); scanned: ${scanned[*]:-nothing}"
[ "$errors" -gt 0 ] && exit 2
[ "$count" -gt 0 ] && [ "$ENFORCE" = "true" ] && exit 1
exit 0
