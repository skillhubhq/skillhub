"""Reject stale first-party repository references in source and build inputs."""
import re

REPOSITORY = b"skillhubhq/skillhub"
REFERENCE = re.compile(
    rb"(?:github\.com[/:]|api\.github\.com/repos/|raw\.githubusercontent\.com/|"
    rb"codeload\.github\.com/|ghcr\.io/)([a-z0-9_.-]+/skillhub)(?:\.git)?(?=[^a-z0-9_.-]|$)",
    re.IGNORECASE,
)


def check_project_references(data, label):
    if any(match.group(1).lower() != REPOSITORY for match in REFERENCE.finditer(data)):
        raise ValueError(f"Noncanonical SkillHub repository reference in {label}; use skillhubhq/skillhub")
