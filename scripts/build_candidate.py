#!/usr/bin/env python3
"""Build a traceable Linux candidate. Dirty builds must be explicitly marked local-only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
def run(*args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)
def output(*args):
    return run(*args, stdout=subprocess.PIPE).stdout.decode().strip()
def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--arch", choices=["amd64", "arm64"], default="amd64")
    parser.add_argument("--allow-dirty", action="store_true", help="local-only diagnostic artifact; not a release candidate")
    args = parser.parse_args()
    dirty = bool(output("git", "status", "--porcelain"))
    if dirty and not args.allow_dirty:
        parser.error("candidate requires a clean checked-out revision; commit/release is a separate authorized step")
    dest = args.output.resolve()
    dest.mkdir(parents=True, exist_ok=False)
    # Build embedded Web assets before hashing and compiling the binaries.
    run("make", "web")
    source = hashlib.sha256()
    paths = set(run("git", "ls-files", "-z", stdout=subprocess.PIPE).stdout.split(b"\0"))
    paths.update(run("git", "ls-files", "--others", "--exclude-standard", "-z", stdout=subprocess.PIPE).stdout.split(b"\0"))
    for raw in sorted(paths):
        if not raw or raw.startswith(b".idea/"): continue
        path = ROOT / os.fsdecode(raw)
        if path.is_file(): source.update(raw + b"\0" + path.read_bytes() + b"\0")
    env = dict(os.environ, GOOS="linux", GOARCH=args.arch, CGO_ENABLED="0")
    files = {}
    for name in ["skillhub-server", "skillhub"]:
        path = dest / name
        run("go", "build", "-trimpath", "-buildvcs=true", "-o", str(path), "./cmd/" + name, env=env)
        files[name] = sha(path)
    metadata = {"revision": output("git", "rev-parse", "HEAD"), "dirty": dirty,
                "release_candidate": not dirty, "source_sha256": source.hexdigest(),
                "toolchain": output("go", "version"), "target": "linux/" + args.arch,
                "artifacts": files, "target_acceptance": "not_run"}
    (dest / "manifest.json").write_text(json.dumps(metadata, indent=2) + "\n")
    (dest / "SHA256SUMS").write_text("".join(f"{value}  {name}\n" for name, value in files.items()))
    print(json.dumps(metadata, indent=2))
if __name__ == "__main__": main()
