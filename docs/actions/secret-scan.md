## Secret scan

Scans for secrets before an image is published, and fails the job on a finding.

- **Build folder** (`path`): gitleaks with its default rules, including inside archives, plus a check for credentials in any copied `.git/config` (gitleaks skips `.git`).
- **Image** (`image`): for every platform of a multi-platform image,
  - trufflehog on every layer, the image config and the build history. This catches secrets deleted in a later layer, passed as build args, set in `ENV` or `LABEL`, or compiled into a binary;
  - gitleaks on the archives in the final filesystem (`.tar.gz`, `.tgz`, `.zip`, `.jar`, `.whl`, ...), which trufflehog does not fully unpack. Only regular archive files are copied out of the image (never links, absolute or `..` paths), so image contents cannot write outside the scan folder. An archive larger than `max-archive-mb` (default 1024) fails the scan rather than being skipped. Needs docker and python3; turn off with `scan-archives: false`.

`path` and `image` must not start with `-`. Scanner errors always fail the job, even with `enforce: false`: trufflehog exits 0 when it cannot read an image, so the action checks its log and fails if nothing was scanned. Secret values are never printed; findings show the tool, rule, file, platform and layer.

Example usage, in a release job after the image is pushed to a private registry and before it is pushed anywhere public:

```yaml
- name: Scan for secrets
  uses: TykTechnologies/github-actions/.github/actions/secret-scan@production
  with:
    path: dist
    image: ${{ steps.ecr.outputs.registry }}/my-image:${{ github.sha }}
    gitleaks-config: .gitleaks.toml                # optional
    allowlist: .github/secret-scan-allowlist.json  # optional
```

`image` takes a registry reference (pulled with the job's docker login; every platform of a multi-platform tag is scanned), `docker://<name>` for an image in the local daemon, or `file://<path>` for a saved tarball.

False positives:

- Build folder: pass `gitleaks-config`, a `.gitleaks.toml` that starts with `[extend]` and `useDefault = true`, then adds `[[allowlists]]` paths. Nothing inside the scanned folder or the working directory changes the rules: the action always passes its own config, ignores `gitleaks:allow` comments and does not read `.gitleaksignore` from the working directory. gitleaks always applies a `.gitleaksignore` at the root of the scanned folder, so the action reports that file as a finding (`unreviewed-gitleaksignore`).
- Archives in the image: by default all gitleaks rules except `generic-api-key`, which matches identifiers in minified JavaScript bundles. A `gitleaks-config` replaces this.
- Image layers: a JSON list of `{"detector": "...", "path": "<regex>", "reason": "..."}`; a malformed list or regex fails the scan. A default entry for dpkg checksum files in base images is built in.

The `gitleaks-config` and `allowlist` files should be reviewed by AppSec (CODEOWNERS).

Reports are written to `$RUNNER_TEMP/secret-scan-results`. trufflehog's raw output there contains the secret values: never upload that folder as an artifact.

`tests/secret-scan` builds 16 images that leak fake credentials in common ways and 2 correctly built controls; `secret-scan-test.yml` runs them, the fail-closed cases and a real multi-platform release image on every change.

Source: [/.github/actions/secret-scan/action.yml](/.github/actions/secret-scan/action.yml)
