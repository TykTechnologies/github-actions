## Release bot

```
name: Release bot

on:
  issue_comment:
    types: [created]

jobs:
  release_bot:
    uses: TykTechnologies/github-actions/.github/workflows/release-bot.yaml@main
```

Comment `/release to <branch>` on a merged pull request to open a cherry-pick PR against `<branch>`.

Only users with **write** (or maintain/admin) access to the repository can trigger the bot; commands from anyone else are ignored and logged as a notice in the workflow run.
