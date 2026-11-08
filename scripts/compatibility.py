"""Run actual old/new binaries in isolated smoke fixtures; never use a live Hub."""
from types import SimpleNamespace
import json
import os
import secrets
import sqlite3
import tempfile
import time
from urllib.request import Request, urlopen
from urllib.error import URLError
from smoke import free_port
from pathlib import Path
import subprocess
import sys



def verify_protocol_boundary(args):
    """跨协议主版本必须拒绝连接，候选程序不得改写旧数据库。"""
    processes = []
    env = {k: v for k, v in os.environ.items() if not k.startswith("SKILLHUB_TOKEN_")}
    with tempfile.TemporaryDirectory(prefix="skillhub-boundary-") as temporary:
        root = Path(temporary)
        origins, majors = [], []
        try:
            for name, binary in (("baseline", args.baseline_server), ("candidate", args.server)):
                data = root / name
                subprocess.run([binary, "init", "--data", str(data), "--password-stdin"], input=secrets.token_urlsafe(24)+"\n", text=True, capture_output=True, check=True, env=env)
                origin = f"http://127.0.0.1:{free_port()}"
                process = subprocess.Popen([binary, "serve", "--data", str(data), "--listen", origin[7:], "--public-url", origin], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
                processes.append(process)
                for _ in range(100):
                    try:
                        request = Request(origin+"/api.registry.v1.RegistryService/GetRegistry", data=b"{}", headers={"Content-Type":"application/json", "Connect-Protocol-Version":"1"})
                        with urlopen(request, timeout=1) as response: info = json.load(response)
                        break
                    except (URLError, TimeoutError):
                        if process.poll() is not None: raise RuntimeError("fixture exited before discovery")
                        time.sleep(.1)
                else: raise RuntimeError("fixture discovery timed out")
                origins.append(origin)
                majors.append(info["protocolMajor"])
            if majors[0] == majors[1]: return False
            for name, binary, origin in (("new-to-old", args.cli, origins[0]), ("old-to-new", args.baseline_cli, origins[1])):
                result = subprocess.run([binary, "--data", str(root/name), "source", "add", "fixture", origin], capture_output=True, text=True, env=env, timeout=30)
                if result.returncode == 0 or "unsupported registry protocol" not in result.stderr:
                    raise RuntimeError("cross-protocol connection did not fail explicitly: "+name)
            processes[0].terminate(); processes[0].wait(timeout=15)
            db = root/"baseline/hub.db"
            def rows():
                with sqlite3.connect(f"file:{db}?mode=ro", uri=True) as connection: return list(connection.iterdump())
            before = rows()
            result = subprocess.run([args.server, "backup", "--data", str(root/"baseline"), "--output", str(root/"unsupported-backup")], capture_output=True, text=True, env=env, timeout=30)
            if result.returncode == 0 or "unsupported hub database schema" not in result.stderr:
                raise RuntimeError("candidate did not reject the unsupported schema")
            if before != rows(): raise RuntimeError("unsupported schema was modified")
            print(f"PASS: protocol {majors[0]} → {majors[1]} rejects both cross-major clients; unsupported database remains unchanged", flush=True)
            return True
        finally:
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                    try: process.wait(timeout=15)
                    except subprocess.TimeoutExpired: process.kill(); process.wait()


def main():
    root = Path(__file__).resolve().parents[1]
    args = SimpleNamespace(
        server=str(root / "bin/skillhub-server"),
        cli=str(root / "bin/skillhub"),
        baseline_server=str(root / ".local/compatibility-baseline/bin/skillhub-server"),
        baseline_cli=str(root / ".local/compatibility-baseline/bin/skillhub"),
    )
    if verify_protocol_boundary(args): return
    smoke = Path(__file__).with_name("smoke.py")
    matrix = [
        ("new CLI / baseline Hub", args.baseline_server, args.cli, None),
        ("baseline CLI / new Hub", args.server, args.baseline_cli, None),
        ("alternating CLIs / shared local state", args.server, args.baseline_cli, args.cli),
    ]
    for label, server, cli, alternate in matrix:
        print(label, flush=True)
        command = [sys.executable, str(smoke), "--server", server, "--cli", cli]
        if alternate:
            command += ["--alternate-cli", alternate]
        subprocess.run(command, check=True, timeout=300)
    print("baseline Hub → candidate Hub → snapshot rollback", flush=True)
    subprocess.run([sys.executable, str(smoke), "--server", args.baseline_server,
                    "--cli", args.cli, "--upgrade-server", args.server], check=True, timeout=300)
    print("PASS: compatibility matrix (baseline identity must be recorded separately)")


if __name__ == "__main__":
    main()
