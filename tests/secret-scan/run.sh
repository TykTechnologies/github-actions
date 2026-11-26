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
    return 1
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

# Only the action's inputs decide the rules and suppressions: files in the
# scanned folder or the working directory cannot switch the scan off.
TOKEN_FILE="$CTX/s01/.pat"
mkdir -p "$W/f-config" "$W/f-allow" "$W/f-ignore" "$W/f-cwd/build"
cp "$TOKEN_FILE" "$W/f-config/.pat"
printf '[[rules]]\nid = "nothing"\nregex = "zzzz-never-matches"\n' > "$W/f-config/.gitleaks.toml"
check folder-own-gitleaks-toml 1 SCAN_PATH="$W/f-config"
printf 'token = "%s"  # gitleaks:allow\n' "$(cat "$TOKEN_FILE")" > "$W/f-allow/app.py"
check folder-gitleaks-allow-comment 1 SCAN_PATH="$W/f-allow"
cp "$TOKEN_FILE" "$W/f-ignore/.pat"
gitleaks dir "$W/f-ignore" --no-banner --report-format json --report-path "$W/fp.json" > /dev/null 2>&1 || true
jq -r '.[].Fingerprint' "$W/fp.json" > "$W/f-ignore/.gitleaksignore"
check folder-own-gitleaksignore 1 SCAN_PATH="$W/f-ignore"
if grep -q unreviewed-gitleaksignore "$W/out/folder-own-gitleaksignore.log"; then
  echo "ok    folder-own-gitleaksignore reported"
else
  echo "FAIL  folder-own-gitleaksignore not reported"; fail=1
fi
cp "$TOKEN_FILE" "$W/f-cwd/build/.pat"
gitleaks dir "$W/f-cwd/build" --no-banner --report-format json --report-path "$W/fp2.json" > /dev/null 2>&1 || true
jq -r '.[].Fingerprint | sub("^[^:]*/build/"; "build/")' "$W/fp2.json" > "$W/f-cwd/.gitleaksignore"
jq -r '.[].Fingerprint' "$W/fp2.json" >> "$W/f-cwd/.gitleaksignore"
(cd "$W/f-cwd" && check cwd-gitleaksignore 1 SCAN_PATH=build) || fail=1

# Allowlist must be well formed.
echo '[{"detector":"Box"}]' > "$W/bad-allowlist-1.json"
echo '[{"detector":"Box","path":"(unclosed"}]' > "$W/bad-allowlist-2.json"
check allowlist-without-path 2 SCAN_IMAGE="docker://secret-scan-test/c01" ALLOWLIST="$W/bad-allowlist-1.json"
check allowlist-bad-regex 2 SCAN_IMAGE="docker://secret-scan-test/c01" ALLOWLIST="$W/bad-allowlist-2.json"

# A file name with a newline cannot start a workflow command of its own.
mkdir -p "$W/f-newline"
cp "$TOKEN_FILE" "$W/f-newline/$(printf 'a\n::warning::injected.pat')"
check folder-newline-name 1 SCAN_PATH="$W/f-newline"
if grep -q '^::warning::injected' "$W/out/folder-newline-name.log"; then
  echo "FAIL  a file name started a workflow command"; fail=1
else
  echo "ok    file names are escaped in annotations"
fi

# Both together, as used in release jobs.
for id in s01 s04 s09 s11; do
  check "$id-both" 1 SCAN_PATH="$CTX/$id" SCAN_IMAGE="docker://secret-scan-test/$id"
done

# Correctly built images are clean. c02 keeps its key in the build folder,
# which the folder scan rightly reports.
check c01-both 0 SCAN_PATH="$CTX/c01" SCAN_IMAGE="docker://secret-scan-test/c01"
check c02-image 0 SCAN_IMAGE="docker://secret-scan-test/c02"
check c02-folder 1 SCAN_PATH="$CTX/c02"

# The default allowlist drops dpkg checksum files only where dpkg keeps them.
check d01-dpkg-info 1 SCAN_IMAGE="docker://secret-scan-test/d01" SCAN_ARCHIVES=false
if grep -q '/opt/notes/busybox.md5sums' "$W/out/d01-dpkg-info.log" &&
   ! grep -q '/var/lib/dpkg/info/' "$W/out/d01-dpkg-info.log"; then
  echo "ok    d01 dpkg/info allowlisted, same file elsewhere still reported"
else
  echo "FAIL  d01 dpkg/info allowlist not applied as expected"; fail=1
fi

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

check archive-over-limit 2 SCAN_IMAGE="docker://secret-scan-test/s09" MAX_ARCHIVE_MB=0
check bad-max-archive 2 SCAN_IMAGE="docker://secret-scan-test/s09" MAX_ARCHIVE_MB=lots
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

# Registry images with a single manifest (classic build, or buildx without
# provenance) are scanned by digest, and an image the job already has locally
# is never removed: the job's next docker tag / push must still work.
REG_PORT=${SECRET_SCAN_TEST_REGISTRY_PORT:-5055}
REG="localhost:$REG_PORT"
docker rm -f secret-scan-test-registry > /dev/null 2>&1 || true
docker pull -q registry:2 > /dev/null
docker run -d --name secret-scan-test-registry -p "$REG_PORT:5000" registry:2 > /dev/null
for _ in 1 2 3 4 5 6 7 8 9 10; do curl -fsS "http://$REG/v2/" > /dev/null 2>&1 && break; sleep 1; done
for id in c01 s01; do
  docker buildx build -q --provenance=false --sbom=false --load -t "$REG/ss/$id:rel" "$CTX/$id" > /dev/null
  docker push -q "$REG/ss/$id:rel" > /dev/null
done
check single-manifest-clean 0 SCAN_IMAGE="$REG/ss/c01:rel"
if docker tag "$REG/ss/c01:rel" "$REG/public/c01:rel" 2> /dev/null; then
  echo "ok    single-manifest local tag kept for the next push"
else
  echo "FAIL  single-manifest scan removed the job's local tag"; fail=1
fi
if grep -q "$REG/ss/c01@sha256:" "$W/out/single-manifest-clean.log"; then
  echo "ok    single-manifest scanned by digest"
else
  echo "FAIL  single-manifest not scanned by digest"; fail=1
fi
check single-manifest-leak 1 SCAN_IMAGE="$REG/ss/s01:rel"
# Not present locally: pulled for the scan, then removed again.
docker rmi "$REG/ss/s01:rel" > /dev/null
check single-manifest-pulled 1 SCAN_IMAGE="$REG/ss/s01:rel"
if [ -z "$(docker images -q "$REG/ss/s01")" ]; then
  echo "ok    image pulled by the scan is removed afterwards"
else
  echo "FAIL  image pulled by the scan was left behind"; fail=1
fi
# Provenance attestations: with mode=max, BuildKit publishes every build arg
# value with the image. The ARGs are used only in a builder stage, so no layer,
# config or history entry of the final image holds them (an ARG in the final
# stage would show in its history, which the layer scan already reads): only
# the attestation does.
mkdir -p "$W/prov"
printf 'FROM alpine:3.20 AS build\nARG NOTE\nARG DB_PASSWORD\nARG BUILD_PACKAGE_NAME\nRUN echo built > /built\nFROM alpine:3.20\nCOPY --from=build /built /built\n' > "$W/prov/Dockerfile"
docker buildx rm secret-scan-test > /dev/null 2>&1 || true
if ! docker buildx create --name secret-scan-test --driver docker-container --driver-opt network=host > /dev/null; then
  echo "FAIL  cannot create the buildx builder for the provenance tests"; fail=1
fi
prov_push() {
  local tag=$1 mode=$2 provenance; shift 2
  provenance="mode=$mode"; [ "$mode" = none ] && provenance=false
  if ! docker buildx build -q --builder secret-scan-test --provenance="$provenance" --sbom=false \
       "$@" --push -t "$REG/ss/prov:$tag" "$W/prov" > /dev/null; then
    echo "FAIL  cannot build and push provenance image $tag"; fail=1
  fi
}
prov_push token max --build-arg NOTE="$(cat "$TOKEN_FILE")"
prov_push password max --build-arg DB_PASSWORD=correct-horse-battery
prov_push clean max --build-arg BUILD_PACKAGE_NAME=tyk-gateway
prov_push token-min min --build-arg NOTE="$(cat "$TOKEN_FILE")"
prov_push benign-names max --build-arg GPG_KEY_ID=ABCDEF0123456789 --build-arg PUBLIC_KEY=ssh-ed25519 --build-arg NPM_TOKEN=
prov_push none none --build-arg NOTE="$(cat "$TOKEN_FILE")"

check provenance-token 1 SCAN_IMAGE="$REG/ss/prov:token" SCAN_ARCHIVES=false
if grep -q 'gitleaks-provenance' "$W/out/provenance-token.log"; then
  echo "ok    provenance-token found by gitleaks in the attestation"
else
  echo "FAIL  provenance-token not reported as a provenance finding"; fail=1
fi
check provenance-secret-name 1 SCAN_IMAGE="$REG/ss/prov:password" SCAN_ARCHIVES=false
if grep -q 'secret-named-build-arg' "$W/out/provenance-secret-name.log" &&
   grep -q 'build-arg:DB_PASSWORD' "$W/out/provenance-secret-name.log" &&
   ! grep -q 'correct-horse-battery' "$W/out/provenance-secret-name.log"; then
  echo "ok    provenance-secret-name reported by name, value not printed"
else
  echo "FAIL  provenance-secret-name not reported as expected"; fail=1
fi
check provenance-clean 0 SCAN_IMAGE="$REG/ss/prov:clean" SCAN_ARCHIVES=false
if grep -q 'scanned:.*provenance' "$W/out/provenance-clean.log"; then
  echo "ok    provenance-clean attestation scanned"
else
  echo "FAIL  provenance-clean attestation not scanned"; fail=1
fi
# The leak is only in the attestation: without the attestation scan it passes,
# and mode=min does not record build args at all.
check provenance-token-off 0 SCAN_IMAGE="$REG/ss/prov:token" SCAN_ARCHIVES=false SCAN_ATTESTATIONS=false
check provenance-token-mode-min 0 SCAN_IMAGE="$REG/ss/prov:token-min" SCAN_ARCHIVES=false
if [ -n "$(find "$W/out/provenance-token" "$W/out/provenance-secret-name" -name 'provenance*.json*')" ]; then
  echo "FAIL  provenance files holding build arg values were left in the results folder"; fail=1
else
  echo "ok    provenance files removed from the results folder"
fi
# Names that only look like secrets, and empty values, are not findings; a
# real one can be allowlisted by rule and build arg.
check provenance-benign-names 0 SCAN_IMAGE="$REG/ss/prov:benign-names" SCAN_ARCHIVES=false
echo '[{"detector":"secret-named-build-arg","path":"^build-arg:DB_PASSWORD$","reason":"test"}]' > "$W/prov-allowlist.json"
check provenance-allowlisted 0 SCAN_IMAGE="$REG/ss/prov:password" SCAN_ARCHIVES=false ALLOWLIST="$W/prov-allowlist.json"
# One token is recorded several times in a statement but reported once.
if [ "$(grep -c 'gitleaks-provenance' "$W/out/provenance-token.log")" = 1 ]; then
  echo "ok    provenance-token reported once"
else
  echo "FAIL  provenance-token reported $(grep -c 'gitleaks-provenance' "$W/out/provenance-token.log") times"; fail=1
fi
# An image without attestations passes with a warning, never silently.
check provenance-none 0 SCAN_IMAGE="$REG/ss/prov:none" SCAN_ARCHIVES=false
if grep -q '::warning title=secret-scan::no provenance attestation found' "$W/out/provenance-none.log" &&
   grep -q 'provenance (none)' "$W/out/provenance-none.log"; then
  echo "ok    provenance-none warned"
else
  echo "FAIL  provenance-none did not warn"; fail=1
fi
check bad-scan-attestations 2 SCAN_PATH="$CTX/c01" SCAN_ATTESTATIONS=yes
docker buildx rm secret-scan-test > /dev/null 2>&1 || true

docker rm -f secret-scan-test-registry > /dev/null
docker rmi "$REG/ss/c01:rel" "$REG/public/c01:rel" > /dev/null 2>&1 || true

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
# Its mode=max provenance (one statement per platform) is scanned too, with no
# false positives.
if grep -q 'scanned:.*provenance' "$W/out/portal-release.log"; then
  echo "ok    portal-release provenance scanned"
else
  echo "FAIL  portal-release provenance not scanned"; fail=1
fi

# With REPORT_DETAILS=false the finding still fails the scan and is in the
# report, but its file and rule stay out of the log and the job summary.
mkdir -p "$W/out/quiet"
: > "$W/out/quiet/summary.md"; : > "$W/out/quiet/output"
got=0
REPORT_DETAILS=false SCAN_PATH="$CTX/s01" OUT_DIR="$W/out/quiet/scan" \
  GITHUB_STEP_SUMMARY="$W/out/quiet/summary.md" GITHUB_OUTPUT="$W/out/quiet/output" \
  bash "$SCAN" > "$W/out/quiet.log" 2>&1 || got=$?
report=$(sed -n 's/^report=//p' "$W/out/quiet/output")
if [ "$got" = 1 ] \
  && ! grep -q '\.pat' "$W/out/quiet.log" "$W/out/quiet/summary.md" \
  && grep -q 'withheld' "$W/out/quiet.log" \
  && [ "$report" = "$W/out/quiet/scan/report/findings.jsonl" ] && grep -q '\.pat' "$report" \
  && [ "$(ls -A "$W/out/quiet/scan/report")" = "findings.jsonl" ]; then
  echo "ok    report-details=false withholds details, keeps the report"
else
  echo "FAIL  report-details=false (exit $got, report '$report')"
  sed 's/^/        /' "$W/out/quiet.log" "$W/out/quiet/summary.md"
  fail=1
fi
check bad-report-details 2 REPORT_DETAILS=maybe SCAN_PATH="$CTX/s01"

# The same for an image: neither the file nor the detector shows in the log.
check quiet-image 0 REPORT_DETAILS=false ENFORCE=false SCAN_IMAGE="docker://secret-scan-test/s01"
if grep -qiE '\.pat|github' "$W/out/quiet-image.log"; then
  echo "FAIL  quiet-image log names the finding"; sed 's/^/        /' "$W/out/quiet-image.log"; fail=1
elif ! grep -q '\.pat' "$W/out/quiet-image/report/findings.jsonl"; then
  echo "FAIL  quiet-image finding missing from the report"; fail=1
else
  echo "ok    quiet-image withholds details, keeps the report"
fi

# A scanner error's log can name files in the image: withheld too.
check quiet-scanner-error 2 REPORT_DETAILS=false SCAN_IMAGE="docker://secret-scan-test/s09" MAX_ARCHIVE_MB=0
if grep -q 'archive larger than' "$W/out/quiet-scanner-error.log"; then
  echo "FAIL  quiet-scanner-error log names a file in the image"; sed 's/^/        /' "$W/out/quiet-scanner-error.log"; fail=1
elif ! grep -q 'archive larger than' "$W/out/archive-over-limit.log"; then
  echo "FAIL  archive-over-limit log no longer names the archive, so quiet-scanner-error proves nothing"; fail=1
else
  echo "ok    quiet-scanner-error withholds the scanner log"
fi

# Docker's own errors (manifest, pull, provenance) are withheld the same way.
check quiet-registry-error 2 REPORT_DETAILS=false SCAN_IMAGE="tykio/secret-scan-test-does-not-exist:none"
if grep -q 'withheld' "$W/out/quiet-registry-error.log" && ! grep -qi 'not found\|denied\|unauthorized' "$W/out/quiet-registry-error.log"; then
  echo "ok    quiet-registry-error withholds docker's error"
else
  echo "FAIL  quiet-registry-error shows docker's error"; sed 's/^/        /' "$W/out/quiet-registry-error.log"; fail=1
fi

# Secret values never reach the log.
if grep -rqE 'ghp_[0-9A-Za-z]{36}|npm_[0-9A-Za-z]{36}|sk_live_[0-9A-Za-z]{24}|AKIA[A-Z2-7]{16}' "$W/out"/*.log; then
  echo "FAIL  a secret value appears in the scan output"
  fail=1
else
  echo "ok    no secret values in scan output"
fi

exit "$fail"
