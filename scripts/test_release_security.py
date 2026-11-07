import unittest
from release_security import CATEGORIES, verify


class ReleaseSecurityTests(unittest.TestCase):
    def analyses(self):
        return [{"commit_sha": "a" * 40, "ref": "refs/heads/main", "tool": {"name": "CodeQL"},
                 "category": category, "error": "", "warning": ""} for category in CATEGORIES]

    def test_requires_complete_exact_revision_and_resolved_alerts(self):
        verify("a" * 40, self.analyses(), [])
        for mutation in ("revision", "ref", "tool", "error", "warning", "missing", "alerts"):
            with self.subTest(mutation=mutation):
                analyses, alerts = self.analyses(), []
                if mutation == "revision":
                    analyses[0]["commit_sha"] = "b" * 40
                elif mutation == "ref":
                    analyses[0]["ref"] = "refs/pull/1/merge"
                elif mutation == "tool":
                    analyses[0]["tool"]["name"] = "Other"
                elif mutation in ("error", "warning"):
                    analyses[0][mutation] = "incomplete extraction"
                elif mutation == "missing":
                    analyses.pop()
                else:
                    alerts.append({"state": "open"})
                with self.assertRaises(ValueError):
                    verify("a" * 40, analyses, alerts)

    def test_failed_rerun_does_not_reuse_earlier_success(self):
        analyses = self.analyses()
        failure = dict(analyses[0], error="failed rerun")
        with self.assertRaises(ValueError):
            verify("a" * 40, [failure, *analyses], [])


if __name__ == "__main__":
    unittest.main()
