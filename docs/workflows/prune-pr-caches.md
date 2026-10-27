## Prune PR caches

Scheduled cleanup (every 30 minutes) of GitHub Actions caches that belong to pull-request
refs (`refs/pull/<N>/merge`) in the listed Tyk repositories. PR builds can leave several GB
of cache per push (BuildKit blobs, setup-go) and push the repository over its 10 GB limit, so
GitHub evicts the useful entries on master.

Rules (see `.github/scripts/prune-pr-caches.sh`):

* only PR refs are touched; master, release branches and tags never are
* closed or merged PR: all its caches are deleted
* open PR: the newest entries are kept up to a per-PR budget (default 1024 MB), entries
  younger than 30 minutes are always kept, the rest is deleted
* PR state unknown: nothing is deleted

Safe by default: scheduled runs only report what they would delete (see the job summary).
Set the repository variable `PR_CACHE_PRUNE_DELETE` to `true` to let scheduled runs delete.
Manual runs (`workflow_dispatch`) take `repos`, `budget_mb`, `guard_min` and `dry_run`
(default `true`).

Requires the GitHub App behind `PROBE_APP_ID` / `PROBE_APP_PRIVATE_KEY` to have
"Actions: read and write" and "Pull requests: read" on the target repositories.

Adoption: Gateway, Dashboard, Pump, Sink, TIB, Portal.
