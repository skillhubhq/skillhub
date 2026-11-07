"""Exercise real App replacement with isolated state and the OS credential store.

Requires native-smoke instrumented baseline/candidate builds. Does not use token
injection, change live installations, publish artifacts or contact a real Hub.
"""
import argparse
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import plistlib
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from smoke import free_port

ROOT = Path(__file__).resolve().parents[1]


def run(args, **kwargs):
    return subprocess.run([str(a) for a in args], check=True, timeout=kwargs.pop("timeout", 180), capture_output=True, **kwargs)


@contextmanager
def build_baseline(reference):
    """Overlay only test instrumentation onto a detached comparison revision."""
    revision = run(["git", "rev-parse", "--verify", "--end-of-options", reference + "^{commit}"], cwd=ROOT, text=True).stdout.strip()
    scratch = ROOT / ".local/desktop-upgrade"
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=scratch) as temporary:
        worktree = Path(temporary) / "baseline"
        run(["git", "worktree", "add", "--detach", worktree, revision], cwd=ROOT)
        try:
            overlays = ["desktop/native_upgrade.go", "desktop/frontend/src/nativeUpgrade.ts"]
            for name in overlays:
                shutil.copy2(ROOT / name, worktree / name)
            entry = worktree / "desktop/frontend/src/nativeSmoke.ts"
            source = entry.read_text(encoding="utf-8")
            if 'from "./nativeUpgrade"' not in source:
                hook = "export async function runNativeSmoke() {"
                if hook not in source: raise RuntimeError("comparison revision lacks the supported native smoke entry")
                source = 'import { runUpgradeSmoke } from "./nativeUpgrade";\n' + source.replace(hook, hook + "\n  if (await runUpgradeSmoke()) return;", 1)
                entry.write_text(source, encoding="utf-8")
            npm = "npm.cmd" if os.name == "nt" else "npm"
            for directory in ("sdk/typescript", "desktop/frontend"):
                run([npm, "ci", "--ignore-scripts"], cwd=worktree / directory, timeout=600)
            run(["node", "build.mjs", "--native-smoke"], cwd=worktree / "desktop", timeout=900)
            app = worktree / "desktop/bin" / ("SkillHub-smoke.app" if sys.platform == "darwin" else "SkillHub-smoke.exe")
            provenance = {"baseline_revision": revision, "baseline_test_overlays": {
                name: hashlib.sha256((worktree / name).read_bytes()).hexdigest()
                for name in [*overlays, "desktop/frontend/src/nativeSmoke.ts"]}}
            yield app, provenance
        finally:
            run(["git", "worktree", "remove", "--force", worktree], cwd=ROOT)


def exercise(baseline, candidate, server, cli, report_path, provenance=None):
    if sys.platform not in ("darwin", "win32"):
        raise RuntimeError("native App replacement requires macOS or Windows")
    evidence = {"passed": False, "phases": [], "platform": sys.platform, **(provenance or {})}
    processes = []
    env = {k: v for k, v in os.environ.items() if not k.startswith("SKILLHUB_TOKEN_")}
    scratch = ROOT / ".local/desktop-upgrade"
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=scratch) as temporary:
        root = Path(temporary)
        data = root / "client"
        data.mkdir(mode=0o700)
        env.update(SKILLHUB_HOME=str(data), SKILLHUB_NATIVE_SMOKE="1")
        (data / ".native-smoke-fixture").write_bytes(b"skillhub-native-smoke-v1\n")
        def binary(app):
            return app / "Contents/MacOS/SkillHub" if sys.platform == "darwin" else app
        def app_hash(app):
            return hashlib.sha256(binary(app).read_bytes()).hexdigest()
        def client(*args, password=None):
            return run([cli, "--data", data, "--json", *args], input=password, text=True, env=env)
        installed = root / "Applications" / baseline.name
        installed.parent.mkdir()
        def replace(app):
            if installed.is_dir(): shutil.rmtree(installed)
            elif installed.exists(): installed.unlink()
            if sys.platform == "darwin":
                run(["ditto", app, installed])
                run(["codesign", "--verify", "--strict", installed])
            else: shutil.copy2(app, installed)
            if app_hash(app) != app_hash(installed): raise RuntimeError("application replacement digest mismatch")
        def phase(name):
            output = data / "native-smoke.json"
            output.unlink(missing_ok=True)
            phase_env = dict(env, SKILLHUB_UPGRADE_PHASE=name)
            app = subprocess.Popen([str(binary(installed))], env=phase_env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            processes.append(app)
            try: app.wait(timeout=90)
            except subprocess.TimeoutExpired:
                app.kill(); app.wait(); raise RuntimeError("native upgrade phase timed out: " + name)
            if not output.is_file(): raise RuntimeError("App exited without an upgrade report: " + name)
            result = json.loads(output.read_text(encoding="utf-8"))
            evidence["phases"].append({"phase": name, "binary_sha256": app_hash(installed), **result})
            if app.returncode or not result.get("passed"):
                raise RuntimeError("upgrade phase failed: " + name + ": " + result.get("error", "unknown"))
            # The independent process must read the same real system credentials.
            for alias in ("company", "partner"):
                identity = json.loads(client("whoami", "--source", alias).stdout)
                if identity.get("identity", {}).get("username") != "admin": raise RuntimeError("independent CLI identity changed")
        try:
            evidence["baseline_sha256"], evidence["candidate_sha256"] = app_hash(baseline), app_hash(candidate)
            if evidence["baseline_sha256"] == evidence["candidate_sha256"]:
                raise RuntimeError("baseline and candidate must be distinct binaries")
            if sys.platform == "darwin":
                for app in (baseline, candidate):
                    with (app / "Contents/Info.plist").open("rb") as f: info = plistlib.load(f)
                    if info.get("CFBundleIdentifier") != "net.silkage.skillhub.smoke":
                        raise RuntimeError("use isolated native-smoke bundles, never the installed production App")
            accounts = []
            for alias in ("company", "partner"):
                hub_data, password = root / alias, secrets.token_urlsafe(24)
                run([server, "init", "--data", hub_data, "--password-stdin"], input=(password + "\n").encode())
                origin = f"http://127.0.0.1:{free_port()}"
                hub = subprocess.Popen([str(server), "serve", "--data", str(hub_data), "--listen", origin[7:], "--public-url", origin, "--private=true"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                processes.append(hub)
                for _ in range(150):
                    try:
                        with urlopen(origin + "/healthz", timeout=1): break
                    except OSError:
                        if hub.poll() is not None: raise RuntimeError("isolated Hub exited")
                        time.sleep(.1)
                else: raise RuntimeError("isolated Hub timed out")
                client("source", "add", alias, origin)
                if alias == "company":
                    # CLI stores a temporary session in the real OS store; App replaces it with its own login.
                    client("login", alias, "--username", "admin", "--password-stdin", password=password+"\n")
                    client("admin", "namespace-put", "team", "--source", alias, "--review-required=false")
                    package = root / "package"; package.mkdir()
                    (package / "SKILL.md").write_text("---\nname: upgrade-check\ndescription: Isolated upgrade fixture.\n---\nPreserve these files.\n")
                    client("publish", str(package), "--source", alias, "--namespace", "team", "--version", "1.0.0")
                accounts.append({"alias":alias,"password":password})
            account_file = data / "upgrade-accounts.json"
            account_file.write_text(json.dumps(accounts)); account_file.chmod(0o600)
            replace(baseline)
            phase("seed")
            account_file.unlink()  # Subsequent phases cannot silently log in again.
            phase("baseline")
            # Back up closed state before replacement; no live SQLite copy is used.
            shutil.copytree(data, root / "client-before-upgrade")
            if sys.platform == "darwin":
                stage = root / "disk-image"; stage.mkdir()
                run(["ditto", candidate, stage / candidate.name])
                (stage / "Applications").symlink_to("/Applications")
                image, mount = root / "candidate.dmg", root / "mounted"
                mount.mkdir()
                run(["hdiutil", "create", "-volname", "SkillHub Upgrade Fixture", "-srcfolder", stage, "-format", "UDZO", image])
                run(["hdiutil", "verify", image])
                run(["hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, image])
                try: replace(mount / candidate.name)
                finally: run(["hdiutil", "detach", mount])
                evidence["dmg_verified_and_replaced"] = True
            else: replace(candidate)
            phase("candidate")
            phase("restart")
            replace(baseline)
            phase("rollback")
            evidence["passed"] = True
        finally:
            # Only these random fixture credential references are removed.
            cleanup_errors = []
            for alias in ("company", "partner"):
                try: client("logout", alias)
                except (subprocess.CalledProcessError, subprocess.TimeoutExpired): cleanup_errors.append(alias)
            for process in reversed(processes):
                if process.poll() is None:
                    process.terminate()
                    try: process.wait(timeout=10)
                    except subprocess.TimeoutExpired: process.kill(); process.wait()
            evidence["credential_cleanup_passed"] = not cleanup_errors
            if cleanup_errors: evidence["passed"] = False
            report_path.parent.mkdir(parents=True, exist_ok=True)
            report_path.write_text(json.dumps(evidence, indent=2) + "\n")
            if cleanup_errors: raise RuntimeError("fixture credential cleanup failed for: " + ", ".join(cleanup_errors))
    return evidence


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    baseline = parser.add_mutually_exclusive_group(required=True)
    baseline.add_argument("--baseline-app", type=Path)
    baseline.add_argument("--baseline-ref", help="build a comparison commit with test-only instrumentation")
    for name in ("candidate-app", "server", "cli", "report"):
        parser.add_argument("--" + name, type=Path, required=True)
    args = parser.parse_args()
    paths = [args.candidate_app.resolve(), args.server.resolve(), args.cli.resolve(), args.report.resolve()]
    if args.baseline_ref:
        with build_baseline(args.baseline_ref) as (app, provenance):
            exercise(app, *paths, provenance=provenance)
    else:
        exercise(args.baseline_app.resolve(), *paths)
    print("PASS: native App overwrite upgrade, OS credentials, drafts, preferences, project/global skills and compatible rollback")
