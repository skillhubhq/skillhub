#!/usr/bin/env python3
"""Copy the verified static documentation export into the embedded Hub assets."""
from pathlib import Path
import shutil

ROOT = Path(__file__).resolve().parents[1]


def main():
    source = ROOT / "website/out"
    target = ROOT / "internal/webui/static/docs"
    for required in ("index.html", "csp.json"):
        if not (source / required).is_file():
            raise SystemExit("Build and verify documentation before copying it")
    if target.exists():
        shutil.rmtree(target)
    shutil.copytree(source, target)
    print("Copied verified static documentation")


if __name__ == "__main__":
    main()
