"""Resolve coordinated sources once and freeze existing CI images by digest."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import time
from urllib.parse import quote

REPOSITORIES = {"gateway": "tyk", "dashboard": "tyk-analytics", "pump": "tyk-pump", "sink": "tyk-sink"}
IMAGE_REPOSITORIES = {"gateway": "tyk-ee", "dashboard": "tyk-analytics", "pump": "tyk-pump",
                      "sink": "tyk-sink", "mock": "tyk-mock-mcp-server"}
MOCK_SHA = "29fefa17ad1a8ce168a66ccc7680030d46cfbb13"


def commit_sha(value):
    if not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{40}", value):
        raise ValueError("Component revision must be an immutable commit SHA")
    return value


def github_commit(repository, ref, optional=False):
    result = subprocess.run(["gh", "api", f"repos/TykTechnologies/{repository}/commits/{quote(ref, safe='')}",
                             "--jq", ".sha"], text=True, capture_output=True)
    if result.returncode:
        if optional and "HTTP 404" in result.stderr:
            return None
        raise RuntimeError(f"Cannot resolve source revision for {repository}")
    return commit_sha(result.stdout.strip())


def resolve(caller, caller_sha, branch, base, topology, lookup=github_commit):
    if topology not in ("direct", "mdcb") or caller not in {"TykTechnologies/" + repo for repo in REPOSITORIES.values()}:
        raise ValueError("Unsupported MCP caller or topology")
    components = ["gateway", "dashboard", "pump"] + (["sink"] if topology == "mdcb" else [])
    shas = {}
    for component in components:
        repository = REPOSITORIES[component]
        revision = caller_sha if caller == "TykTechnologies/" + repository else (
            lookup(repository, branch, optional=True) or lookup(repository, base))
        shas[component] = commit_sha(revision)
    shas["mock"] = MOCK_SHA
    environment = lookup("tyk-pro", branch, optional=True) or lookup("tyk-pro", "main")
    return {"schema_version": 1, "topology": topology, "tested_shas": shas,
            "environment_sha": commit_sha(environment)}


def image_digest(repository, revision):
    # BatchGetImage is part of the existing CI pull/push role. DescribeImages
    # requires additional registry permissions that ordinary API jobs do not use.
    result = subprocess.run(["aws", "ecr", "batch-get-image", "--repository-name", repository,
                             "--image-ids", "imageTag=sha-" + revision,
                             "--output", "json"],
                            text=True, capture_output=True)
    if result.returncode:
        # Report only AWS's error code, never raw CLI output or credentials.
        error = re.search(r"\(([A-Za-z0-9]+)\)", result.stderr)
        code = error.group(1) if error else "RegistryRequestFailed"
        raise RuntimeError(f"Cannot read immutable CI image for {repository}: {code}")
    response = json.loads(result.stdout)
    failures = response.get("failures", [])
    found = response.get("images", [])
    if not found and len(failures) == 1 and failures[0].get("failureCode") == "ImageNotFound":
        return None
    if failures or len(found) != 1:
        raise RuntimeError(f"Registry did not return one immutable CI image for {repository}")
    digest = found[0]["imageId"]["imageDigest"]
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        raise ValueError(f"Invalid CI image digest for {repository}")
    return digest


def freeze(manifest, registry, lookup=image_digest, timeout=1200, sleep=time.sleep, clock=time.monotonic):
    if not re.fullmatch(r"[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com", registry):
        raise ValueError("Invalid CI image registry")
    images = {}
    deadline = clock() + timeout
    while len(images) < len(manifest["tested_shas"]):
        for component, revision in manifest["tested_shas"].items():
            commit_sha(revision)
            if component in images:
                continue
            repository = IMAGE_REPOSITORIES[component]
            digest = lookup(repository, revision)
            if digest:
                if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
                    raise ValueError("Invalid CI image digest")
                images[component] = f"{registry}/{repository}@{digest}"
        if len(images) < len(manifest["tested_shas"]):
            if clock() >= deadline:
                missing = sorted(set(manifest["tested_shas"]) - set(images))
                raise RuntimeError("Coordinated PR images were not published: " + ", ".join(missing))
            sleep(10)
    return {**manifest, "images": images}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=("resolve", "mock", "freeze"))
    parser.add_argument("--manifest", type=Path, required=True)
    args = parser.parse_args()
    if args.phase == "mock":
        raise SystemExit(0 if image_digest(IMAGE_REPOSITORIES["mock"], MOCK_SHA) else 10)
    if args.phase == "resolve":
        manifest = resolve(os.environ["CALLER_REPOSITORY"], os.environ["CALLER_SHA"],
                           os.environ["SOURCE_BRANCH"], os.environ["BASE_BRANCH"], os.environ["MCP_TOPOLOGY"])
        args.manifest.write_text(json.dumps(manifest) + "\n")
    else:
        manifest = freeze(json.loads(args.manifest.read_text()), os.environ["ECR"])
        args.manifest.write_text(json.dumps(manifest, indent=2) + "\n")
        with Path(os.environ["GITHUB_OUTPUT"]).open("a") as output:
            output.write("manifest=" + json.dumps(manifest, separators=(",", ":")) + "\n")


if __name__ == "__main__":
    main()
