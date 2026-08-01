## Dependency EoL Notifier

A scheduled job that reads the [endoflife.date](https://endoflife.date) API every
day at 07:00 UTC and posts one message to Slack. The message goes to the channel
of the webhook held in `EOL_SLACK_WEBHOOK_URL`, which is `#service-EoL-dependency`.

The job sends two kinds of alert:

- **Approaching end of life** — a tracked version reaches the end of a lifecycle
  phase in exactly 12, 6 or 1 month.
- **New versions** — the API lists a version that the run before did not see.

When there is nothing to report, the job posts no message at all.

Acting on an alert is manual work. Adding a version to the test matrix, or taking
one out of it, needs a PR against this repository. For a removal, the dependency
policy asks for commercial approval first.

### Configuration

The tracked dependencies are listed in
[.github/eol-notifier/dependencies.yaml](/.github/eol-notifier/dependencies.yaml).
Each entry maps one dependency to an endoflife.date product slug:

```yaml
thresholds_months: [12, 6, 1]

dependencies:
  - name: Amazon RDS PostgreSQL
    product: amazon-rds-postgresql
    track: [eol, eoes]

  - name: GCP Cloud SQL
    product: postgresql
    upstream_proxy: true
```

`thresholds_months` sets how long before the end date an alert goes out. It is
optional, and `[12, 6, 1]` is the default.

`track` selects the lifecycle phases to watch: `eol`, `eoas` (active support) or
`eoes` (extended support). It is optional and defaults to `[eol]`. Not every
product publishes every phase.

`upstream_proxy` marks a service that endoflife.date does not track, such as GCP
MemoryStore or Azure DocumentDB. For those, the job follows the upstream engine
instead, so the date is only a hint. The alert marks the entry, and the footer of
the message says the date has to be confirmed with the cloud provider.

The job reads this file before it calls the API. An unknown phase name, an empty
product or a repeated dependency name stops the run before any request is sent.

### Recorded state

To know that a version is *new*, the job needs to know what the run before it
saw. It keeps that in a `state.json` file on a branch of its own, called
`eol-notifier-state`. It is a branch and not a file on `main`, because `main`
needs a reviewed PR and the job cannot push there.

The branch holds that one file and nothing else. The job commits only when the
content changes, so expect a few commits a year, not one a day. It creates the
branch itself on the first run.

If you delete the branch, the next run starts from zero. It records the versions
the API lists that day and reports none of them as new. Versions that appeared
while the branch was gone become part of that new record, so they are never
announced. A dependency added to the config behaves the same way: its first run
only records.

End-of-life alerts do not use the state file. They are also the part with no
second chance. The alert goes out on the single day that is exactly 12, 6 or 1
month before the end date, which is what the policy asks for, so an alert is lost
when the run of that day is skipped or fails. The next threshold still fires, so
a missed 12-month alert is followed by the 6-month one. If you expected an alert
and it never arrived, check the run history of the workflow.

### Failures

A product the API does not return is logged and skipped. The job still posts the
alerts for the other products, and then fails, so the problem shows up in the
Actions tab. The state file is still written in that case. Without that, the same
new versions would be announced again the next day.

### Requirements

The repository secret `EOL_SLACK_WEBHOOK_URL` must hold an incoming webhook for
the target channel. The job cannot post without it.

### Manual runs

Start the workflow by hand with `workflow_dispatch`. With `dry_run: true` it
writes the message into the job log, posts nothing to Slack and leaves the state
file alone. A dry run shows only what is due on that day. It does not report the
lifecycle of every tracked product.

Adoption: Internal use for the scheduled job on this repository.
