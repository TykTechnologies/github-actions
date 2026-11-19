#!/usr/bin/env bash
# Runs the secret-scan action script against every scenario built by
# make-scenarios.sh and checks each exit code: 0 clean, 1 findings, 2 scanner error.
# Needs gitleaks, trufflehog, jq and docker on PATH.
# Usage: run.sh <workdir>
set -uo pipefail
W=${1:?workdir}
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SCAN="$HERE/../../.github/actions/secret-scan/scan.sh"
CTX="$W/ctx"
fail=0

check() {
  local name=$1 want=$2; shift 2
  local got=0
  env "$@" OUT_DIR="$W/out/$name" GITHUB_STEP_SUMMARY= GITHUB_OUTPUT= bash "$SCAN" > "$W/out/$name.log" 2>&1 || got=$?
  if [ "$got" = "$want" ]; then
    printf 'ok    %-34s exit %s\n' "$name" "$got"
  else
    printf 'FAIL  %-34s exit %s, want %s\n' "$name" "$got" "$want"
    sed 's/^/        /' "$W/out/$name.log"
    fail=1
  fi
}

mkdir -p "$W/out"

# Every leak is caught by the image scan. s09 and s15 hide the secret in a
# tar archive, which trufflehog does not unpack and only the archive scan finds;
# trufflehog does unpack the zip-based .jar in s16.
for id in s01 s02 s03 s04 s05 s06 s07 s08 s09 s10 s11 s12 s13 s14 s15 s16; do
  check "$id-image" 1 SCAN_IMAGE="docker://secret-scan-test/$id"
done
check s09-image-no-archives 0 SCAN_IMAGE="docker://secret-scan-test/s09" SCAN_ARCHIVES=false
check s15-image-no-archives 0 SCAN_IMAGE="docker://secret-scan-test/s15" SCAN_ARCHIVES=false
check s16-image-no-archives 1 SCAN_IMAGE="docker://secret-scan-test/s16" SCAN_ARCHIVES=false

# A saved image tarball works the same way.
docker save -o "$W/s15.tar" secret-scan-test/s15
check s15-tarball 1 SCAN_IMAGE="file://$W/s15.tar"

# Build-folder scan: catches what is in the folder, cannot see build args, and
# gitleaks does not unpack zip archives named .jar (s16); the image scan does.
for id in s01 s02 s03 s05 s06 s07 s09 s10 s11 s12 s13; do
  check "$id-folder" 1 SCAN_PATH="$CTX/$id"
done
for id in s04 s08 s14 s15 s16; do
  check "$id-folder" 0 SCAN_PATH="$CTX/$id"
done

# Both together, as used in release jobs.
for id in s01 s04 s09 s11; do
  check "$id-both" 1 SCAN_PATH="$CTX/$id" SCAN_IMAGE="docker://secret-scan-test/$id"
done

# Correctly built images are clean. c02 keeps its key in the build folder,
# which the folder scan rightly reports.
check c01-both 0 SCAN_PATH="$CTX/c01" SCAN_IMAGE="docker://secret-scan-test/c01"
check c02-image 0 SCAN_IMAGE="docker://secret-scan-test/c02"
check c02-folder 1 SCAN_PATH="$CTX/c02"

# Report-only mode passes with findings.
check s01-report-only 0 SCAN_IMAGE="docker://secret-scan-test/s01" ENFORCE=false

# Fail closed.
check missing-image 2 SCAN_IMAGE="docker://secret-scan-test/does-not-exist"
check missing-tarball 2 SCAN_IMAGE="file:///nonexistent/image.tar"
check missing-folder 2 SCAN_PATH="$CTX/does-not-exist"
check missing-allowlist 2 SCAN_IMAGE="docker://secret-scan-test/c01" ALLOWLIST=/nonexistent.json
check no-input 2
check bad-enforce 2 SCAN_PATH="$CTX/c01" ENFORCE=yes
check bad-scan-archives 2 SCAN_PATH="$CTX/c01" SCAN_ARCHIVES=yes
check missing-registry-image 2 SCAN_IMAGE="tykio/secret-scan-test-does-not-exist:none"

check dash-path 2 SCAN_PATH="-v"
check dash-image 2 SCAN_IMAGE="--help"
check dash-local-image 2 SCAN_IMAGE="docker://--help"

# The archive extractor only writes regular archive files inside its
# destination, whatever an image contains.
python3 -I - "$W/hostile.tar" <<'PY'
import io, sys, tarfile
with tarfile.open(sys.argv[1], "w") as t:
    def add(name, data=b"x", **kw):
        i = tarfile.TarInfo(name); i.size = len(data)
        for k, v in kw.items(): setattr(i, k, v)
        t.addfile(i, io.BytesIO(data) if i.isreg() else None)
    add("ok/good.tar.gz")
    add("../escape.tgz")
    add("/absolute.zip")
    add("a/../../escape2.zip")
    add("link.tgz", b"", type=tarfile.SYMTYPE, linkname="/etc/passwd")
    add("dir", b"", type=tarfile.SYMTYPE, linkname="/tmp")
    add("dir/through-link.zip")
    add("hard.zip", b"", type=tarfile.LNKTYPE, linkname="ok/good.tar.gz")
PY
rm -rf "$W/extract" && mkdir -p "$W/extract/dest"
written=$(python3 -I "$HERE/../../.github/actions/secret-scan/extract-archives.py" "$W/hostile.tar" "$W/extract/dest")
found=$(cd "$W/extract" && find . -mindepth 1 | sort | tr '\n' ' ')
# dir/through-link.zip is a regular file; because the "dir" symlink is never
# created, it lands in a real directory inside dest.
if [ "$written" = "2" ] && [ "$found" = "./dest ./dest/dir ./dest/dir/through-link.zip ./dest/ok ./dest/ok/good.tar.gz " ] &&
   [ -z "$(find "$W/extract" -type l)" ] && [ ! -e /tmp/through-link.zip ]; then
  echo "ok    extractor writes only safe archive files"
else
  echo "FAIL  extractor wrote $written file(s): $found"
  fail=1
fi

# A real multi-platform release image from Docker Hub: clean on every platform.
check portal-release 0 SCAN_IMAGE="tykio/portal:v1.20.1"
for p in linux/amd64 linux/arm64 linux/s390x; do
  if grep -q "$p" "$W/out/portal-release.log"; then
    printf 'ok    %-34s scanned\n' "portal-release $p"
  else
    printf 'FAIL  %-34s not scanned\n' "portal-release $p"
    fail=1
  fi
done

# Secret values never reach the log.
if grep -rqE 'ghp_[0-9A-Za-z]{36}|npm_[0-9A-Za-z]{36}|sk_live_[0-9A-Za-z]{24}|AKIA[A-Z2-7]{16}' "$W/out"/*.log; then
  echo "FAIL  a secret value appears in the scan output"
  fail=1
else
  echo "ok    no secret values in scan output"
fi

exit "$fail"
