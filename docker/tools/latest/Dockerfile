FROM golang:1.23 AS tools-build

ENV CGO_ENABLED=0
ENV GOBIN=/usr/local/bin
ENV GOPROXY=direct
WORKDIR /usr/local/bin

RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/httpbin-logserver@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/workflow-lint@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/schema-gen@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/summary@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/modcheck@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/TykTechnologies/exp/cmd/go-fsck@main
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install go.uber.org/mock/mockgen@v0.4.0
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install golang.org/x/tools/cmd/goimports@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/fatih/faillint@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install gotest.tools/gotestsum@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/wadey/gocovmerge@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/evilmartians/lefthook@latest
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go install github.com/bwplotka/mdox@latest

ARG VENOM_VERSION=v1.2.0
RUN curl -L https://github.com/ovh/venom/releases/download/${VENOM_VERSION}/venom.linux-amd64 -o /usr/local/bin/venom && chmod +x /usr/local/bin/venom

ARG TASK_VERSION=v3.27.1
RUN curl -sSL https://github.com/go-task/task/releases/download/${TASK_VERSION}/task_linux_amd64.tar.gz | tar -zxv

# Final tools image
#
# This uses the `scratch` as the image base. The image
# doesn't need a base OS, as it's intended to be used
# in CI pipelines that do. Example action:
#
# ```
# - uses: shrink/actions-docker-extract@v3
#   with:
#     image: <tools>
#     path: /usr/local/bin/.
#     destination: /usr/local/bin
# ```
#
# https://github.com/marketplace/actions/docker-extract

FROM scratch
COPY --from=tools-build /usr/local/bin/ /usr/local/bin/
COPY --from=busybox:latest /bin/ls /bin/ls

ENTRYPOINT ["/bin/ls"]
