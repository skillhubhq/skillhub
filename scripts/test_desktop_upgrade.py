"""Reject ambiguous or option-like comparison revisions before invoking Git."""
import unittest
from unittest.mock import patch

from desktop_upgrade import build_baseline


class DesktopUpgradeTests(unittest.TestCase):
    def test_rejects_noncanonical_revision_before_running_commands(self):
        for reference in ("", "HEAD", "main", "v0.1.0", "a" * 39, "a" * 41,
                          "A" * 40, "a" * 40 + "\n", "--help", "--upload-pack=fixture",
                          "HEAD; echo fixture", "$(echo fixture)", "HEAD^{tree}"):
            with self.subTest(reference=reference), patch("desktop_upgrade.run") as run:
                with self.assertRaisesRegex(ValueError, "full lowercase commit SHA"):
                    with build_baseline(reference):
                        self.fail("invalid revision was accepted")
                run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
