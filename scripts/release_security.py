"""Fail closed unless the exact candidate has complete, clean CodeQL evidence."""
import json
import os
import re
import subprocess

CATEGORIES = {"/language:" + name for name in ("go", "javascript-typescript", "python", "actions")}


def verify(revision, analyses, alerts):
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Invalid candidate revision")
    # Responses are newest-first: a later failed rerun must not reuse an earlier success.
    latest = {}
    for analysis in analyses:
        if (analysis.get("commit_sha") == revision and analysis.get("ref") == "refs/heads/main"
                and analysis.get("tool", {}).get("name") == "CodeQL"):
            latest.setdefault(analysis.get("category"), analysis)
    if not CATEGORIES.issubset(latest):
        raise ValueError("Candidate is missing complete main-branch CodeQL evidence")
    if any(latest[category].get("error") or latest[category].get("warning") for category in CATEGORIES):
        raise ValueError("Candidate CodeQL analysis reported an error or warning")
    if alerts:
        raise ValueError("Resolve or individually review open code-scanning alerts before preparing a release")


def main():
    repo, revision = os.environ["GH_REPO"], os.environ["REVISION"]

    def api(path):
        return json.loads(subprocess.check_output(["gh", "api", f"repos/{repo}/{path}"], timeout=60))

    # Bounded history: older candidates without sufficient evidence must be rescanned.
    analyses = api("code-scanning/analyses?ref=refs/heads/main&per_page=100")
    alerts = api("code-scanning/alerts?state=open&per_page=1")
    verify(revision, analyses, alerts)
    print("Verified exact-revision CodeQL evidence and no open code-scanning alerts")


if __name__ == "__main__":
    main()
