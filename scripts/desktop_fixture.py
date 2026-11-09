"""Run disposable local Hubs and the native desktop for manual acceptance."""
import argparse
from contextlib import closing
import json
import os
from pathlib import Path
import secrets
import sqlite3
import subprocess
import sys
import tempfile
import time
from urllib.request import Request, urlopen
from smoke import free_port


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--app", required=True)
    parser.add_argument("--automated", action="store_true", help="run the native_smoke build and verify its report")
    parser.add_argument("--report", help="copy the isolated native runtime report to this path")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parent.parent
    suffix = ".exe" if os.name == "nt" else ""
    server = repo / "bin" / f"skillhub-server{suffix}"
    cli_binary = repo / "bin" / f"skillhub{suffix}"
    (repo / ".local").mkdir(exist_ok=True)
    processes = []
    with tempfile.TemporaryDirectory(prefix="desktop-fixture-", dir=repo / ".local") as temporary:
        root = Path(temporary)
        env = {k: v for k, v in os.environ.items() if not k.startswith("SKILLHUB_TOKEN_")}
        env["SKILLHUB_HOME"] = str(root / "client")

        def cli(*command, stdin=None):
            result = subprocess.run([str(cli_binary), "--json", *command], input=stdin, text=True, encoding="utf-8",
                                    capture_output=True, env=env, timeout=30)
            if result.returncode:
                raise RuntimeError(f"fixture command {command[0]} failed: {result.stderr}")
            return json.loads(result.stdout)

        def login(url, username, password):
            request = Request(url + "/api.auth.v1.AuthService/Login",
                              data=json.dumps({"username": username, "password": password}).encode(),
                              headers={"Content-Type": "application/json", "Connect-Protocol-Version": "1"})
            with urlopen(request, timeout=10) as response:
                return json.load(response)["accessToken"]

        try:
            origins = {}
            for alias in ("company", "public"):
                password = secrets.token_urlsafe(24)
                data = root / alias
                subprocess.run([str(server), "init", "--data", str(data), "--password-stdin"],
                               input=password + "\n", text=True, encoding="utf-8", capture_output=True, check=True)
                if args.automated and alias == "public":
                    # 仅在隔离 fixture 中建立已验证身份；完整 JWT/OIDC 校验由服务端测试覆盖。
                    with closing(sqlite3.connect(data / "hub.db")) as db, db:
                        for user_id in ("pending-approve", "pending-reject"):
                            db.execute("INSERT INTO users(id,username,password_hash,administrator,disabled) VALUES(?,?,?,0,1)", (user_id, user_id, b""))
                            db.execute("INSERT INTO profiles(user_id,display_name) VALUES(?,?)", (user_id, user_id))
                            db.execute("INSERT INTO external_identities(issuer,subject,user_id) VALUES(?,?,?)", ("https://id.example.com", user_id, user_id))
                            db.execute("INSERT INTO access_requests(user_id,created_at) VALUES(?,?)", (user_id, int(time.time())))
                url = f"http://127.0.0.1:{free_port()}"
                origins[alias] = url
                process = subprocess.Popen([str(server), "serve", "--data", str(data), "--listen", url[7:],
                                            "--public-url", url, "--name", "Company Skills" if alias == "company" else "Community Skills",
                                            f"--private={'true' if alias == 'company' else 'false'}"],
                                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                processes.append(process)
                for attempt in range(100):
                    try:
                        with urlopen(url + "/healthz", timeout=1) as response:
                            if response.status == 200:
                                break
                    except OSError:
                        if process.poll() is not None:
                            raise RuntimeError("fixture Hub exited")
                        time.sleep(0.1)
                else:
                    raise RuntimeError("fixture Hub readiness timed out")
                cli("source", "add", alias, url)
                env[f"SKILLHUB_TOKEN_{alias.upper()}"] = login(url, "admin", password)
                cli("admin", "namespace-put", "platform", "--source", alias,
                    f"--review-required={'true' if alias == 'company' else 'false'}")
                cli("admin", "category-put", "operations", "--source", alias, "--title", "运维与交付")
                cli("admin", "group-add", "engineering", "--source", alias)

            reviewer_password = secrets.token_urlsafe(24)
            reviewer = cli("admin", "user-add", "reviewer", "--source", "company", "--password-stdin",
                           stdin=reviewer_password + "\n")["user"]["user_id"]
            for role in ("maintainer", "reviewer"):
                cli("admin", "grant", "platform", reviewer, "--source", "company", "--role", role)
            reviewer_token = login(origins["company"], "reviewer", reviewer_password)
            company_admin = env["SKILLHUB_TOKEN_COMPANY"]
            for alias, name, description in (("company", "release-check", "发布前检查变更、依赖与回滚准备，生成清晰的验收记录。"),
                                             ("public", "incident-brief", "把排查过程整理为结构清晰的事件简报，便于团队协作。")):
                package = root / name
                package.mkdir()
                (package / "SKILL.md").write_text(
                    f"---\nname: {name}\ndescription: {description}\n---\n# {name}\n\nThis is a disposable acceptance fixture.\n\n1. Read the supplied context.\n2. Record evidence and unresolved questions.\n3. Produce a concise checklist.\n", encoding="utf-8")
                (package / "references").mkdir()
                (package / "skillhub.config.json").write_text(json.dumps({"version": 1, "variables": [
                    {"name": "EXAMPLE_REGION", "description": "Synthetic configuration fixture", "required": True, "secret": False}
                ]}), encoding="utf-8")
                (package / "references" / "checklist.md").write_text(
                    "# Review checklist\n\nConfirm the exact version, audience and rollback instructions.\n"
                    "\n<script>This is displayed as text, never executed.</script>\n", encoding="utf-8")
                submitted = cli("publish", str(package), "--source", alias, "--namespace", "platform", "--version", "1.0.0",
                                "--category", "operations", "--tag", "review", "--visibility", "members" if alias == "company" else "public")["release"]
                if alias == "company":
                    env["SKILLHUB_TOKEN_COMPANY"] = reviewer_token
                    cli("review", "approve", submitted["release_id"], "--source", "company", "--reason", "Reviewed fixture")
                    with (package / "SKILL.md").open("a", encoding="utf-8") as f:
                        f.write("\nVersion 1.1 adds a final handoff check.\n")
                        for step in range(12):
                            f.write(f"\n## Verification step {step + 1}\n\nInspect the relevant evidence, record the result and identify the owner of any remaining work.\n")
                    rejected = cli("publish", str(package), "--source", "company", "--namespace", "platform", "--version", "1.0.1",
                                   "--visibility", "members", "--category", "operations", "--tag", "review")
                    env["SKILLHUB_TOKEN_COMPANY"] = company_admin
                    cli("review", "reject", rejected["release"]["release_id"], "--source", "company", "--reason", "Disposable history cleanup fixture")
                    env["SKILLHUB_TOKEN_COMPANY"] = reviewer_token
                    cli("publish", str(package), "--source", "company", "--namespace", "platform", "--version", "1.1.0",
                        "--visibility", "members", "--category", "operations", "--tag", "review",
                        "--display-name-en", "Release Readiness", "--display-name-zh", "发布就绪检查", "--icon", "workflow")
                    env["SKILLHUB_TOKEN_COMPANY"] = company_admin
            project = root / "demo-project"
            project.mkdir()
            cli("project", "init", "--project", str(project))
            for tool, coordinate in (("codex", "company/platform/release-check@1.0.0"),
                                     ("claude-code", "public/platform/incident-brief@1.0.0")):
                cli("project", "add", coordinate, "--tool", tool, "--project", str(project))
            cli("project", "lock", "--project", str(project))
            if args.automated:
                env["SKILLHUB_TOKEN_LIMITED"] = reviewer_token
                env["SKILLHUB_NATIVE_SMOKE"] = "1"
                # The Go guard compares exact bytes; Windows text mode would write CRLF.
                (root / "client" / ".native-smoke-fixture").write_bytes(b"skillhub-native-smoke-v1\n")
            print(json.dumps({"fixture_root": str(root), "project": str(project), "hubs": origins}, indent=2), flush=True)
            app = subprocess.Popen([str(Path(args.app).resolve())], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            processes.append(app)
            app.wait(timeout=120 if args.automated else None)
            if args.automated:
                report_file = root / "client" / "native-smoke.json"
                if not report_file.is_file():
                    raise RuntimeError(f"native App exited {app.returncode} without a runtime report")
                report = json.loads(report_file.read_text(encoding="utf-8"))
                if args.report:
                    destination = Path(args.report).resolve()
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
                expected_platform = "windows" if os.name == "nt" else sys.platform
                if app.returncode or not report.get("passed") or report.get("platform") != expected_platform or len(report.get("checks", [])) < 20:
                    raise RuntimeError(f"native runtime acceptance failed: {report.get('error', 'incomplete report')}")
                if "Native draft editor verification." in (root / "incident-brief" / "SKILL.md").read_text(encoding="utf-8"):
                    raise RuntimeError("native draft editor modified the original package")
                installed = cli("list")
                if len(installed) != 1 or installed[0]["name"] != "release-check" or installed[0]["version"] != "1.1.0":
                    raise RuntimeError("CLI did not observe the native runtime's final installation")
                if cli("cache", "info")["entries"] != 0:
                    raise RuntimeError("CLI did not observe native cache cleanup")
                print(f"PASS: native {report['platform']} WebView, {len(report['checks'])} bridge/product checks, independent CLI verification", flush=True)
        except KeyboardInterrupt:
            pass
        finally:
            for process in reversed(processes):
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)


if __name__ == "__main__":
    main()
