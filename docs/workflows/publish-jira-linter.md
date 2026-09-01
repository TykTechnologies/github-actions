## Publish jira-linter image

We build a docker image from the CI pipeline in this repository that
compiles the Jira linter and packages it for the `jira-linter` action.

Providing the docker image avoids compiling the linter with
`actions/setup-go` and `go install` on every pull request run, decreasing
resource usage on GitHub actions. The `jira-linter` action references the
published image instead of building from source.

The linter is built with `CGO_ENABLED=0` in a `golang:1.24.7-alpine`
builder stage, then copied into an `alpine:3.22` runner stage. The image
is built for `linux/amd64`.

The images built are:

- `tykio/jira-linter:latest`.

The image is rebuilt on changes to `jira-linter/`, on merges to `main`,
and weekly to pick up base image patches.

Pull requests build the image and smoke test it without pushing, so a
change to the `Dockerfile` or the entrypoint is verified before it
merges. Registry credentials are skipped on pull requests.

To roll the image back, run the workflow through `workflow_dispatch`
against the last known good ref. The run rebuilds and overwrites
`tykio/jira-linter:latest`.

The image is referenced by
[jira-linter/action.yaml](https://github.com/TykTechnologies/github-actions/blob/main/jira-linter/action.yaml),
and the build is defined in
[jira-linter/Dockerfile](https://github.com/TykTechnologies/github-actions/blob/main/jira-linter/Dockerfile).

Adoption: Internal use, consumed by the `jira-linter` action.
