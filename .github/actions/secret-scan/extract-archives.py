#!/usr/bin/env python3
"""Copy the archive files out of an exported container filesystem.

Only regular files whose names end in an archive extension are written, under
DEST, with the same relative path. Links, devices, absolute names and names
with a ".." component are skipped, so nothing in the image can write outside
DEST or point at the runner's own files.

Usage: extract-archives.py <export.tar> <dest>
Prints the number of archives written.
"""
import os
import re
import shutil
import sys
import tarfile

ARCHIVE = re.compile(
    r"\.(tar|tgz|gz|zip|tbz|tbz2|txz|jar|war|ear|whl|egg|bz2|xz|zst|7z|rar)$",
    re.IGNORECASE,
)


def safe_relpath(name):
    parts = [p for p in name.split("/") if p not in ("", ".")]
    if not parts or name.startswith("/") or ".." in parts:
        return None
    return os.path.join(*parts)


def main():
    if len(sys.argv) != 3:
        sys.stderr.write(__doc__)
        return 2
    src, dest = sys.argv[1], os.path.realpath(sys.argv[2])
    count = 0
    with tarfile.open(src, "r:") as tar:
        for member in tar:
            if not member.isreg() or not ARCHIVE.search(member.name):
                continue
            rel = safe_relpath(member.name)
            if rel is None:
                continue
            target = os.path.realpath(os.path.join(dest, rel))
            if not target.startswith(dest + os.sep):
                continue
            os.makedirs(os.path.dirname(target), exist_ok=True)
            with tar.extractfile(member) as fin, open(target, "wb") as fout:
                shutil.copyfileobj(fin, fout)
            count += 1
    print(count)
    return 0


if __name__ == "__main__":
    sys.exit(main())
