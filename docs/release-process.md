# Release process

We use a floating tag `@production` to release and distribute GitHub Actions from this repository.

All product repositories (such as `tyk`, `tyk-analytics`, etc.) must pin their GitHub Actions usage to the `@production` tag.

The `@production` tag is moved to the latest version of the `main` branch when changes need to be promoted.

The process:

1. Contributors (editing this repository is limited to the internal Tyk team) will create a Pull Request (PR) to `main`.
2. The `main` branch is protected and requires a Pull Request and review before merging.
3. When the PR is merged, changes are not yet used across all repositories; moving the tag is needed.
4. The contributor needs to ask on the Slack channel `#team-ext-non-functional` to move the `@production` tag.
5. Editing/moving this tag is limited to repository owners.
