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


APPLICATION_ID = b"io.github.skillhubhq.skillhub"
APPLICATION_REFERENCE = re.compile(rb"(?:[a-z0-9-]+\.)+skillhub(?:\.smoke)?(?=[^a-z0-9_.-]|$)", re.IGNORECASE)

def check_application_references(data, label):
    if any(match.group(0).lower() not in (APPLICATION_ID, APPLICATION_ID + b".smoke")
           for match in APPLICATION_REFERENCE.finditer(data)):
        raise ValueError(f"Noncanonical SkillHub application identifier in {label}")
