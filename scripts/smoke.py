"""Exercise the built Hub and CLI executables with two isolated local registries."""

import argparse
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
from urllib.error import URLError
from urllib.request import Request, urlopen


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", required=True)
    parser.add_argument("--cli", required=True)
    parser.add_argument("--alternate-cli", help="alternate two CLI versions against the same local state")
    args = parser.parse_args()
    server_binary = str(Path(args.server).resolve())
    cli_binary = str(Path(args.cli).resolve())
    processes = []
    with tempfile.TemporaryDirectory(prefix="skillhub-smoke-") as temporary:
        root = Path(temporary)
        environment = os.environ.copy()
        # The test never inherits credentials belonging to the developer.
        for key in list(environment):
            if key.startswith("SKILLHUB_TOKEN_"):
                del environment[key]

        calls = 0
        def cli(*command, stdin=None, expected=0):
            nonlocal calls
            calls += 1
            executable = str(Path(args.alternate_cli).resolve()) if args.alternate_cli and calls % 2 == 0 else cli_binary
            result = subprocess.run(
                [executable, "--data", str(root / "client"), "--json", *command],
                input=stdin,
                text=True, encoding="utf-8",
                capture_output=True,
                env=environment,
                timeout=60,
            )
            if result.returncode != expected:
                raise RuntimeError(f"CLI {command[0]} returned {result.returncode}: {result.stderr}")
            return json.loads(result.stdout)

        def login(origin, username, password):
            request = Request(
                origin + "/api.auth.v1.AuthService/Login",
                data=json.dumps({"username": username, "password": password}).encode(),
                headers={"Content-Type": "application/json", "Connect-Protocol-Version": "1"},
            )
            with urlopen(request, timeout=10) as response:
                return json.load(response)["accessToken"]

        def start_hub(data, origin, private):
            process = subprocess.Popen(
                [server_binary, "serve", "--data", str(data), "--listen", origin.removeprefix("http://"),
                 "--public-url", origin, f"--private={'true' if private else 'false'}"],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            )
            processes.append(process)
            deadline = time.monotonic() + 15
            while True:
                if process.poll() is not None:
                    raise RuntimeError("Hub exited before becoming ready")
                try:
                    with urlopen(origin + "/healthz", timeout=1) as response:
                        if response.read() == b"ok\n":
                            return process
                except (URLError, TimeoutError):
                    pass
                if time.monotonic() > deadline:
                    raise RuntimeError("Hub readiness timed out")
                time.sleep(0.1)

        def maintenance(*command):
            result = subprocess.run([server_binary, *command], text=True, encoding="utf-8", capture_output=True, timeout=60)
            if result.returncode:
                raise RuntimeError(f"Hub {command[0]} failed: {result.stderr}")

        try:
            origins = {}
            for alias in ("public", "company"):
                password = secrets.token_urlsafe(24)
                data = root / alias
                subprocess.run(
                    [server_binary, "init", "--data", str(data), "--password-stdin"],
                    input=password + "\n", text=True, encoding="utf-8", check=True, capture_output=True,
                )
                origin = f"http://127.0.0.1:{free_port()}"
                origins[alias] = origin
                start_hub(data, origin, alias == "company")
                cli("source", "add", alias, origin)
                environment[f"SKILLHUB_TOKEN_{alias.upper()}"] = login(origin, "admin", password)
                cli("admin", "namespace-put", "team", "--source", alias,
                    f"--review-required={'false' if alias == 'public' else 'true'}")

            reviewer_password = secrets.token_urlsafe(24)
            reviewer = cli("admin", "user-add", "reviewer", "--source", "company", "--password-stdin",
                           stdin=reviewer_password + "\n")["user"]["user_id"]
            cli("admin", "grant", "team", reviewer, "--source", "company", "--role", "reviewer")
            package = root / "package"
            package.mkdir()
            content = "---\nname: smoke-check\ndescription: Verify the packaged executables.\n---\nTest fixture only.\n"
            (package / "SKILL.md").write_text(content, encoding="utf-8")
            cli("publish", str(package), "--source", "public", "--namespace", "team", "--version", "1.0.0", "--visibility", "public")
            pending = cli("publish", str(package), "--source", "company", "--namespace", "team", "--version", "1.0.0")["release"]
            assert pending["status"] == "RELEASE_STATUS_PENDING"
            environment["SKILLHUB_TOKEN_COMPANY"] = login(origins["company"], "reviewer", reviewer_password)
            cli("review", "approve", pending["release_id"], "--source", "company", "--reason", "Verified test fixture")
            del environment["SKILLHUB_TOKEN_PUBLIC"]
            results = cli("search", "smoke-check")
            assert len(results) == 2 and all(len(result["skills"]) == 1 for result in results)
            project = root / "project"
            project.mkdir()
            cli("install", "company/team/smoke-check@1.0.0", "--tool", "codex", "--project", str(project), "--yes")
            cli("install", "public/team/smoke-check@1.0.0", "--tool", "claude-code", "--project", str(project), "--yes")
            installed = cli("list")
            assert len(installed) == 2
            cache = cli("cache", "info")
            assert cache["entries"] == 2 and 0 < cache["bytes"] <= cache["limit_bytes"]
            for item in installed:
                assert (Path(item["target"]) / "SKILL.md").read_text(encoding="utf-8") == content
            cli("project", "init", "--project", str(project))
            for tool, alias in (("codex", "company"), ("claude-code", "public")):
                cli("project", "add", f"{alias}/team/smoke-check@1.0.0", "--tool", tool, "--project", str(project))
            lock = cli("project", "lock", "--project", str(project))
            assert len(lock["skills"]) == 2
            restored = root / "restored-project"
            restored.mkdir()
            for name in ("skillhub.json", "skillhub.lock.json"):
                shutil.copyfile(project / name, restored / name)
            plan = cli("project", "restore", "--project", str(restored), "--dry-run")
            assert len(plan["plans"]) == 2 and not (restored / ".agents").exists()
            results = cli("project", "restore", "--project", str(restored), "--yes")
            assert len(results) == 2 and all(result["applied"] for result in results)
            for directory in (".agents", ".claude"):
                assert (restored / directory / "skills" / "smoke-check" / "SKILL.md").read_text(encoding="utf-8") == content
            installed = cli("list")
            assert len(installed) == 4
            backup = root / "company-backup"
            recovered = root / "company-recovered"
            maintenance("backup", "--data", str(root / "company"), "--output", str(backup))
            maintenance("verify-backup", "--input", str(backup))
            maintenance("restore", "--input", str(backup), "--data", str(recovered))
            processes[1].terminate()
            processes[1].wait(timeout=10)
            partial = cli("search", "smoke-check", expected=1)
            assert partial[0]["source"] == "company" and partial[0]["error"]
            assert partial[1]["source"] == "public" and len(partial[1]["skills"]) == 1
            start_hub(recovered, origins["company"], True)
            revoked = cli("search", "smoke-check", expected=1)
            assert revoked[0]["error"] and len(revoked[1]["skills"]) == 1
            environment["SKILLHUB_TOKEN_COMPANY"] = login(origins["company"], "reviewer", reviewer_password)
            results = cli("search", "smoke-check")
            assert all(len(result["skills"]) == 1 for result in results)
            recovery_project = root / "recovery-project"
            recovery_project.mkdir()
            cli("install", "company/team/smoke-check@1.0.0", "--tool", "codex", "--project", str(recovery_project), "--yes")
            assert (recovery_project / ".agents" / "skills" / "smoke-check" / "SKILL.md").read_text(encoding="utf-8") == content
            installed = cli("list")
            assert len(installed) == 5
            assert cli("cache", "info")["entries"] == 2
            assert cli("cache", "clear", "--yes")["entries"] == 0
            assert len(cli("list")) == 5
            for item in installed:
                cli("remove", item["target"], "--yes")
            assert cli("list") == []
            print("PASS: two built Hubs, independent identities, review, two tool installations, locked project restore, shared cache/cleanup, online backup/recovery with reauthentication, partial failure, removal")
        finally:
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)


if __name__ == "__main__":
    main()
