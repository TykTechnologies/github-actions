## CI tools

We build a docker image from the CI pipeline in this repository that
builds and installs all the CI tooling needed for the test pipelines.

Providing the docker image avoids continous compilation of the tools from
using `go install` or `go get`, decreasing resource usage on GitHub
actions.

All the tools are built using a recent go version and `CGO_ENABLED=0`,
enabling reuse for old releases. It's still possible to version the
tooling against releases either inside the image, or by creating new
versions of the docker image in the future.

The images built are:

- `tykio/ci-tools:latest`.

The image is rebuilt weekly and on triggers from `exp/cmd`.

To use the CI tools from any github pipeline:

```yaml
- name: 'Extract tykio/ci-tools:${{ matrix.tag }}'
  uses: shrink/actions-docker-extract@v3
  with:
    image: tykio/ci-tools:${{ matrix.tag }}
    path: /usr/local/bin/.
    destination: /usr/local/bin

- run: gotestsum --version
```

The action
[shrink/actions-docker-extract](https://github.com/shrink/actions-docker-extract)
is used to download and extract the CI tools binaries into your CI
workflow. The set of tools being provided can be adjusted in
[docker/tools/latest/Dockerfile](https://github.com/TykTechnologies/tyk-github-actions/blob/main/docker/tools/latest/Dockerfile).

A local Taskfile is available in `docker/tools/` that allows you to build
the tools image locally. Changes are tested in PRs.
