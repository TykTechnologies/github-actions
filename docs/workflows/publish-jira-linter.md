## Publish jira-linter image

We build a docker image from the CI pipeline in this repository that
compiles the Jira linter and packages it for the `jira-linter` action.

Providing the docker image avoids compiling the linter with
`actions/setup-go` and `go install` on every pull request run, decreasing
resource usage on GitHub actions.

The linter is built with `CGO_ENABLED=0` in a `golang:1.24.7-alpine`
builder stage, then copied into an `alpine:3.22` runner stage. The image
is built for `linux/amd64`.

The images built are:

- `754489498669.dkr.ecr.eu-central-1.amazonaws.com/jira-linter:latest`.

The image is rebuilt on changes to `jira-linter/`, on merges to `main`,
and weekly to pick up base image patches.

Pushes authenticate to Amazon ECR through OIDC, assuming
`arn:aws:iam::754489498669:role/ecr_rw_tyk` in `eu-central-1`. This is the
same role and region used by the SBOM workflows, so the job requires
`id-token: write`.

Pull requests build the image and smoke test it without pushing, so a
change to the `Dockerfile` or the entrypoint is verified before it
merges. AWS credentials are skipped on pull requests.

To roll the image back, run the workflow through `workflow_dispatch`
against the last known good ref. The run rebuilds and overwrites the
`latest` tag.

The build is defined in
[jira-linter/Dockerfile](https://github.com/TykTechnologies/github-actions/blob/main/jira-linter/Dockerfile).

Adoption: Internal use, consumed by the `jira-linter` action.
