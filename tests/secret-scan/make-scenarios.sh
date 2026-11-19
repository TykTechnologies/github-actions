#!/usr/bin/env bash
# Builds one image per way a secret is commonly baked into an image by mistake,
# plus two images built the correct way. All credentials are generated fresh,
# are fake, and never leave the runner.
# Usage: make-scenarios.sh <workdir>   (build folders go to <workdir>/ctx/<id>)
# Dockerfile ${VARS} are expanded by docker, not by this script.
# shellcheck disable=SC2016
set -euo pipefail
W=${1:?workdir}
rm -rf "$W/ctx" && mkdir -p "$W/ctx"
cd "$W/ctx"

b62() { python3 -I -c '
import random,zlib,string,sys
a=string.digits+string.ascii_uppercase+string.ascii_lowercase
p=sys.argv[1]; b="".join(random.choice(a) for _ in range(30))
c=zlib.crc32(b.encode()); s=""
while c: c,r=divmod(c,62); s=a[r]+s
print(p+b+s.rjust(6,"0"))' "$1"; }
rnd() { python3 -I -c 'import random,sys,re,string; pool="".join(c for c in string.printable if re.fullmatch("["+sys.argv[1]+"]",c)); print("".join(random.choice(pool) for _ in range(int(sys.argv[2]))))' "$1" "$2"; }

GH=$(b62 ghp_)
NPM=$(b62 npm_)
AWS_ID="AKIA$(rnd 'A-Z2-7' 16)"
AWS_SECRET=$(rnd 'A-Za-z0-9' 40)
SLACK="https://hooks.slack.com/services/T$(rnd 'A-Z0-9' 10)/B$(rnd 'A-Z0-9' 10)/$(rnd 'A-Za-z0-9' 24)"
STRIPE="sk_live_$(rnd 'A-Za-z0-9' 24)"
openssl genrsa 2048 2>/dev/null > id_rsa

mk() { mkdir -p "$1"; }

# 01 INC-47: token file in the app folder, COPY . copies dotfiles
mk s01; echo "$GH" > s01/.pat; echo 'print("app")' > s01/app.py
printf 'FROM alpine:3.20\nWORKDIR /srv/app\nCOPY . .\n' > s01/Dockerfile

# 02 .env with cloud credentials copied with the source
mk s02; printf 'AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nSTRIPE_KEY=%s\n' "$AWS_ID" "$AWS_SECRET" "$STRIPE" > s02/.env
printf 'FROM alpine:3.20\nWORKDIR /app\nCOPY . .\n' > s02/Dockerfile

# 03 .npmrc used for a private install, deleted in a LATER layer
mk s03; printf '//registry.npmjs.org/:_authToken=%s\n' "$NPM" > s03/.npmrc
printf 'FROM alpine:3.20\nWORKDIR /app\nCOPY .npmrc .\nRUN echo "npm install would run here"\nRUN rm .npmrc\n' > s03/Dockerfile

# 04 build arg used in a RUN line (ends up in image history)
mk s04; printf 'FROM alpine:3.20\nARG GITHUB_TOKEN\nRUN echo "git clone https://x-access-token:${GITHUB_TOKEN}@github.com/org/private.git" > /dev/null\n' > s04/Dockerfile

# 05 ENV with a token (ends up in image config)
mk s05; printf 'FROM alpine:3.20\nENV SLACK_WEBHOOK=%s\nENV GITHUB_TOKEN=%s\n' "$SLACK" "$GH" > s05/Dockerfile

# 06 LABEL with a token (image config)
mk s06; printf 'FROM alpine:3.20\nLABEL deploy.token="%s"\n' "$GH" > s06/Dockerfile

# 07 SSH key copied, used, removed in a later layer
mk s07; cp id_rsa s07/id_rsa
printf 'FROM alpine:3.20\nCOPY id_rsa /root/.ssh/id_rsa\nRUN echo "git clone over ssh would run here"\nRUN rm /root/.ssh/id_rsa\n' > s07/Dockerfile

# 08 token compiled into a Go binary with -ldflags -X
mk s08; printf 'package main\nimport "fmt"\nvar token string\nfunc main(){ fmt.Println(len(token)) }\n' > s08/main.go
printf 'module leak\ngo 1.26\n' > s08/go.mod
printf 'FROM golang:1.26-alpine AS build\nARG TOKEN\nWORKDIR /src\nCOPY . .\nRUN CGO_ENABLED=0 go build -ldflags "-X main.token=${TOKEN}" -o /app .\nFROM alpine:3.20\nCOPY --from=build /app /app\n' > s08/Dockerfile

# 09 credentials inside a tar.gz bundled into the image
mk s09; mkdir -p s09/bundle; printf '[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n' "$AWS_ID" "$AWS_SECRET" > s09/bundle/credentials
tar -C s09 -czf s09/config-bundle.tar.gz bundle; rm -r s09/bundle
printf 'FROM alpine:3.20\nCOPY config-bundle.tar.gz /opt/\n' > s09/Dockerfile

# 10 base64-encoded token in a Kubernetes Secret manifest
mk s10; printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: app\ndata:\n  token: %s\n' "$(printf %s "$GH" | base64)" > s10/secret.yaml
printf 'FROM alpine:3.20\nCOPY secret.yaml /deploy/\n' > s10/Dockerfile

# 11 .git folder copied with a token in the remote URL (actions/checkout style)
mk s11; git -C s11 init -q; git -C s11 remote add origin "https://x-access-token:${GH}@github.com/org/repo.git"; echo x > s11/f
printf 'FROM alpine:3.20\nWORKDIR /src\nCOPY . .\n' > s11/Dockerfile

# 12 Docker registry login copied into the image
mk s12/.docker; printf '{"auths":{"https://index.docker.io/v1/":{"auth":"%s"}}}\n' "$(printf 'tykbot:%s' "$GH" | base64)" > s12/.docker/config.json
printf 'FROM alpine:3.20\nCOPY .docker /root/.docker\n' > s12/Dockerfile

# 13 shell history left behind by a debug RUN
mk s13; printf 'FROM alpine:3.20\nRUN echo "export STRIPE_KEY=%s" > /root/.ash_history\n' "$STRIPE" > s13/Dockerfile

# 14 log file from a build step that printed the token
mk s14; printf 'FROM alpine:3.20\nARG NPM_TOKEN\nRUN mkdir -p /root/.npm/_logs && echo "verbose request using token ${NPM_TOKEN}" > /root/.npm/_logs/debug.log\n' > s14/Dockerfile

# 15 credentials archived in a builder stage, only the archive is copied into the final image
mk s15; printf 'FROM alpine:3.20 AS build\nARG AWS_ID\nARG AWS_SECRET\nRUN mkdir -p /b && printf "[default]\\naws_access_key_id = %%s\\naws_secret_access_key = %%s\\n" "$AWS_ID" "$AWS_SECRET" > /b/credentials && tar -czf /bundle.tgz -C / b\nFROM alpine:3.20\nCOPY --from=build /bundle.tgz /opt/bundle.tgz\n' > s15/Dockerfile

# 16 credentials inside a .jar (a zip under another name) copied into the image
mk s16; python3 -I -c 'import sys,zipfile; z=zipfile.ZipFile(sys.argv[1],"w"); z.writestr("BOOT-INF/classes/application.properties","aws.accessKeyId=%s\naws.secretKey=%s\n" % (sys.argv[2], sys.argv[3])); z.close()' s16/app.jar "$AWS_ID" "$AWS_SECRET"
printf 'FROM alpine:3.20\nCOPY app.jar /opt/app.jar\n' > s16/Dockerfile

# C1 negative control: BuildKit secret mount (correct way)
mk c01; echo "$GH" > c01_token.txt
printf '# syntax=docker/dockerfile:1\nFROM alpine:3.20\nRUN --mount=type=secret,id=gh test -s /run/secrets/gh && echo used\n' > c01/Dockerfile

# C2 negative control: key only in the builder stage of a multi-stage build
mk c02; cp id_rsa c02/id_rsa
printf 'FROM alpine:3.20 AS build\nCOPY id_rsa /root/.ssh/id_rsa\nRUN echo built > /out\nFROM alpine:3.20\nCOPY --from=build /out /out\n' > c02/Dockerfile

build() { docker build -q "$@" >/dev/null; }
for s in s01 s02 s03 s05 s06 s07 s09 s10 s11 s12 s13 s16; do build -t "secret-scan-test/$s" "$s"; done
build -t secret-scan-test/s04 --build-arg GITHUB_TOKEN="$GH" s04
build -t secret-scan-test/s08 --build-arg TOKEN="$GH" s08
build -t secret-scan-test/s14 --build-arg NPM_TOKEN="$NPM" s14
build -t secret-scan-test/s15 --build-arg AWS_ID="$AWS_ID" --build-arg AWS_SECRET="$AWS_SECRET" s15
build -t secret-scan-test/c01 --secret id=gh,src=c01_token.txt c01
build -t secret-scan-test/c02 c02
rm -f id_rsa c01_token.txt
echo built
