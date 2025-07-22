import os
import json
import hashlib
import shutil
from datetime import datetime
from pathlib import Path

# Configuration
CONFIG = {
    "project_root": ".",
    "exclude_dirs": ["node_modules", "vendor", ".git", "dist", "__pycache__"],
    "include_extensions": [".go", ".js", ".ts", ".sql", ".graphql"],
    "output_dir": "mcp_output",
    "archive_dir": "mcp_output/archives"
}

# Ensure output directories exist
output_path = Path(CONFIG["output_dir"])
archive_path = Path(CONFIG["archive_dir"])
output_path.mkdir(exist_ok=True)
archive_path.mkdir(parents=True, exist_ok=True)

def is_valid_file(file_path):
    return any(file_path.endswith(ext) for ext in CONFIG["include_extensions"])

def is_excluded(path):
    return any(excluded in path.parts for excluded in CONFIG["exclude_dirs"])

def hash_file(filepath):
    hasher = hashlib.sha256()
    with open(filepath, 'rb') as f:
        hasher.update(f.read())
    return hasher.hexdigest()

def extract_todos(filepath):
    todos = []
    try:
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            for i, line in enumerate(f):
                if 'TODO' in line or 'FIXME' in line:
                    todos.append({"line": i + 1, "note": line.strip()})
    except Exception:
        pass
    return todos

def summarize_file(filepath):
    summary = {
        "path": str(filepath),
        "last_modified": datetime.fromtimestamp(filepath.stat().st_mtime).isoformat(),
        "hash": hash_file(filepath),
        "functions": [],
        "todos": extract_todos(filepath)
    }
    try:
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            for line in f:
                stripped = line.strip()
                if stripped.startswith("func ") or "function " in stripped or stripped.startswith("def "):
                    summary["functions"].append(stripped[:100])
    except Exception as e:
        summary["error"] = str(e)
    return summary

def archive_existing_files():
    timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
    for fname in ["code_index.json", "changed_files.json", "checkpoints.json"]:
        src = output_path / fname
        if src.exists():
            dst = archive_path / f"{fname.replace('.json', '')}_{timestamp}.json"
            shutil.move(str(src), dst)

def find_latest_archive():
    candidates = list(archive_path.glob("changed_files_*.json"))
    return max(candidates, key=os.path.getmtime) if candidates else None

def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)

def get_changed_files(current, previous):
    prev_hash_map = {entry["file"]: entry["hash"] for entry in previous}
    return [entry["file"] for entry in current if prev_hash_map.get(entry["file"]) != entry["hash"]]

def extract_summaries(index, changed_files):
    return [entry for entry in index if entry["path"] in changed_files]

def write_diff_summary_md(summaries):
    summary_file = output_path / "mcp_diff_summary.md"
    lines = ["# MCP Diff Summary", f"Generated: {datetime.now().isoformat()}", ""]
    for summary in summaries:
        lines.append(f"## `{summary['path']}`")
        lines.append(f"- Last modified: {summary['last_modified']}")
        lines.append(f"- Functions:")
        for fn in summary.get("functions", []):
            lines.append(f"  - `{fn}`")
        if summary.get("todos"):
            lines.append("- TODOs / FIXMEs:")
            for todo in summary["todos"]:
                lines.append(f"  - Line {todo['line']}: {todo['note']}")
        lines.append("")
    summary_file.write_text("\n".join(lines), encoding="utf-8")
    return summary_file

def write_checkpoints(summaries):
    checkpoints = []
    for summary in summaries:
        for todo in summary.get("todos", []):
            checkpoints.append({
                "timestamp": datetime.now().isoformat(),
                "file": summary["path"],
                "note": todo["note"],
                "line": todo["line"]
            })
    checkpoint_file = output_path / "mcp_diff_checkpoints.json"
    with open(checkpoint_file, "w", encoding="utf-8") as f:
        json.dump(checkpoints, f, indent=2)
    return checkpoint_file

def main():
    archive_existing_files()

    code_index = []
    changed_files = []
    checkpoints = []

    for root, dirs, files in os.walk(CONFIG["project_root"]):
        root_path = Path(root)
        if is_excluded(root_path):
            continue
        for file in files:
            file_path = root_path / file
            if is_valid_file(file):
                summary = summarize_file(file_path)
                code_index.append(summary)
                if summary.get("todos"):
                    for todo in summary["todos"]:
                        checkpoints.append({
                            "timestamp": datetime.now().isoformat(),
                            "file": str(file_path),
                            "note": todo["note"],
                            "line": todo["line"]
                        })
                changed_files.append({
                    "file": str(file_path),
                    "hash": summary["hash"]
                })

    (output_path / "code_index.json").write_text(json.dumps(code_index, indent=2), encoding="utf-8")
    (output_path / "checkpoints.json").write_text(json.dumps(checkpoints, indent=2), encoding="utf-8")
    (output_path / "changed_files.json").write_text(json.dumps(changed_files, indent=2), encoding="utf-8")

    latest_archive = find_latest_archive()
    if latest_archive:
        prev_changed = load_json(latest_archive)
        changed_paths = get_changed_files(changed_files, prev_changed)
        summaries = extract_summaries(code_index, changed_paths)
        summary_file = write_diff_summary_md(summaries)
        checkpoint_file = write_checkpoints(summaries)
        print(f"✅ Changed files: {len(changed_paths)}")
        print(f"📝 Summary: {summary_file}")
        print(f"📋 Checkpoints: {checkpoint_file}")
    else:
        print("ℹ️ No previous archive found — skipping diff.")

if __name__ == "__main__":
    main()
