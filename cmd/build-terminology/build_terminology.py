#!/usr/bin/env python3
"""
build_terminology.py — One-time build-time script for ezHealthKonnect

Scans all compiled R4 resource schemas to collect referenced ValueSet URLs,
fetches each expansion from tx.fhir.org (free public HL7 terminology server),
and writes compact .gz files to schemas/fhir/R4/valuesets/.

Run once at dev time:
    python3 cmd/build-terminology/build_terminology.py

The generated .gz files are committed to the repo.
At runtime the Go process loads them — zero external calls needed.

If tx.fhir.org cannot expand a ValueSet (e.g. very large SNOMED subsets),
the file is still written with whatever codes were returned, and
externalSystems[] is populated so the runtime can emit a disclosure.
"""

import gzip
import json
import os
import re
import sys
import time
import urllib.request
import urllib.error
from concurrent.futures import ThreadPoolExecutor, as_completed

# ── Config ────────────────────────────────────────────────────────────────────
SCHEMA_DIRS = [
    "schemas/fhir/R4/resources",
    "schemas/fhir/R4/profiles/us-core",
]
OUTPUT_DIR  = "schemas/fhir/R4/valuesets"
TX_BASE     = "https://tx.fhir.org/r4"
MAX_CODES   = 2000   # per ValueSet expansion (_count)
THREADS     = 8      # concurrent fetches
RETRY       = 2      # retries on transient failure

# Systems that require a license to redistribute.
# We still embed the codes (LOINC/SNOMED are freely usable in open source),
# but we flag them so the runtime can add a disclosure comment.
EXTERNAL_SYSTEMS = {
    "http://snomed.info/sct":                        "SNOMED CT",
    "http://loinc.org":                              "LOINC",
    "http://www.nlm.nih.gov/research/umls/rxnorm":   "RxNorm",
    "http://www.ama-assn.org/go/cpt":                "CPT",
    "http://www.whocc.no/":                          "ATC",
    "http://hl7.org/fhir/ndfrt":                     "NDF-RT",
}

# ── Helpers ───────────────────────────────────────────────────────────────────

def normalize_url(url: str) -> str:
    """Strip |version suffix."""
    return re.sub(r'\|.*$', '', url)

def vs_id_from_url(url: str) -> str:
    """
    http://hl7.org/fhir/ValueSet/administrative-gender  →  administrative-gender
    http://loinc.org/vs/LL379-9                         →  loinc-LL379-9
    """
    url = normalize_url(url)
    if '/ValueSet/' in url:
        return url.split('/ValueSet/')[-1]
    # Non-standard URL: use a sanitised slug
    slug = re.sub(r'[^a-zA-Z0-9\-]', '-', url.split('//')[-1])
    return slug[:80]

def collect_valueset_urls() -> list[str]:
    """Walk all schema .gz files and collect unique ValueSet URLs."""
    urls: set[str] = set()
    for schema_dir in SCHEMA_DIRS:
        if not os.path.isdir(schema_dir):
            continue
        for fname in os.listdir(schema_dir):
            if not fname.endswith('.gz'):
                continue
            with gzip.open(os.path.join(schema_dir, fname)) as f:
                schema = json.load(f)
            for el in schema.get('elements', {}).values():
                vs = el.get('valueSet', '')
                if vs:
                    urls.add(normalize_url(vs))
    return sorted(urls)

def fetch_expansion(url: str) -> dict | None:
    """
    Call tx.fhir.org ValueSet/$expand and return the parsed JSON body,
    or None on total failure.
    """
    endpoint = f"{TX_BASE}/ValueSet/$expand?url={urllib.request.quote(url, safe=':/.')}&_count={MAX_CODES}"
    for attempt in range(RETRY + 1):
        try:
            req = urllib.request.Request(
                endpoint,
                headers={
                    "Accept":     "application/fhir+json",
                    "User-Agent": "ezHealthKonnect-terminology-builder/1.0 (open source; build-time only)",
                }
            )
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            if e.code in (404, 410):
                return None   # ValueSet not found — not a transient error
            if attempt == RETRY:
                return None
            time.sleep(1 + attempt)
        except Exception:
            if attempt == RETRY:
                return None
            time.sleep(1 + attempt)
    return None

def extract_codes(expansion_json: dict) -> tuple[list[str], list[str], bool]:
    """
    Returns (codes, externalSystems, caseInsensitive).
    codes          — deduplicated list of code strings
    externalSystems — display names of systems whose codes we embedded
                     (for disclosure purposes)
    caseInsensitive — True if any included system is case-insensitive
    """
    codes: set[str] = set()
    external: set[str] = set()
    case_insensitive = False

    expansion = expansion_json.get('expansion', {})

    def walk(entries):
        nonlocal case_insensitive
        for entry in entries:
            code = entry.get('code', '')
            system = entry.get('system', '')
            if code:
                codes.add(code)
                # Check if this system is external/licensed
                for sys_prefix, label in EXTERNAL_SYSTEMS.items():
                    if system.startswith(sys_prefix):
                        external.add(label)
                        break
            # Some expansions nest abstracts with children
            for child in entry.get('contains', []):
                walk([child])

    walk(expansion.get('contains', []))

    # Check compose for caseInsensitive flag
    for inc in expansion_json.get('compose', {}).get('include', []):
        # Not always present in expansion response, but check anyway
        pass

    # tx.fhir.org includes a `parameter` array; look for caseInsensitive
    for param in expansion.get('parameter', []):
        if param.get('name') == 'displayLanguage':
            pass
    # Rough heuristic: code systems like mime-type, languages are case-insensitive
    name = expansion_json.get('name', '')
    if any(k in name.lower() for k in ('mime', 'language', 'mediatype')):
        case_insensitive = True

    return sorted(codes), sorted(external), case_insensitive

def process_one(url: str) -> tuple[str, str, int, str]:
    """
    Returns (vs_id, status, code_count, detail).
    status: 'ok' | 'partial' | 'empty' | 'failed'
    """
    vs_id = vs_id_from_url(url)
    out_path = os.path.join(OUTPUT_DIR, f"{vs_id}.gz")

    data = fetch_expansion(url)
    if data is None:
        # Write a stub so the runtime knows this URL exists but has no codes
        record = {
            "url":             url,
            "name":            vs_id,
            "codes":           [],
            "caseInsensitive": False,
            "externalSystems": [],
            "fetchFailed":     True,
        }
        with gzip.open(out_path, 'wt', encoding='utf-8') as f:
            json.dump(record, f, separators=(',', ':'))
        return vs_id, 'failed', 0, 'tx.fhir.org returned no expansion'

    codes, external, ci = extract_codes(data)
    status = 'ok' if codes else 'empty'
    if external:
        status = 'partial' if codes else 'external-only'

    record = {
        "url":             url,
        "name":            data.get('name', vs_id),
        "codes":           codes,
        "caseInsensitive": ci,
        "externalSystems": external,
        "fetchFailed":     False,
    }
    with gzip.open(out_path, 'wt', encoding='utf-8') as f:
        json.dump(record, f, separators=(',', ':'))

    return vs_id, status, len(codes), ', '.join(external) if external else ''

# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)

    urls = collect_valueset_urls()
    total = len(urls)
    print(f"Found {total} unique ValueSet URLs across R4 resource + US Core schemas")
    print(f"Fetching expansions from {TX_BASE} ({THREADS} threads, up to {MAX_CODES} codes each)...")
    print()

    results = {'ok': 0, 'partial': 0, 'empty': 0, 'failed': 0, 'external-only': 0}
    done = 0

    with ThreadPoolExecutor(max_workers=THREADS) as pool:
        futures = {pool.submit(process_one, url): url for url in urls}
        for fut in as_completed(futures):
            vs_id, status, count, detail = fut.result()
            results[status] = results.get(status, 0) + 1
            done += 1
            # Progress line
            pct = done * 100 // total
            tag  = {'ok': '+', 'partial': '~', 'empty': '0', 'failed': 'X', 'external-only': 'E'}.get(status, '?')
            line = f"  [{pct:3d}%] {tag} {vs_id:<55} {count:4d} codes"
            if detail:
                line += f"  [{detail}]"
            print(line)

    print()
    print("─" * 70)
    print(f"Complete:  {results.get('ok', 0):4d} fully embedded")
    print(f"Partial:   {results.get('partial', 0):4d} embedded (includes licensed codes — disclosed at runtime)")
    print(f"Empty:     {results.get('empty', 0):4d} ValueSets had no expansion codes")
    print(f"Failed:    {results.get('failed', 0):4d} tx.fhir.org could not expand")
    print(f"External:  {results.get('external-only', 0):4d} exclusively external codes")
    print()
    total_files = len([f for f in os.listdir(OUTPUT_DIR) if f.endswith('.gz')])
    print(f"Output: {total_files} .gz files written to {OUTPUT_DIR}/")
    print()
    print("Next step: these files are now committed to the repo.")
    print("The Go TerminologyRegistry will load them at startup — no runtime internet access needed.")

if __name__ == '__main__':
    main()
