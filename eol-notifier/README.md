# EoL Notifier

This action reads the [endoflife.date](https://endoflife.date) API and sends one
Slack message per run. It sends an alert when:

- a version we track gets close to the end of one of its support phases, or
- a new version appears.

The action only sends alerts. It does not change any test matrix. Adding or
removing a version is still a manual PR in this repository, and only after the
approval that the dependency policy asks for.

## Usage

```yaml
- name: Check dependency lifecycles
  uses: TykTechnologies/github-actions/eol-notifier@main
  with:
    config-path: .github/eol-notifier/dependencies.yaml
    state-path: ${{ runner.temp }}/state.json
    slack-webhook-url: ${{ secrets.EOL_SLACK_WEBHOOK_URL }}
```

| Input               | Required | Default | Description                                                                     |
|---------------------|----------|---------|---------------------------------------------------------------------------------|
| `config-path`       | yes      |         | YAML file with the list of dependencies to track                                |
| `state-path`        | yes      |         | JSON file with the versions the last run saw and the alerts already sent        |
| `slack-webhook-url` | no       |         | Slack incoming webhook for the channel. Needed unless `dry-run` is `true`       |
| `dry-run`           | no       | `false` | Print the message in the job log, send nothing, and do not touch the state file |

The caller owns both files. So a second workflow can track other dependencies
with its own config file and its own state file.

The action reads and writes `state-path`, but it does not keep the file between
runs. That is the workflow's job. In this repository the file lives on a branch
called `eol-notifier-state`, because `main` needs a reviewed PR and the job
cannot push to it. See [.github/workflows/eol-notifier.yaml](/.github/workflows/eol-notifier.yaml).

## Config

```yaml
thresholds_months: [12, 6, 1]

dependencies:
  - name: PostgreSQL
    product: postgresql
    track: [eol]

  - name: Amazon RDS PostgreSQL
    product: amazon-rds-postgresql
    track: [eol, eoes]

  - name: GCP Cloud SQL
    product: postgresql
    upstream_proxy: true
```

`thresholds_months` sets how many months before the end date to send an alert.
The default is `[12, 6, 1]`.

Each entry under `dependencies` takes these keys:

- `name` is the name shown in the alert. It must be unique.
- `product` is the product name that endoflife.date uses in its URL. Two
  dependencies can use the same product. The action then calls the API once and
  puts both names in the same alert.
- `track` lists the support phases to watch. The default is `[eol]`.
  - `eol` is the end of life, or the end of security support. Every product has
    this phase, but a version keeps no date until the vendor announces one.
  - `eoas` is the end of active support. For example `redis` and `valkey`.
  - `eoes` is the end of extended support. For example `amazon-rds-postgresql`.
- `upstream_proxy` is for a service that `endoflife.date` does not track. The
  action then uses the dates of the open source engine under it. GCP MemoryStore
  uses `redis`, GCP Cloud SQL uses `postgresql`, and Azure DocumentDB uses
  `mongodb`. The alert marks these dates as a hint only, because a cloud
  provider usually supports a version for a different length of time than the
  open source project.

The action checks the config before it makes any network call. It fails the run
if a phase name is unknown, a product is empty, or a key is misspelled. It also
fails if the list of dependencies is empty, if a name or a phase is listed twice,
or if a threshold is repeated or is not above zero.

## How it works

### Alerts before the end of support

The action looks at every phase it tracks, but only where the API gives an end
date. Some versions have no date yet, and the action skips those. It sends an
alert when the date is 12, 6 or 1 month away.

It counts backwards from the end date. If the target month is shorter, it uses
the last day of that month. For example, one month before 31 March is
28 February, and 29 February in a leap year.

An alert stays due from the day it comes up until the day it is sent. So a day,
a week or a year with no run costs nothing: the first run after the gap sends
everything that came up while the action was down. The state file lists the
alerts already sent, so each one still goes out once and once only.

### Alerts after the end of support

When the end date itself passes, the action sends one alert saying the phase has
ended. It replaces the countdown for that version, because a version that is
already out of support is not one month from anything.

This alert is sent once, on the first run after the date passes. If
endoflife.date later moves the date, the new date is a new alert.

### Alerts for new versions

The state file lists the versions that the last run saw. If the API shows a
version that is not in the file, the action sends an alert.

Sometimes a version is already end of life when it first appears. This is an old
version that someone added to endoflife.date later, so it is not news. The action
saves it and sends no new-version alert, only the alert that says the phase has
ended.

A product the state file has never seen is a baseline. The action saves its
versions and announces none of them. It also counts the phases that already
ended as sent, without posting them: a dependency added to the config does not
empty years of past dates into the channel.

A version still counting down is different. If its 12, 6 or 1 month warning has
come up, the action sends it on the first run, even though that run is the first
one. A version weeks away from the end of its support is the first thing the
channel needs to hear about a new dependency, not something to file away.

The action saves a version as seen only after Slack accepts the message that
names it. So a version is never saved as seen if its alert did not arrive. A dry
run never writes the file.

### Long digests

Slack takes a limited number of blocks in one message. If the digest does not
fit, the action splits it and posts every part. Nothing is cut. A part that
Slack rejects is not saved as sent, so the next run posts it again.

### Errors

If the action cannot read a product, it tries once more and then skips it. It
still sends the alerts for the other products. The run then exits with a non-zero
code and lists the products it could not read.

An end date the action cannot read works the same way. It skips that version,
still checks the versions around it, and exits with a non-zero code naming the
version and the date it saw. A version with no end date at all is not an error.
There is nothing to count down to yet, so the action passes over it in silence.

Both cases delay a version the alert it was due. The run fails so that the delay
is visible. Nothing is lost: an alert stays due until it is sent, so the next run
that can read the product sends it. The state file is still written, so the
alerts that did arrive are not sent again.

If there is nothing to report, the action sends no message at all.

## Run it locally

```console
$ go test ./...
$ go run ./cmd/notifier \
    --config ../.github/eol-notifier/dependencies.yaml \
    --state /tmp/eol-state.json \
    --dry-run
```

With `--dry-run` the action prints the message it would send and does not touch
the state file. Without it, set `EN_SLACK_WEBHOOK_URL` in the environment.
