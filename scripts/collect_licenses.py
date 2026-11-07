#!/usr/bin/env python3
"""Collect shipped dependency notices; fail on missing texts or stale overrides."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--desktop", action="store_true")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    module = ROOT / "desktop" if args.desktop else ROOT
    targets = ["."] if args.desktop else ["./cmd/skillhub", "./cmd/skillhub-server"]
    subprocess.run(["go", "run", "github.com/google/go-licenses/v2@v2.0.1", "save", *targets,
                    "--ignore", "github.com/skillhubhq/skillhub", "--save_path", str(output / "go")], cwd=module, check=True)
    result = subprocess.run(["go", "list", "-m", "-json", "all"], cwd=module, check=True, capture_output=True)
    remaining = result.stdout.decode().strip()
    modules = []
    decoder = json.JSONDecoder()
    while remaining:
        module_info, offset = decoder.raw_decode(remaining)
        modules.append({k: module_info[k] for k in ["Path", "Version", "Sum", "GoVersion", "Main"] if k in module_info})
        remaining = remaining[offset:].lstrip()
    (output / "go-modules.json").write_text(json.dumps(modules, indent=2) + "\n")
    frontend = ROOT / "desktop/frontend"
    lock = json.loads((frontend / "package-lock.json").read_text())
    overrides = json.loads((ROOT / "third_party/npm/sources.json").read_text())
    inventory = []
    for relative, entry in lock["packages"].items():
        if not relative or entry.get("dev"):
            continue
        package = frontend / relative
        metadata = json.loads((package / "package.json").read_text())
        name, version = metadata["name"], metadata["version"]
        dest = output / "npm" / relative.removeprefix("node_modules/")
        dest.mkdir(parents=True, exist_ok=True)
        texts = [p for p in package.iterdir() if p.is_file() and p.name.lower().startswith(("license", "licence", "copying", "notice"))]
        if texts:
            for path in texts:
                shutil.copyfile(path, dest / path.name)
        else:
            override = overrides.get(name)
            if not override or override["version"] != version:
                raise ValueError(f"Missing reviewed license text for {name}@{version}")
            source = ROOT / "third_party/npm" / override["file"]
            if hashlib.sha256(source.read_bytes()).hexdigest() != override["sha256"]:
                raise ValueError(f"License checksum mismatch: {name}")
            shutil.copyfile(source, dest / "LICENSE")
        for notice in overrides.get(name, {}).get("notices", []):
            if overrides[name]["version"] != version:
                raise ValueError(f"Review additional notices for {name}@{version}")
            source = ROOT / "third_party/npm" / notice["file"]
            if hashlib.sha256(source.read_bytes()).hexdigest() != notice["sha256"]:
                raise ValueError(f"Notice checksum mismatch: {name}")
            shutil.copyfile(source, dest / notice["file"])
        inventory.append({"name": name, "version": version, "license": metadata.get("license"), "integrity": entry.get("integrity")})
    (output / "npm-packages.json").write_text(json.dumps(inventory, indent=2) + "\n")
    shutil.copyfile(ROOT / "sdk/typescript/LICENSE.protovalidate", output / "LICENSE.protovalidate")
    print(f"Collected dependency notices for {len(inventory)} frontend packages and the current Go target")


if __name__ == "__main__":
    main()
