#!/usr/bin/env python3
"""
build_fhir_schemas.py

Build ezHealthKonnect gzip-compressed schema files from official FHIR NPM
package tarballs (.tgz). This recreates the on-disk JSON shape consumed by:

- fhir/schema_loader.go
- fhir/r4/registry.go

Examples:

  python cmd/build-fhir-schemas/build_fhir_schemas.py ^
    --version R5 ^
    --core-package downloads/hl7.fhir.r5.core-5.0.0.tgz ^
    --out-dir schemas/fhir

  python cmd/build-fhir-schemas/build_fhir_schemas.py ^
    --version R4 ^
    --core-package downloads/hl7.fhir.r4.core-4.0.1.tgz ^
    --profile-package downloads/hl7.fhir.us.core-8.0.1.tgz ^
    --profile-name us-core ^
    --out-dir schemas/fhir
"""

from __future__ import annotations

import argparse
import gzip
import io
import json
import os
import tarfile
from pathlib import Path
from typing import Iterable


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build ezHealthKonnect FHIR .gz schema files from FHIR package .tgz files."
    )
    parser.add_argument("--version", required=True, help="FHIR version label, e.g. R4 or R5")
    parser.add_argument(
        "--core-package",
        required=True,
        help="Path to the official core FHIR package .tgz, e.g. hl7.fhir.r5.core#5.0.0.tgz",
    )
    parser.add_argument(
        "--profile-package",
        action="append",
        default=[],
        help="Optional implementation guide/profile package .tgz. Can be repeated.",
    )
    parser.add_argument(
        "--profile-name",
        action="append",
        default=[],
        help="Output profile folder name corresponding to each --profile-package. Can be repeated.",
    )
    parser.add_argument(
        "--out-dir",
        default="schemas/fhir",
        help="Base output directory. Version folders are created underneath this path.",
    )
    return parser.parse_args()


def iter_package_json(tgz_path: Path) -> Iterable[tuple[str, dict]]:
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
                raw = extracted.read()
                doc = json.loads(raw.decode("utf-8"))
            except Exception:
                continue

            yield member.name, doc


def normalize_type_name(type_code: str) -> str:
    if not type_code:
        return ""
    if "://" in type_code:
        return type_code.rstrip("/").rsplit("/", 1)[-1]
    if "#" in type_code:
        return type_code.rsplit("#", 1)[-1]
    return type_code


def get_element_type(element: dict) -> str:
    types = element.get("type") or []
    if not types:
        return ""
    first = types[0] or {}
    return normalize_type_name(first.get("code", ""))


def get_element_constraints(element: dict) -> list[str]:
    constraints = []
    for item in element.get("constraint") or []:
        text = item.get("human") or item.get("expression") or item.get("key")
        if text:
            constraints.append(text)
    return constraints


def get_element_binding(element: dict) -> tuple[str, str]:
    binding = element.get("binding") or {}
    value_set = binding.get("valueSet") or binding.get("valueSetCanonical") or ""
    strength = binding.get("strength") or ""
    return value_set, strength


def normalize_description(sd: dict, element: dict) -> str:
    return (
        element.get("definition")
        or element.get("short")
        or sd.get("description")
        or ""
    )


def make_schema(sd: dict, version: str, include_constraints: bool) -> dict | None:
    if sd.get("resourceType") != "StructureDefinition":
        return None

    kind = sd.get("kind")
    derivation = sd.get("derivation")
    resource_type = sd.get("type") or sd.get("name")
    abstract = bool(sd.get("abstract"))

    # Keep only actual resource profiles/definitions, not datatypes/extensions.
    if kind != "resource":
        return None
    if not resource_type or resource_type == "Extension":
        return None
    if abstract:
        return None
    if derivation not in ("specialization", "constraint"):
        return None
    if not include_constraints and derivation != "specialization":
        return None

    snapshot = sd.get("snapshot") or {}
    elements = snapshot.get("element") or []
    if not elements:
        return None

    root = elements[0]
    output_elements: dict[str, dict] = {}
    required: list[str] = []
    must_support: list[str] = []

    for element in elements[1:]:
        path = element.get("path")
        if not path or not path.startswith(f"{resource_type}."):
            continue

        min_card = int(element.get("min", 0) or 0)
        max_card = str(element.get("max", "*") or "*")
        cardinality = f"{min_card}..{max_card}"
        data_type = get_element_type(element)
        value_set, binding_strength = get_element_binding(element)

        schema_element = {
            "path": path,
            "name": element.get("short") or path.split(".")[-1],
            "description": normalize_description(sd, element),
            "dataType": data_type,
            "cardinality": cardinality,
            "required": min_card >= 1,
            "mustSupport": bool(element.get("mustSupport")),
            "isModifier": bool(element.get("isModifier")),
            "isSummary": bool(element.get("isSummary")),
            "constraints": get_element_constraints(element),
        }

        if value_set:
            schema_element["valueSet"] = value_set
        if binding_strength:
            schema_element["bindingStrength"] = binding_strength

        output_elements[path] = schema_element

        if min_card >= 1:
            required.append(path)
        if element.get("mustSupport"):
            must_support.append(path)

    base_resource = ""
    base_definition = sd.get("baseDefinition") or ""
    if "/StructureDefinition/" in base_definition:
        base_resource = base_definition.rsplit("/", 1)[-1]
    elif derivation == "specialization":
        base_resource = root.get("base", {}).get("path", "")

    result = {
        "resourceType": resource_type,
        "version": version,
        "name": sd.get("name") or resource_type,
        "title": sd.get("title", ""),
        "description": sd.get("description", ""),
        "baseResource": base_resource,
        "profile": sd.get("url", ""),
        "elements": output_elements,
        "required": sorted(set(required)),
        "mustSupport": sorted(set(must_support)),
        "sourceFile": sd.get("url") or sd.get("id") or resource_type,
    }
    return result


def write_schema_file(out_dir: Path, resource_type: str, schema: dict) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / f"{resource_type}.gz"
    with gzip.open(out_path, "wt", encoding="utf-8") as handle:
        json.dump(schema, handle, separators=(",", ":"))


def build_from_package(
    tgz_path: Path,
    version: str,
    out_dir: Path,
    include_constraints: bool,
) -> tuple[int, int]:
    written = 0
    skipped = 0

    for _, doc in iter_package_json(tgz_path):
        schema = make_schema(doc, version, include_constraints)
        if schema is None:
            skipped += 1
            continue

        write_schema_file(out_dir, schema["resourceType"], schema)
        written += 1

    return written, skipped


def main() -> int:
    args = parse_args()

    if len(args.profile_package) != len(args.profile_name):
        raise SystemExit("--profile-package and --profile-name must be provided the same number of times")

    version = args.version.upper()
    base_out = Path(args.out_dir)

    core_package = Path(args.core_package)
    if not core_package.exists():
        raise SystemExit(f"Core package not found: {core_package}")

    core_out = base_out / version / "resources"
    written, skipped = build_from_package(
        core_package,
        version,
        core_out,
        include_constraints=False,
    )
    print(f"[core] wrote {written} schemas to {core_out} ({skipped} package files skipped)")

    for pkg, profile_name in zip(args.profile_package, args.profile_name):
        pkg_path = Path(pkg)
        if not pkg_path.exists():
            raise SystemExit(f"Profile package not found: {pkg_path}")

        profile_out = base_out / version / "profiles" / profile_name
        p_written, p_skipped = build_from_package(
            pkg_path,
            version,
            profile_out,
            include_constraints=True,
        )
        print(
            f"[profile:{profile_name}] wrote {p_written} schemas to {profile_out} "
            f"({p_skipped} package files skipped)"
        )

    print("Done.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
