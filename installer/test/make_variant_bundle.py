#!/usr/bin/env python3
"""Make an upgrade-test bundle from a real application bundle.

Copies the bundle zip, sets a new version in bundle-manifest.json and adds
files (e.g. an extra migration), so installer end-to-end tests can exercise
upgrade, automatic rollback and manual rollback with real binaries.

usage: make_variant_bundle.py SRC.zip DST.zip VERSION [--add PATH CONTENT]...
"""
import json
import sys
import zipfile


def main(argv):
    if len(argv) < 4:
        print(__doc__)
        return 2
    src, dst, version = argv[1], argv[2], argv[3]
    adds = []
    rest = argv[4:]
    while rest:
        if rest[0] != "--add" or len(rest) < 3:
            print(__doc__)
            return 2
        adds.append((rest[1], rest[2]))
        rest = rest[3:]

    with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
        root = zin.namelist()[0].split("/")[0]
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == root + "/bundle-manifest.json":
                info = json.loads(data)
                info["version"] = version
                data = json.dumps(info, indent=2).encode()
            zout.writestr(item, data)
        for path, content in adds:
            zi = zipfile.ZipInfo(root + "/" + path)
            zi.external_attr = 0o644 << 16
            zi.compress_type = zipfile.ZIP_DEFLATED
            zout.writestr(zi, content)
    print(f"{dst}: version {version}, +{len(adds)} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
