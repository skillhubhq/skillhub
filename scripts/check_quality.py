#!/usr/bin/env python3
"""Check distributable source hygiene and local documentation links without secrets."""
import re
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit
from project_identity import check_project_references, check_application_references

ROOT = Path(__file__).resolve().parents[1]


def main():
    raw = subprocess.check_output(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=ROOT)
    paths = sorted(set(p for p in raw.decode().split("\0") if p))
    errors = []
    required = ["LICENSE", "NOTICE", "README.md", "README.zh-CN.md", "CONTRIBUTING.md", "SECURITY.md", "THIRD_PARTY_NOTICES.md"]
    for name in required:
        if not (ROOT / name).is_file():
            errors.append(f"Missing project document: {name}")
    for name in paths:
        path = ROOT / name
        if not path.is_file():
            continue
        try:
            check_project_references(path.read_bytes(), name)
            check_application_references(path.read_bytes(), name)
        except ValueError as error:
            errors.append(str(error))
        private_suffixes = (".db", ".db-wal", ".db-shm", ".sqlite", ".sqlite3", ".sqlite-wal", ".sqlite-shm", ".sqlite3-wal", ".sqlite3-shm", ".pem", ".key", ".p12", ".pfx", ".local.yaml", ".local.yml")
        private_env = path.name == ".env" or (path.name.startswith(".env.") and path.name != ".env.example")
        if any(part in {".local", ".idea", ".agents", "node_modules", "backups", "secrets"} for part in path.relative_to(ROOT).parts) or name.endswith(private_suffixes) or private_env:
            errors.append(f"Local/private artifact included in source: {name}")
        if name.startswith("desktop/frontend/src/") and path.suffix == ".tsx":
            if re.search(r"<select(?:\s|>)", path.read_text()):
                errors.append(f"Use the shared themed Select instead of a native select: {name}")
        if path.suffix == ".md":
            content = path.read_text()
            for target in re.findall(r'\]\(([^\s)]+)(?:\s+"[^"]*")?\)', content):
                parsed = urlsplit(target.strip("<>"))
                if parsed.scheme or parsed.netloc or not parsed.path:
                    continue
                local = (path.parent / unquote(parsed.path)).resolve()
                if not local.is_relative_to(ROOT) or not local.exists():
                    errors.append(f"Broken local link in {name}: {target}")
    if errors:
        raise SystemExit("\n".join(errors))
    print(f"Source hygiene and Markdown file links passed ({len(paths)} files)")


if __name__ == "__main__":
    main()
