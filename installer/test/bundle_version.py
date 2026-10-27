#!/usr/bin/env python3
"""Print the version of a real application bundle, optionally bumped.

The installer refuses to downgrade, so the upgrade end-to-end tests need versions that are genuinely NEWER than
the bundle they start from. That bundle's version is not fixed: pull requests build it as v0.0.1-ci, a release
builds it as the tag (v26.10). Hardcoded "next" versions worked for the first and were a downgrade for the second.

usage: bundle_version.py BUNDLE.zip [--bump N]

  v0.0.1-ci  --bump 1  ->  v0.0.2-ci
  v26.10     --bump 1  ->  v26.10.1
  v26.10     --bump 2  ->  v26.10.2
"""
import json
import re
import sys
import zipfile


def bump(version, n):
    """Return version with its last numeric part raised by n (a missing third part counts as 0)."""
    m = re.match(r"^(v?)(\d+(?:\.\d+)*)(.*)$", version.strip())
    if not m:
        raise ValueError(f"cannot bump version {version!r}")
    prefix, core, suffix = m.groups()
    parts = [int(p) for p in core.split(".")]
    while len(parts) < 3:
        parts.append(0)
    parts[-1] += n
    return prefix + ".".join(str(p) for p in parts) + suffix


def bundle_version(path):
    with zipfile.ZipFile(path) as z:
        root = z.namelist()[0].split("/")[0]
        return json.loads(z.read(root + "/bundle-manifest.json"))["version"]


def main(argv):
    if len(argv) not in (2, 4) or (len(argv) == 4 and argv[2] != "--bump"):
        print(__doc__)
        return 2
    version = bundle_version(argv[1])
    print(bump(version, int(argv[3])) if len(argv) == 4 else version)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
