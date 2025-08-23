import os
import re
import ast
from datetime import datetime

# ---- SETTINGS ----
ROOT_DIR = r"C:\Projects\ezHealthKonnect"
OUTPUT_DIR = "context"
IGNORE_DIRS = {
    ".git", "node_modules", "dist", "build", "out", "coverage", "__pycache__",
    ".next", ".turbo", "vendor", "bin", "obj", "target", ".venv", "env", "venv"
}

# ---------------------------
# 1) POSTGRES SCHEMA SUMMARY
# ---------------------------
def schema_summary(pg_conn_str: str) -> str:
    try:
        import psycopg2
    except Exception:
        return "# Schema Summary\n\n⚠️ psycopg2 not installed. Run: `pip install psycopg2`.\n"

    try:
        conn = psycopg2.connect(pg_conn_str)
        cur = conn.cursor()
        cur.execute("""
            SELECT table_name, column_name, data_type, is_nullable
            FROM information_schema.columns
            WHERE table_schema = 'public'
            ORDER BY table_name, ordinal_position;
        """)
        rows = cur.fetchall()

        # Try to get PKs / FKs
        cur.execute("""
            SELECT
                tc.table_name, kcu.column_name
            FROM information_schema.table_constraints tc
            JOIN information_schema.key_column_usage kcu
              ON tc.constraint_name = kcu.constraint_name
             AND tc.table_schema = kcu.table_schema
            WHERE tc.table_schema='public' AND tc.constraint_type='PRIMARY KEY'
            ORDER BY tc.table_name, kcu.ordinal_position;
        """)
        pk_map = {}
        for t, c in cur.fetchall():
            pk_map.setdefault(t, []).append(c)

        cur.execute("""
            SELECT
                tc.table_name, kcu.column_name, ccu.table_name AS foreign_table, ccu.column_name AS foreign_column
            FROM information_schema.table_constraints AS tc
            JOIN information_schema.key_column_usage AS kcu
              ON tc.constraint_name = kcu.constraint_name
             AND tc.table_schema = kcu.table_schema
            JOIN information_schema.constraint_column_usage AS ccu
              ON ccu.constraint_name = tc.constraint_name
             AND ccu.table_schema = tc.table_schema
            WHERE tc.table_schema='public' AND tc.constraint_type = 'FOREIGN KEY'
            ORDER BY tc.table_name, kcu.column_name;
        """)
        fk_map = {}
        for t, c, ft, fc in cur.fetchall():
            fk_map.setdefault(t, []).append((c, ft, fc))

        conn.close()

        tables = {}
        for table, col, dtype, nullable in rows:
            tables.setdefault(table, []).append(
                f"- {col} ({dtype}{' NULL' if nullable=='YES' else ' NOT NULL'})"
            )

        lines = [f"# Schema Summary (Generated {datetime.utcnow().isoformat()})\n"]
        for t in sorted(tables.keys()):
            lines.append(f"## {t}")
            if t in pk_map:
                lines.append(f"- **PK**: {', '.join(pk_map[t])}")
            if t in fk_map:
                for (c, ft, fc) in fk_map[t]:
                    lines.append(f"- **FK**: {c} → {ft}.{fc}")
            lines.extend(tables[t])
            lines.append("")
        return "\n".join(lines) if tables else "# Schema Summary\n\n_No tables found in `public` schema._\n"

    except Exception as e:
        return f"# Schema Summary\n\n⚠️ Failed to connect or query Postgres: `{e}`\n"


# -----------------------------------
# 2) CODE SUMMARY (PY, JS/TS, and GO)
# -----------------------------------
def should_skip_dir(d: str) -> bool:
    name = os.path.basename(d)
    return name in IGNORE_DIRS or name.startswith(".")

def iter_files(root: str, exts: tuple[str, ...]):
    for dirpath, dirnames, files in os.walk(root):
        # prune ignored dirs
        dirnames[:] = [d for d in dirnames if not should_skip_dir(os.path.join(dirpath, d))]
        for f in files:
            if f.endswith(exts):
                yield os.path.join(dirpath, f)

# --- Python ---
def summarize_python(root: str):
    file_count = func_count = class_count = 0
    sections = []
    for fp in iter_files(root, (".py",)):
        file_count += 1
        try:
            with open(fp, "r", encoding="utf-8", errors="ignore") as src:
                source = src.read()
            tree = ast.parse(source, filename=os.path.basename(fp))
        except Exception as e:
            sections.append(f"### {fp}\n- ⚠️ Skipped (parse error: {e})\n")
            continue

        funcs, classes = [], []
        for node in ast.iter_child_nodes(tree):  # top-level only
            if isinstance(node, ast.FunctionDef):
                args = [a.arg for a in node.args.args]
                funcs.append(f"- Function `{node.name}`(args: {args})")
                func_count += 1
            elif isinstance(node, ast.ClassDef):
                methods = [n.name for n in node.body if isinstance(n, ast.FunctionDef)]
                classes.append(f"- Class `{node.name}` (methods: {methods})")
                class_count += 1

        if funcs or classes:
            sections.append(f"### {fp}")
            sections.extend(funcs)
            sections.extend(classes)
            sections.append("")
    header = f"## Python\n_Scanned {file_count} files → {func_count} functions, {class_count} classes._\n"
    body = "\n".join(sections) if sections else "_No Python code found_\n"
    return header + "\n" + body

# --- Node.js (JS/TS) ---
JS_FUNC = re.compile(
    r"(?:^|\s)function\s+([A-Za-z_$][\w$]*)\s*\(|"                       # function foo(
    r"(?:^|\s)(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*function\s*\(|"  # const foo = function(
    r"(?:^|\s)(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*\(.*?\)\s*=>|"   # const foo = (...) =>
    r"(?:^|\s)export\s+(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(",    # export function foo(
    re.MULTILINE
)
JS_CLASS = re.compile(r"(?:^|\s)class\s+([A-Za-z_$][\w$]*)\b|export\s+default\s+class\s+([A-Za-z_$][\w$]*)", re.MULTILINE)

def summarize_node(root: str):
    file_count = func_count = class_count = 0
    sections = []
    for fp in iter_files(root, (".js", ".ts", ".mjs", ".cjs")):
        # Skip TypeScript declaration files
        if fp.endswith(".d.ts"):
            continue
        file_count += 1
        try:
            with open(fp, "r", encoding="utf-8", errors="ignore") as src:
                content = src.read()
        except Exception as e:
            sections.append(f"### {fp}\n- ⚠️ Skipped (read error: {e})\n")
            continue

        funcs = [g for m in JS_FUNC.finditer(content) for g in m.groups() if g]
        classes = [g for m in JS_CLASS.finditer(content) for g in m.groups() if g]

        func_count += len(funcs)
        class_count += len(classes)

        if funcs or classes:
            sections.append(f"### {fp}")
            for fn in funcs:
                sections.append(f"- Function `{fn}`")
            for cl in classes:
                sections.append(f"- Class `{cl}`")
            # Quick Express/Router route hint (optional, lightweight)
            if "router." in content or "app." in content:
                routes = []
                for http in ("get", "post", "put", "patch", "delete"):
                    r = re.findall(rf"\b(?:router|app)\.{http}\s*\(\s*['\"]([^'\"]+)['\"]", content)
                    for path in r[:5]:  # cap per file
                        routes.append(f"- Route {http.upper()} {path}")
                if routes:
                    sections.append("_Routes:_")
                    sections.extend(routes)
            sections.append("")
    header = f"## Node.js (JS/TS)\n_Scanned {file_count} files → {func_count} functions, {class_count} classes._\n"
    body = "\n".join(sections) if sections else "_No Node.js code found_\n"
    return header + "\n" + body

# --- Go ---
GO_FUNC = re.compile(r"func\s+(?:\([^)]+\)\s*)?([A-Za-z_]\w*)\s*\(")
GO_TYPE = re.compile(r"type\s+([A-Za-z_]\w+)\s+(struct|interface)\b")

def summarize_go(root: str):
    file_count = func_count = type_count = 0
    sections = []
    for fp in iter_files(root, (".go",)):
        # Skip generated files
        base = os.path.basename(fp).lower()
        if base.endswith("_gen.go") or base.endswith("_mock.go"):
            continue
        file_count += 1
        try:
            with open(fp, "r", encoding="utf-8", errors="ignore") as src:
                content = src.read()
        except Exception as e:
            sections.append(f"### {fp}\n- ⚠️ Skipped (read error: {e})\n")
            continue

        funcs = GO_FUNC.findall(content)
        types = GO_TYPE.findall(content)

        func_count += len(funcs)
        type_count += len(types)

        if funcs or types:
            sections.append(f"### {fp}")
            for fn in funcs:
                sections.append(f"- Function/Method `{fn}`")
            for (t, kind) in types:
                sections.append(f"- {kind.title()} `{t}`")
            sections.append("")
    header = f"## Go\n_Scanned {file_count} files → {func_count} functions/methods, {type_count} types._\n"
    body = "\n".join(sections) if sections else "_No Go code found_\n"
    return header + "\n" + body

# ---------------------------
# SAVE HELPERS
# ---------------------------
def save_text(filename: str, text: str):
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    path = os.path.join(OUTPUT_DIR, filename)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    print(f"✅ Saved {path}")

# ---------------------------
# MAIN
# ---------------------------
if __name__ == "__main__":
    # 1) SCHEMA SUMMARY
    # Edit this:
    PG_CONN_STR = "dbname=ezhealthkonnect user=ezhealth_user password=secure_password_change_me host=localhost port=5432"
    schema_text = schema_summary(PG_CONN_STR)
    save_text("schema_summary.md", schema_text)

    # 2) CODE SUMMARY (PY + JS/TS + GO)
    code_parts = [
        f"# Code Summary (Generated {datetime.utcnow().isoformat()})\n",
        summarize_python(ROOT_DIR),
        "",
        summarize_node(ROOT_DIR),
        "",
        summarize_go(ROOT_DIR),
    ]
    save_text("code_summary.md", "\n".join(code_parts))
