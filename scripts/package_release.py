#!/usr/bin/env python3
"""Package exact CI artifacts into a new release directory; never rebuild binaries."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import tarfile
import tempfile
import zipfile
from project_identity import check_project_references

ROOT = Path(__file__).resolve().parents[1]
DOCUMENTS = ["LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"]


def package(tag, revision, run_id, artifacts, dest):
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", tag) or not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Invalid version tag or full source revision")
    version = json.loads((ROOT / "desktop/frontend/package.json").read_text())["version"]
    if tag != "v" + version:
        raise ValueError("Release tag must match the App package version in the source revision")
    dest.mkdir(parents=True, exist_ok=False)
    provenance = {"version": tag, "revision": revision, "ci_run": str(run_id), "desktop_signed": False,
                  "target_deployment_acceptance": "not_run", "artifacts": {}}
    for runner in ["ubuntu-latest", "macos-latest", "windows-latest"]:
        for kind in (["hub-cli"] if runner == "ubuntu-latest" else ["hub-cli", "desktop"]):
            source = artifacts / f"{kind}-{runner}"
            metadata = json.loads((source / "build.json").read_text())
            if metadata["revision"] != revision or metadata["dirty"]:
                raise ValueError("Artifact source revision/cleanliness mismatch")
            target = {"ubuntu-latest": ("linux", "amd64"), "macos-latest": ("darwin", "arm64"), "windows-latest": ("windows", "amd64")}[runner]
            if (metadata["goos"], metadata["goarch"]) != target:
                raise ValueError("CI runner architecture changed; update the supported release matrix explicitly")
            if kind == "desktop":
                report = json.loads((artifacts / f"native-runtime-{runner}" / "native-smoke.json").read_text())
                if report.get("passed") is not True or report.get("error") or not report.get("checks"):
                    raise ValueError("Native acceptance did not pass")
                provenance.setdefault("native_checks", {})[runner] = report["checks"]
            os_name = "macos" if target[0] == "darwin" else target[0]
            suffix = "zip" if kind == "desktop" or os_name == "windows" else "tar.gz"
            prefix = "skillhub-app" if kind == "desktop" else "skillhub"
            archive = dest / f"{prefix}-{tag}-{os_name}-{target[1]}.{suffix}"
            with tempfile.TemporaryDirectory(prefix="skillhub-package-") as temp:
                stage = Path(temp)
                for name in DOCUMENTS:
                    shutil.copyfile(ROOT / name, stage / name)
                shutil.copytree(source / "third-party", stage / "third-party")
                shutil.copyfile(ROOT / "docs/distribution.md", stage / "DISTRIBUTION.md")
                if kind == "hub-cli":
                    for binary in ["skillhub", "skillhub-server"]:
                        binary += ".exe" if os_name == "windows" else ""
                        shutil.copyfile(source / binary, stage / binary)
                        (stage / binary).chmod(0o755)
                elif os_name == "windows":
                    shutil.copyfile(source / "SkillHub.exe", stage / "SkillHub.exe")
                for path in stage.rglob("*"):
                    if path.is_file():
                        check_project_references(path.read_bytes(), str(path.relative_to(stage)))
                if suffix == "zip":
                    if kind == "desktop" and os_name == "macos":
                        with zipfile.ZipFile(source / "SkillHub-macos.zip") as app_zip:
                            for entry in app_zip.infolist():
                                if not entry.is_dir():
                                    check_project_references(app_zip.read(entry), entry.filename)
                        shutil.copyfile(source / "SkillHub-macos.zip", archive)
                    with zipfile.ZipFile(archive, "a", compression=zipfile.ZIP_DEFLATED) as out:
                        for path in sorted(stage.rglob("*")):
                            if path.is_file(): out.write(path, path.relative_to(stage))
                else:
                    with tarfile.open(archive, "w:gz") as out:
                        for path in sorted(stage.iterdir()): out.add(path, arcname=path.name)
            provenance["artifacts"][archive.name] = {"sha256": hashlib.sha256(archive.read_bytes()).hexdigest(), "target": "/".join(target)}
            if kind == "desktop" and os_name == "macos":
                image = source / "SkillHub-macos.dmg"
                image_metadata = json.loads((source / "SkillHub-macos.dmg.json").read_text())
                digest = hashlib.sha256(image.read_bytes()).hexdigest()
                if (image_metadata.get("revision") != revision or image_metadata.get("version") != version
                        or image_metadata.get("build") != "source" or image_metadata.get("sha256") != digest):
                    raise ValueError("DMG source version/revision/integrity mismatch")
                image_output = dest / f"skillhub-app-{tag}-macos-{target[1]}.dmg"
                shutil.copyfile(image, image_output)
                provenance["artifacts"][image_output.name] = {"sha256": digest, "target": "/".join(target)}
    (dest / "SHA256SUMS").write_text("".join(f"{v['sha256']}  {k}\n" for k, v in sorted(provenance["artifacts"].items())))
    (dest / "PROVENANCE.json").write_text(json.dumps(provenance, indent=2) + "\n")
    return provenance


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--tag", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    package(args.tag, args.revision, args.run_id, args.artifacts, args.output)
