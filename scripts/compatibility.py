"""Run actual old/new binaries in isolated smoke fixtures; never use a live Hub."""
import argparse
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser()
    for option in ("server", "cli", "baseline-server", "baseline-cli"):
        parser.add_argument("--" + option, required=True)
    args = parser.parse_args()
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
