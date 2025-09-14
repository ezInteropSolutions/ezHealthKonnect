import os
import re
import ast
import json
from datetime import datetime
from typing import Dict, List, Tuple, Set
from pathlib import Path

# ---- PROJECT-SPECIFIC SETTINGS ----
ROOT_DIR = r"C:\Projects\ezHealthKonnect"
OUTPUT_DIR = "context"
IGNORE_DIRS = {
    ".git", "node_modules", "dist", "build", "out", "coverage", "__pycache__",
    ".next", ".turbo", "vendor", "bin", "obj", "target", ".venv", "env", "venv",
    "logs", "temp", "tmp", ".cache", ".sass-cache", "bower_components"
}

# CRITICAL DIRECTORIES TO ANALYZE (these will be analyzed in detail)
CRITICAL_DIRECTORIES = [
    "controllers", "public", "services", "routes", "config", 
    "fhir", "hl7", "database", "middleware", "models", "utils", 
    "lib", "src", "views", "assets", "migrations", "service"
]

# Comprehensive file extensions with specific handling
FILE_EXTENSIONS = {
    'javascript': ('.js', '.mjs', '.cjs'),
    'typescript': ('.ts', '.tsx'),
    'python': ('.py', '.pyw'),
    'go': ('.go',),
    'html': ('.html', '.htm', '.ejs', '.hbs', '.handlebars', '.pug', '.jade'),
    'css': ('.css', '.scss', '.sass', '.less', '.stylus'),
    'sql': ('.sql', '.psql', '.mysql'),
    'config': ('.json', '.yaml', '.yml', '.toml', '.ini', '.conf', '.config', '.env'),
    'markdown': ('.md', '.markdown', '.mdx'),
    'shell': ('.sh', '.bash', '.zsh', '.bat', '.cmd'),
    'docker': ('Dockerfile', '.dockerfile'),
    'other': ('.txt', '.xml', '.gitignore', '.dockerignore', '.editorconfig')
}

# ---------------------------
# 1) POSTGRES SCHEMA SUMMARY (Enhanced)
# ---------------------------
def schema_summary(pg_conn_str: str) -> str:
    try:
        import psycopg2
    except ImportError:
        return "# Schema Summary\n\n⚠️ psycopg2 not installed. Run: `pip install psycopg2-binary`.\n"

    try:
        conn = psycopg2.connect(pg_conn_str)
        cur = conn.cursor()
        
        # Get all tables with comprehensive column info
        cur.execute("""
            SELECT table_name, column_name, data_type, is_nullable, column_default,
                   character_maximum_length, numeric_precision, numeric_scale
            FROM information_schema.columns
            WHERE table_schema = 'public'
            ORDER BY table_name, ordinal_position;
        """)
        rows = cur.fetchall()

        # Get primary keys
        cur.execute("""
            SELECT tc.table_name, kcu.column_name
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

        # Get foreign keys
        cur.execute("""
            SELECT tc.table_name, kcu.column_name, ccu.table_name AS foreign_table, 
                   ccu.column_name AS foreign_column
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

        # Build table summaries
        tables = {}
        for table, col, dtype, nullable, default, max_len, precision, scale in rows:
            type_info = dtype
            if max_len:
                type_info += f"({max_len})"
            elif precision:
                type_info += f"({precision}"
                if scale:
                    type_info += f",{scale}"
                type_info += ")"
            
            nullable_str = " NULL" if nullable == 'YES' else " NOT NULL"
            
            tables.setdefault(table, []).append(
                f"{col} ({type_info}{nullable_str})"
            )

        # Generate summary
        lines = [f"# Schema Summary (Generated {datetime.utcnow().isoformat()})\n"]
        
        if not tables:
            lines.append("_No tables found in `public` schema._\n")
            return "\n".join(lines)

        lines.append(f"**Database Overview:** {len(tables)} tables with {sum(len(cols) for cols in tables.values())} total columns\n")
        
        for table_name in sorted(tables.keys()):
            lines.append(f"## {table_name}")
            
            if table_name in pk_map:
                lines.append(f"- **PK**: {', '.join(pk_map[table_name])}")
            
            if table_name in fk_map:
                for (col, foreign_table, foreign_col) in fk_map[table_name]:
                    lines.append(f"- **FK**: {col} → {foreign_table}.{foreign_col}")
            
            for col_info in tables[table_name]:
                lines.append(f"- {col_info}")
            
            lines.append("")
        
        return "\n".join(lines)

    except Exception as e:
        return f"# Schema Summary\n\n⚠️ Failed to connect or query Postgres: `{e}`\n"


# -----------------------------------
# 2) ENHANCED PROJECT STRUCTURE 
# -----------------------------------
def generate_comprehensive_directory_tree(root_path: str) -> str:
    """Generate detailed directory tree focusing on critical directories"""
    lines = ["# Complete Project Structure\n"]
    
    def get_file_type_icon(filename: str) -> str:
        """Get icon based on file type"""
        filename_lower = filename.lower()
        if filename_lower.endswith(('.js', '.mjs', '.cjs')):
            return "📄"
        elif filename_lower.endswith(('.ts', '.tsx')):
            return "📘"
        elif filename_lower.endswith('.py'):
            return "🐍"
        elif filename_lower.endswith('.go'):
            return "🔷"
        elif filename_lower.endswith(('.html', '.htm', '.ejs')):
            return "🌐"
        elif filename_lower.endswith(('.css', '.scss', '.sass')):
            return "🎨"
        elif filename_lower.endswith('.sql'):
            return "🗄️"
        elif filename_lower.endswith(('.json', '.yaml', '.yml')):
            return "⚙️"
        elif filename_lower.endswith('.md'):
            return "📝"
        else:
            return "📄"
    
    def add_tree_line(path: str, prefix: str = "", is_last: bool = True, depth: int = 0, max_depth: int = 4):
        if depth > max_depth:
            return
            
        name = os.path.basename(path)
        if should_skip_dir(path) and os.path.isdir(path):
            return
        
        # Special handling for critical directories - show more depth
        is_critical = any(critical_dir in path.lower() for critical_dir in CRITICAL_DIRECTORIES)
        effective_max_depth = max_depth + 2 if is_critical else max_depth
        
        connector = "└── " if is_last else "├── "
        
        if os.path.isfile(path):
            icon = get_file_type_icon(name)
            lines.append(f"{prefix}{connector}{icon} {name}")
        else:
            folder_icon = "📁"
            if is_critical:
                folder_icon = "📂"  # Open folder for critical dirs
            lines.append(f"{prefix}{connector}{folder_icon} {name}/")
        
        if os.path.isdir(path) and depth < effective_max_depth:
            try:
                items = sorted(os.listdir(path))
                dirs = [item for item in items if os.path.isdir(os.path.join(path, item)) 
                       and not should_skip_dir(os.path.join(path, item))]
                files = [item for item in items if os.path.isfile(os.path.join(path, item))]
                
                # For critical directories, show all files, otherwise limit
                file_limit = len(files) if is_critical else min(10, len(files))
                all_items = dirs + files[:file_limit]
                
                for i, item in enumerate(all_items):
                    is_last_item = i == len(all_items) - 1
                    extension = "    " if is_last else "│   "
                    add_tree_line(
                        os.path.join(path, item),
                        prefix + extension,
                        is_last_item and len(files) <= file_limit,
                        depth + 1,
                        effective_max_depth
                    )
                
                if len(files) > file_limit:
                    extension = "    " if is_last else "│   "
                    lines.append(f"{prefix}{extension}└── ... ({len(files) - file_limit} more files)")
                    
            except PermissionError:
                pass
    
    add_tree_line(root_path)
    return "\n".join(lines) + "\n"


# -----------------------------------
# 3) FILE DISCOVERY AND ANALYSIS
# -----------------------------------
def should_skip_dir(d: str) -> bool:
    """Check if directory should be skipped"""
    name = os.path.basename(d).lower()
    return name in IGNORE_DIRS or name.startswith(".")

def discover_all_files(root: str) -> Dict[str, List[Dict]]:
    """Discover and categorize ALL files in the project"""
    discovered_files = {category: [] for category in FILE_EXTENSIONS.keys()}
    discovered_files['uncategorized'] = []
    
    total_scanned = 0
    
    print(f"🔍 Scanning directory: {root}")
    
    for dirpath, dirnames, filenames in os.walk(root):
        # Don't skip critical directories even if they might match ignore patterns
        current_dir_name = os.path.basename(dirpath).lower()
        is_critical_dir = any(critical in dirpath.lower() for critical in CRITICAL_DIRECTORIES)
        
        if not is_critical_dir:
            # Prune ignored directories only for non-critical paths
            dirnames[:] = [d for d in dirnames if not should_skip_dir(os.path.join(dirpath, d))]
        
        for filename in filenames:
            total_scanned += 1
            
            # Skip some obvious files but be more permissive
            if filename.startswith('.') and filename not in ['.env', '.gitignore', '.dockerignore']:
                continue
                
            filepath = os.path.join(dirpath, filename)
            categorized = False
            
            # Get file size and line count
            try:
                file_size = os.path.getsize(filepath)
                with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                    line_count = sum(1 for _ in f)
            except Exception:
                file_size = 0
                line_count = 0
            
            file_info = {
                'path': filepath,
                'name': filename,
                'lines': line_count,
                'size': file_size,
                'dir': os.path.relpath(dirpath, root),
                'is_critical': is_critical_dir
            }
            
            # Categorize file
            for category, extensions in FILE_EXTENSIONS.items():
                if any(filename.lower().endswith(ext) for ext in extensions) or filename in extensions:
                    discovered_files[category].append(file_info)
                    categorized = True
                    break
            
            if not categorized:
                discovered_files['uncategorized'].append(file_info)
    
    print(f"📊 Total files scanned: {total_scanned}")
    for category, files in discovered_files.items():
        if files:
            print(f"  {category}: {len(files)} files")
    
    return discovered_files

def analyze_javascript_typescript_files(js_files: List[Dict], ts_files: List[Dict]) -> str:
    """Enhanced JavaScript/TypeScript analysis with route detection"""
    all_files = js_files + ts_files
    if not all_files:
        return "_No JavaScript/TypeScript files found_\n"
    
    # Sort by critical directories first, then by size
    all_files.sort(key=lambda x: (not x['is_critical'], -x['lines']))
    
    sections = [f"## JavaScript/TypeScript Files ({len(all_files)} files)\n"]
    
    # Enhanced patterns for better detection
    patterns = {
        'functions': [
            r'(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(',
            r'(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:function|\([^)]*\)\s*=>)',
            r'([A-Za-z_$][\w$]*)\s*:\s*(?:async\s+)?function\s*\(',
            r'([A-Za-z_$][\w$]*)\s*:\s*\([^)]*\)\s*=>',
            r'\.([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:function|\([^)]*\)\s*=>)'
        ],
        'classes': [
            r'(?:export\s+(?:default\s+)?)?class\s+([A-Za-z_$][\w$]*)',
            r'([A-Za-z_$][\w$]*)\s*=\s*class\b'
        ],
        'routes': [
            r'(?:router|app)\.\s*(?:get|post|put|patch|delete|use|all)\s*\(\s*[\'"`]([^\'"`,]+)[\'"`]',
            r'\.route\s*\(\s*[\'"`]([^\'"`,]+)[\'"`]\)',
            r'express\s*\(\s*\)\s*\.(?:get|post|put|patch|delete)\s*\(\s*[\'"`]([^\'"`,]+)[\'"`]'
        ],
        'middleware': [
            r'(?:export\s+(?:default\s+)?)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*\([^)]*\)\s*=>\s*\([^)]*\)\s*=>\s*\([^)]*\)\s*=>',
            r'function\s+([A-Za-z_$][\w$]*)\s*\([^)]*req[^)]*res[^)]*next[^)]*\)',
            r'\.use\s*\(\s*([A-Za-z_$][\w$]*)\s*\)'
        ],
        'imports': [
            r'import\s+.*?\s+from\s+[\'"`]([^\'"`,]+)[\'"`]',
            r'require\s*\(\s*[\'"`]([^\'"`,]+)[\'"`]\)',
            r'import\s*\(\s*[\'"`]([^\'"`,]+)[\'"`]\)'
        ]
    }
    
    total_funcs = total_classes = total_routes = 0
    
    for file_info in all_files[:25]:  # Show top 25 files
        filepath = file_info['path']
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            # Extract information using patterns
            functions = set()
            classes = set()
            routes = set()
            middleware = set()
            imports = set()
            
            for pattern in patterns['functions']:
                matches = re.findall(pattern, content, re.MULTILINE | re.IGNORECASE)
                functions.update([m for m in matches if m and len(m) < 50])
                
            for pattern in patterns['classes']:
                matches = re.findall(pattern, content, re.MULTILINE)
                classes.update([m for m in matches if m])
                
            for pattern in patterns['routes']:
                matches = re.findall(pattern, content, re.MULTILINE)
                routes.update([m for m in matches if m and len(m) < 100])
                
            for pattern in patterns['middleware']:
                matches = re.findall(pattern, content, re.MULTILINE)
                middleware.update([m for m in matches if m])
            
            for pattern in patterns['imports']:
                matches = re.findall(pattern, content, re.MULTILINE)
                imports.update([m for m in matches if m and not m.startswith('.')])
            
            total_funcs += len(functions)
            total_classes += len(classes)
            total_routes += len(routes)
            
            if functions or classes or routes or middleware:
                rel_path = os.path.relpath(filepath, ROOT_DIR)
                file_type = "TypeScript" if filepath.endswith(('.ts', '.tsx')) else "JavaScript"
                critical_marker = "🔥 " if file_info['is_critical'] else ""
                
                sections.append(f"### {critical_marker}{rel_path}")
                sections.append(f"**Type:** {file_type} | **Lines:** {file_info['lines']} | **Directory:** {file_info['dir']}")
                
                if classes:
                    sections.append(f"**Classes:** `{', '.join(list(classes)[:8])}`")
                if functions:
                    sections.append(f"**Functions:** `{', '.join(list(functions)[:12])}`")
                if routes:
                    sections.append(f"**Routes:** `{', '.join(list(routes)[:8])}`")
                if middleware:
                    sections.append(f"**Middleware:** `{', '.join(list(middleware)[:5])}`")
                
                # Key external imports only
                key_imports = [imp for imp in imports if '/' not in imp and 
                             imp not in ['express', 'react', 'lodash', 'axios', 'fs', 'path', 'util']][:6]
                if key_imports:
                    sections.append(f"**Key Imports:** `{', '.join(key_imports)}`")
                sections.append("")
                
        except Exception as e:
            continue
    
    summary = f"**Summary:** {total_funcs} functions, {total_classes} classes, {total_routes} routes across {len(all_files)} files\n\n"
    return summary + "\n".join(sections)

def analyze_html_template_files(files: List[Dict]) -> str:
    """Analyze HTML and template files"""
    if not files:
        return "_No HTML/template files found_\n"
    
    sections = [f"## HTML/Template Files ({len(files)} files)\n"]
    
    patterns = {
        'forms': r'<form[^>]*>',
        'inputs': r'<input[^>]*type=[\'"](\w+)[\'"]',
        'scripts': r'<script[^>]*src=[\'"]([^\'"]+)[\'"]',
        'stylesheets': r'<link[^>]*href=[\'"]([^\'"]+\.css[^\'"]*)[\'"]',
        'templates': r'\{\{([^}]+)\}\}|\<%([^%]+)%\>|\$\{([^}]+)\}',
        'ids': r'id=[\'"]([^\'"]+)[\'"]',
        'classes': r'class=[\'"]([^\'"]+)[\'"]'
    }
    
    for file_info in sorted(files, key=lambda x: x['lines'], reverse=True)[:15]:
        filepath = file_info['path']
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            forms = len(re.findall(patterns['forms'], content, re.IGNORECASE))
            input_types = set(re.findall(patterns['inputs'], content, re.IGNORECASE))
            scripts = set(re.findall(patterns['scripts'], content, re.IGNORECASE))
            stylesheets = set(re.findall(patterns['stylesheets'], content, re.IGNORECASE))
            template_vars = set()
            
            # Extract template variables from different template engines
            for pattern in [r'\{\{([^}]+)\}\}', r'\<%([^%]+)%\>', r'\$\{([^}]+)\}']:
                matches = re.findall(pattern, content)
                template_vars.update([m.strip() for m in matches if m.strip()])
            
            if forms > 0 or input_types or scripts or template_vars:
                rel_path = os.path.relpath(filepath, ROOT_DIR)
                critical_marker = "🔥 " if file_info['is_critical'] else ""
                
                sections.append(f"### {critical_marker}{rel_path}")
                sections.append(f"**Lines:** {file_info['lines']} | **Directory:** {file_info['dir']}")
                
                if forms > 0:
                    sections.append(f"**Forms:** {forms}")
                if input_types:
                    sections.append(f"**Input Types:** `{', '.join(list(input_types)[:8])}`")
                if scripts:
                    script_names = [os.path.basename(s) for s in scripts]
                    sections.append(f"**Scripts:** `{', '.join(script_names[:5])}`")
                if stylesheets:
                    style_names = [os.path.basename(s) for s in stylesheets]
                    sections.append(f"**Stylesheets:** `{', '.join(style_names[:5])}`")
                if template_vars:
                    sections.append(f"**Template Variables:** `{', '.join(list(template_vars)[:8])}`")
                sections.append("")
                
        except Exception:
            continue
    
    return "\n".join(sections)

def analyze_python_files(files: List[Dict]) -> str:
    """Enhanced Python file analysis"""
    if not files:
        return "_No Python files found_\n"
    
    # Sort by critical directories first, then by size
    files.sort(key=lambda x: (not x['is_critical'], -x['lines']))
    
    sections = [f"## Python Files ({len(files)} files)\n"]
    total_funcs = total_classes = 0
    
    for file_info in files[:20]:  # Top 20 files
        filepath = file_info['path']
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            try:
                tree = ast.parse(content)
                funcs = []
                classes = []
                imports = []
                decorators = []
                
                for node in ast.walk(tree):
                    if isinstance(node, ast.FunctionDef):
                        # Get function decorators
                        func_decorators = [d.id if isinstance(d, ast.Name) else str(d) for d in node.decorator_list[:3]]
                        if func_decorators:
                            decorators.extend(func_decorators)
                        
                        if node.col_offset == 0:  # Top-level functions
                            args = [a.arg for a in node.args.args]
                            funcs.append(f"`{node.name}`({', '.join(args[:4])})")
                            total_funcs += 1
                            
                    elif isinstance(node, ast.ClassDef) and node.col_offset == 0:  # Top-level classes
                        methods = [n.name for n in ast.walk(node) if isinstance(n, ast.FunctionDef)]
                        classes.append(f"`{node.name}` ({len(methods)} methods)")
                        total_classes += 1
                        
                    elif isinstance(node, ast.Import):
                        imports.extend([alias.name for alias in node.names])
                    elif isinstance(node, ast.ImportFrom) and node.module:
                        imports.append(node.module)
                
                if funcs or classes:
                    rel_path = os.path.relpath(filepath, ROOT_DIR)
                    critical_marker = "🔥 " if file_info['is_critical'] else ""
                    
                    sections.append(f"### {critical_marker}{rel_path}")
                    sections.append(f"**Lines:** {file_info['lines']} | **Directory:** {file_info['dir']}")
                    
                    if classes:
                        sections.append(f"**Classes:** {', '.join(classes[:5])}")
                    if funcs:
                        sections.append(f"**Functions:** {', '.join(funcs[:10])}")
                    if decorators:
                        unique_decorators = list(set(decorators))
                        sections.append(f"**Decorators:** `{', '.join(unique_decorators[:6])}`")
                    
                    # Key imports only
                    key_imports = [imp for imp in set(imports[:15]) if not imp.startswith('_') and 
                                 '.' not in imp and imp not in ['os', 'sys', 're', 'json', 'datetime']]
                    if key_imports:
                        sections.append(f"**Key Imports:** `{', '.join(key_imports[:8])}`")
                    sections.append("")
                    
            except SyntaxError:
                rel_path = os.path.relpath(filepath, ROOT_DIR)
                critical_marker = "🔥 " if file_info['is_critical'] else ""
                sections.append(f"### {critical_marker}{rel_path}")
                sections.append(f"**Lines:** {file_info['lines']} | ⚠️ _Syntax error in file_")
                sections.append("")
                
        except Exception:
            continue
    
    summary = f"**Summary:** {total_funcs} functions, {total_classes} classes across {len(files)} files\n\n"
    return summary + "\n".join(sections)

def analyze_go_files(files: List[Dict]) -> str:
    """Enhanced Go file analysis"""
    if not files:
        return "_No Go files found_\n"
    
    files.sort(key=lambda x: (not x['is_critical'], -x['lines']))
    
    sections = [f"## Go Files ({len(files)} files)\n"]
    
    patterns = {
        'functions': r'func\s+(?:\([^)]+\)\s*)?([A-Za-z_]\w*)\s*\(',
        'types': r'type\s+([A-Za-z_]\w+)\s+(struct|interface|map|chan|\[\]|\*)',
        'imports': r'import\s+(?:\(\s*([^)]+)\s*\)|"([^"]+)")',
        'packages': r'package\s+(\w+)',
        'methods': r'func\s+\([^)]+\)\s*([A-Za-z_]\w*)\s*\('
    }
    
    total_funcs = total_types = 0
    
    for file_info in files[:15]:
        filepath = file_info['path']
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            functions = re.findall(patterns['functions'], content)
            methods = re.findall(patterns['methods'], content)
            types = re.findall(patterns['types'], content)
            package = re.search(patterns['packages'], content)
            
            total_funcs += len(functions) + len(methods)
            total_types += len(types)
            
            if functions or methods or types:
                rel_path = os.path.relpath(filepath, ROOT_DIR)
                pkg_name = package.group(1) if package else "unknown"
                critical_marker = "🔥 " if file_info['is_critical'] else ""
                
                sections.append(f"### {critical_marker}{rel_path}")
                sections.append(f"**Package:** {pkg_name} | **Lines:** {file_info['lines']} | **Directory:** {file_info['dir']}")
                
                if types:
                    type_summary = {}
                    for name, kind in types:
                        type_summary.setdefault(kind, []).append(name)
                    
                    for kind, names in type_summary.items():
                        sections.append(f"**{kind.title()}s:** `{', '.join(names[:5])}`")
                
                if functions:
                    sections.append(f"**Functions:** `{', '.join(functions[:8])}`")
                
                if methods:
                    sections.append(f"**Methods:** `{', '.join(methods[:8])}`")
                
                sections.append("")
                
        except Exception:
            continue
    
    summary = f"**Summary:** {total_funcs} functions/methods, {total_types} types across {len(files)} files\n\n"
    return summary + "\n".join(sections)

def analyze_config_sql_files(config_files: List[Dict], sql_files: List[Dict]) -> str:
    """Analyze configuration and SQL files"""
    all_files = config_files + sql_files
    if not all_files:
        return "_No configuration or SQL files found_\n"
    
    sections = [f"## Configuration & SQL Files ({len(all_files)} files)\n"]
    
    for file_info in sorted(all_files, key=lambda x: x['lines'], reverse=True)[:15]:
        filepath = file_info['path']
        rel_path = os.path.relpath(filepath, ROOT_DIR)
        critical_marker = "🔥 " if file_info['is_critical'] else ""
        
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()[:2000]  # First 2000 chars
            
            sections.append(f"### {critical_marker}{rel_path}")
            sections.append(f"**Lines:** {file_info['lines']} | **Size:** {file_info['size']} bytes | **Directory:** {file_info['dir']}")
            
            # Try to parse JSON files
            if filepath.endswith('.json'):
                try:
                    data = json.loads(content if len(content) < 2000 else open(filepath, 'r').read())
                    if isinstance(data, dict):
                        keys = list(data.keys())[:10]
                        sections.append(f"**JSON Keys:** `{', '.join(keys)}`")
                except:
                    sections.append("_JSON parsing failed_")
            
            # Analyze SQL files
            elif filepath.endswith('.sql'):
                tables = re.findall(r'(?:CREATE|ALTER|DROP)\s+TABLE\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?([A-Za-z_]\w*)', content, re.IGNORECASE)
                procedures = re.findall(r'(?:CREATE|ALTER)\s+(?:PROCEDURE|FUNCTION)\s+([A-Za-z_]\w*)', content, re.IGNORECASE)
                if tables:
                    sections.append(f"**Tables:** `{', '.join(set(tables)[:8])}`")
                if procedures:
                    sections.append(f"**Procedures/Functions:** `{', '.join(set(procedures)[:5])}`")
            
            sections.append("")
            
        except Exception:
            continue
    
    return "\n".join(sections)

def analyze_directory_specific_patterns(discovered_files: Dict[str, List[Dict]]) -> str:
    """Analyze patterns specific to each critical directory"""
    sections = ["## Directory-Specific Analysis\n"]
    
    # Group files by directory
    dir_analysis = {}
    for category, files in discovered_files.items():
        for file_info in files:
            if file_info['is_critical']:
                dir_name = file_info['dir'].split(os.sep)[0] if os.sep in file_info['dir'] else file_info['dir']
                if dir_name not in dir_analysis:
                    dir_analysis[dir_name] = {'files': [], 'categories': set()}
                dir_analysis[dir_name]['files'].append(file_info)
                dir_analysis[dir_name]['categories'].add(category)
    
    for dir_name, data in sorted(dir_analysis.items()):
        if data['files']:
            file_count = len(data['files'])
            total_lines = sum(f['lines'] for f in data['files'])
            categories = ', '.join(sorted(data['categories']))
            
            sections.append(f"### 📂 {dir_name}/ Directory")
            sections.append(f"**Files:** {file_count} | **Total Lines:** {total_lines:,} | **Types:** {categories}")
            
            # Show key files in this directory
            key_files = sorted(data['files'], key=lambda x: x['lines'], reverse=True)[:8]
            for file_info in key_files:
                file_path = file_info['path'].replace(ROOT_DIR, '').lstrip(os.sep)
                sections.append(f"- `{file_path}` ({file_info['lines']} lines)")
            
            if len(data['files']) > 8:
                sections.append(f"- ... and {len(data['files']) - 8} more files")
            
            sections.append("")
    
    return "\n".join(sections)

# ---------------------------
# 4) MAIN ANALYSIS FUNCTION
# ---------------------------
def generate_comprehensive_summary(root: str) -> str:
    """Generate comprehensive project summary"""
    print("🔍 Starting comprehensive project analysis...")
    
    # Discover all files
    discovered_files = discover_all_files(root)
    
    # Calculate totals
    total_files = sum(len(files) for files in discovered_files.values())
    total_lines = sum(sum(f['lines'] for f in files) for files in discovered_files.values())
    
    # Generate sections
    sections = [
        f"# Comprehensive Project Analysis",
        f"**Generated:** {datetime.utcnow().isoformat()}",
        f"**Project Root:** {root}",
        f"**Total Files Analyzed:** {total_files:,} ({total_lines:,} total lines)\n",
        
        "## File Statistics by Category\n",
    ]
    
    # File count summary table
    sections.append("| Category | Files | Lines | Avg Lines/File | Critical Files |")
    sections.append("|----------|-------|-------|----------------|----------------|")
    
    for category, files in discovered_files.items():
        if files:
            total_category_lines = sum(f['lines'] for f in files)
            critical_count = sum(1 for f in files if f['is_critical'])
            avg_lines = total_category_lines // len(files) if files else 0
            sections.append(f"| {category.title()} | {len(files)} | {total_category_lines:,} | {avg_lines} | {critical_count} |")
    
    sections.extend(["", "---", ""])
    
    # Critical directories analysis
    sections.append(analyze_directory_specific_patterns(discovered_files))
    
    # Detailed analysis by file type
    sections.append(analyze_javascript_typescript_files(
        discovered_files['javascript'], 
        discovered_files['typescript']
    ))
    
    sections.append(analyze_python_files(discovered_files['python']))
    sections.append(analyze_go_files(discovered_files['go']))
    sections.append(analyze_html_template_files(discovered_files['html']))
    sections.append(analyze_config_sql_files(
        discovered_files['config'], 
        discovered_files['sql']
    ))
    
    # Show uncategorized files if any
    if discovered_files['uncategorized']:
        sections.append(f"## Uncategorized Files ({len(discovered_files['uncategorized'])} files)\n")
        for file_info in discovered_files['uncategorized'][:10]:
            rel_path = os.path.relpath(file_info['path'], ROOT_DIR)
            sections.append(f"- `{rel_path}` ({file_info['lines']} lines)")
        if len(discovered_files['uncategorized']) > 10:
            sections.append(f"- ... and {len(discovered_files['uncategorized']) - 10} more files")
        sections.append("")
    
    return "\n".join(sections)

def save_text(filename: str, text: str):
    """Save text to output directory"""
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    path = os.path.join(OUTPUT_DIR, filename)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    size_kb = len(text.encode('utf-8')) / 1024
    print(f"✅ Saved {path} ({len(text.splitlines())} lines, {size_kb:.1f} KB)")

# ---------------------------
# MAIN EXECUTION
# ---------------------------
if __name__ == "__main__":
    print("🚀 Starting enhanced project analysis...")
    print(f"📁 Root directory: {ROOT_DIR}")
    print(f"🔍 Critical directories: {', '.join(CRITICAL_DIRECTORIES)}")
    
    # 1) Generate directory structure
    print("\n📁 Generating project structure...")
    tree_text = generate_comprehensive_directory_tree(ROOT_DIR)
    save_text("project_structure.md", tree_text)
    
    # 2) Generate comprehensive code summary
    print("\n📊 Analyzing all code files...")
    code_summary = generate_comprehensive_summary(ROOT_DIR)
    save_text("code_summary.md", code_summary)
    
    # 3) Generate schema summary (if database connection available)
    print("\n🗄️ Analyzing database schema...")
    PG_CONN_STR = "dbname=ezhealthkonnect user=ezhealth_user password=secure_password_change_me host=localhost port=5432"
    schema_text = schema_summary(PG_CONN_STR)
    save_text("schema_summary.md", schema_text)
    
    print(f"\n🎉 Analysis complete!")
    print(f"📂 Output directory: {os.path.abspath(OUTPUT_DIR)}")
    print("📄 Generated files:")
    print("  - project_structure.md (directory tree)")
    print("  - code_summary.md (comprehensive code analysis)")
    print("  - schema_summary.md (database schema)")
    print("\n💡 Files in critical directories are marked with 🔥")