# CI tools

Providing the docker image with CI tooling avoids continous compilation
of the tools from using `go install`, decreasing resource usage on GitHub
actions. Adopters can extract single binaries for use.

This uses Go 1.23 (or latest version) to build the CI tooling.

## Development

We build a docker image from the CI pipeline in this repository that
builds and installs all the CI tooling needed for the test pipelines.

- Image rebuilds based on a schedule, 1x / week
- Image rebuilds on changes from exp/cmd repository

The experimental repository holds several tools that ensure static code
analysis, or aid in automation tasks.

All the tools are built using a recent go version and `CGO_ENABLED=0`,
enabling reuse for old releases. It's still possible to version the
tooling against releases either inside the image, or by creating new
versions of the docker image in the future.

## Local testing

Run `task` to build all local images. It will build:

- `internal/ci-tools:latest`

Inspect other taskfile targets with `task -l`.

## CI tools

The images built are:

- `tykio/ci-tools:latest`.

The image is rebuilt weekly.

To use the CI tools from any github pipeline:

```yaml
- name: 'Extract CI tools'
  uses: shrink/actions-docker-extract@v3
  with:
    image: tykio/ci-tools:latest
    path: /usr/local/bin/.
    destination: /usr/local/bin

- run: gotestsum --version
```

To use a single tool replace the `.` in the `path` value with the binary
you want. This allows you to extract only what's used in the pipeline,
for example, if you only need `gocovmerge`:

```yaml
- name: 'Extract gocovmerge'
  uses: shrink/actions-docker-extract@v3
  with:
    image: tykio/ci-tools:latest
    path: /usr/local/bin/gocovmerge
    destination: /usr/local/bin

- run: gotestsum --version
```

## References

- Uses [shrink/actions-docker-extract](https://github.com/shrink/actions-docker-extract)
- Tools installed configured via [docker/tools/latest/Dockerfile](https://github.com/TykTechnologies/github-actions/blob/main/docker/tools/latest/Dockerfile#L8-L20)
- [Used in Tyk Gateway](https://github.com/TykTechnologies/tyk/blob/master/.github/workflows/ci-tests.yml#L62)
- [Used in Tyk Dashboard - golangci-lint](https://github.com/TykTechnologies/tyk-analytics/blob/master/.github/workflows/ci-tests.yml#L39)
- [Used in Tyk Dashboard - goimports](https://github.com/TykTechnologies/tyk-analytics/blob/master/.github/workflows/ci-tests.yml#L142)
