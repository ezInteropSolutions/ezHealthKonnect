#!/usr/bin/env python3
"""
build_fhir_terminology.py

Build ezHealthKonnect gzip-compressed ValueSet files from official FHIR package
tarballs. This replaces the earlier R4-only tx.fhir.org script with a
version-aware package-based builder.

Example:

  python cmd/build-fhir-terminology/build_fhir_terminology.py ^
    --version R5 ^
    --schema-dir schemas/fhir ^
    --core-package downloads/hl7.fhir.r5.core-5.0.0.tgz ^
    --expansions-package downloads/hl7.fhir.r5.expansions-5.0.0.tgz
"""

from __future__ import annotations

import argparse
import gzip
import json
import re
import tarfile
from pathlib import Path


EXTERNAL_SYSTEMS = {
    "http://snomed.info/sct": "SNOMED CT",
    "http://loinc.org": "LOINC",
    "http://www.nlm.nih.gov/research/umls/rxnorm": "RxNorm",
    "http://www.ama-assn.org/go/cpt": "CPT",
    "http://www.whocc.no/": "ATC",
    "http://hl7.org/fhir/ndfrt": "NDF-RT",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build ezHealthKonnect FHIR terminology .gz files from package tarballs."
    )
    parser.add_argument("--version", required=True, help="FHIR version label, e.g. R4 or R5")
    parser.add_argument("--schema-dir", default="schemas/fhir", help="Base schema directory")
    parser.add_argument("--core-package", required=True, help="Core FHIR package .tgz")
    parser.add_argument("--expansions-package", required=True, help="FHIR expansions package .tgz")
    return parser.parse_args()


def normalize_url(url: str) -> str:
    return re.sub(r"\|.*$", "", url or "")


def valueset_file_id(url: str) -> str:
    url = normalize_url(url)
    if "/ValueSet/" in url:
        return url.split("/ValueSet/")[-1]
    slug = re.sub(r"[^a-zA-Z0-9\\-]", "-", url.split("//")[-1])
    return slug[:120]


def iter_package_json(tgz_path: Path):
    with tarfile.open(tgz_path, "r:gz") as tar:
        for member in tar.getmembers():
            if not member.isfile():
                continue
            if not member.name.startswith("package/"):
                continue
            if not member.name.endswith(".json"):
                continue
            extracted = tar.extractfile(member)
            if extracted is None:
                continue
            try:
                doc = json.loads(extracted.read().decode("utf-8"))
            except Exception:
                continue
            yield doc


def collect_valueset_urls(version_dir: Path) -> list[str]:
    urls: set[str] = set()
    scan_dirs = [
        version_dir / "resources",
        version_dir / "profiles",
    ]

    files = []
    for scan_dir in scan_dirs:
        if not scan_dir.exists():
            continue
        files.extend(scan_dir.rglob("*.gz"))

    for path in files:
        with gzip.open(path, "rt", encoding="utf-8") as handle:
            schema = json.load(handle)
        for element in (schema.get("elements") or {}).values():
            value_set = normalize_url(element.get("valueSet", ""))
            if value_set:
                urls.add(value_set)

    return sorted(urls)


def flatten_contains(entries: list[dict], codes: set[str], systems: set[str]) -> None:
    for entry in entries or []:
        code = entry.get("code")
        system = entry.get("system", "")
        if code:
            codes.add(str(code))
            for prefix, label in EXTERNAL_SYSTEMS.items():
                if system.startswith(prefix):
                    systems.add(label)
                    break
        flatten_contains(entry.get("contains") or [], codes, systems)


def explicit_concepts_from_compose(compose: dict) -> tuple[list[str], list[str]]:
    codes: set[str] = set()
    systems: set[str] = set()
    for include in compose.get("include") or []:
        system = include.get("system", "")
        concepts = include.get("concept") or []
        for concept in concepts:
            code = concept.get("code")
            if code:
                codes.add(str(code))
                for prefix, label in EXTERNAL_SYSTEMS.items():
                    if system.startswith(prefix):
                        systems.add(label)
                        break
    return sorted(codes), sorted(systems)


def build_expansion_index(expansions_package: Path) -> dict[str, dict]:
    index: dict[str, dict] = {}
    for doc in iter_package_json(expansions_package):
        if doc.get("resourceType") != "ValueSet":
            continue
        url = normalize_url(doc.get("url", ""))
        if not url:
            continue
        codes: set[str] = set()
        systems: set[str] = set()
        expansion = doc.get("expansion") or {}
        flatten_contains(expansion.get("contains") or [], codes, systems)
        index[url] = {
            "name": doc.get("name") or doc.get("id") or valueset_file_id(url),
            "codes": sorted(codes),
            "externalSystems": sorted(systems),
            "fetchFailed": False,
        }
    return index


def build_core_index(core_package: Path) -> dict[str, dict]:
    index: dict[str, dict] = {}
    for doc in iter_package_json(core_package):
        if doc.get("resourceType") != "ValueSet":
            continue
        url = normalize_url(doc.get("url", ""))
        if not url:
            continue
        codes, systems = explicit_concepts_from_compose(doc.get("compose") or {})
        index[url] = {
            "name": doc.get("name") or doc.get("id") or valueset_file_id(url),
            "codes": codes,
            "externalSystems": systems,
            "fetchFailed": False,
        }
    return index


def write_record(out_dir: Path, url: str, record: dict) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / f"{valueset_file_id(url)}.gz"
    payload = {
        "url": url,
        "name": record["name"],
        "codes": record["codes"],
        "caseInsensitive": False,
        "externalSystems": record["externalSystems"],
        "fetchFailed": record["fetchFailed"],
    }
    with gzip.open(out_path, "wt", encoding="utf-8") as handle:
        json.dump(payload, handle, separators=(",", ":"))


def main() -> int:
    args = parse_args()
    version = args.version.upper()
    version_dir = Path(args.schema_dir) / version
    out_dir = version_dir / "valuesets"

    if not version_dir.exists():
        raise SystemExit(f"Schema version directory not found: {version_dir}")

    urls = collect_valueset_urls(version_dir)
    exp_index = build_expansion_index(Path(args.expansions_package))
    core_index = build_core_index(Path(args.core_package))

    complete = 0
    stubbed = 0

    for url in urls:
        record = exp_index.get(url) or core_index.get(url)
        if record is None:
            record = {
                "name": valueset_file_id(url),
                "codes": [],
                "externalSystems": [],
                "fetchFailed": True,
            }
            stubbed += 1
        else:
            complete += 1

        write_record(out_dir, url, record)

    print(f"[valuesets] scanned {len(urls)} referenced ValueSet URLs from {version_dir}")
    print(f"[valuesets] wrote {complete} expanded/explicit ValueSets to {out_dir}")
    print(f"[valuesets] wrote {stubbed} stub ValueSets to {out_dir}")
    print("Done.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
