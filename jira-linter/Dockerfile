# ---- Builder ----------------------------------------------------------------
# Pinned to the same Go version as go.mod and .github/workflows/ci-test.yml.
FROM golang:1.26 AS builder

WORKDIR /src

# Copy the module files first so dependency downloads stay cached when only
# the linter source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/

# CGO_ENABLED=0 produces a static binary that runs on a bare alpine image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/linter ./cmd/linter

# ---- Runner -----------------------------------------------------------------
FROM alpine:3.22

# ca-certificates: HTTPS calls to the Jira and GitHub APIs.
RUN apk add --no-cache ca-certificates

COPY --from=builder /out/linter /usr/local/bin/linter

# The image holds the compiled linter and nothing else. The jira-linter action
# runs it with `docker run`, and keeps the comment handling in its own steps.
ENTRYPOINT ["/usr/local/bin/linter"]
