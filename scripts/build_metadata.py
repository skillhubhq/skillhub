#!/usr/bin/env python3
"""Record the actual binary target and revision before uploading CI artifacts."""
import argparse
import json
import os
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument("--binary", required=True)
parser.add_argument("--output", required=True, type=Path)
args = parser.parse_args()
output = subprocess.check_output(["go", "version", "-m", args.binary], text=True)
settings = {}
for line in output.splitlines():
    words = line.split()
    if len(words) == 2 and words[0] == "build" and "=" in words[1]:
        key, value = words[1].split("=", 1)
        settings[key] = value
revision = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
if settings.get("vcs.revision") != revision or settings.get("vcs.modified") != "false":
    raise SystemExit("Release binary must identify this exact clean Git revision")
if os.environ.get("GITHUB_SHA", revision) != revision:
    raise SystemExit("Checkout does not match the CI source revision")
args.output.write_text(json.dumps({"revision": revision, "goos": settings["GOOS"], "goarch": settings["GOARCH"],
                                   "dirty": False, "toolchain": output.splitlines()[0].split(": ")[-1]}, indent=2) + "\n")
