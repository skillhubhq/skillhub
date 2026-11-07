"""Keep package generation and user-facing distribution on the same repository."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DistributionIdentityTests(unittest.TestCase):
    def test_modules_codegen_and_sdk_share_the_owner(self):
        module = re.search(r"^module (\S+)", (ROOT / "go.mod").read_text()).group(1)
        self.assertEqual(module, "github.com/skillhubhq/skillhub")
        desktop = (ROOT / "desktop/go.mod").read_text()
        self.assertIn(f"module {module}/desktop\n", desktop)
        self.assertRegex(desktop, rf"(?m)^\s*{re.escape(module)} v0\.0\.0$")
        self.assertIn(f"replace {module} => ..", desktop)
        self.assertIn(f"value: {module}/gen", (ROOT / "buf.gen.yaml").read_text())
        package = json.loads((ROOT / "sdk/typescript/package.json").read_text())
        lock = json.loads((ROOT / "sdk/typescript/package-lock.json").read_text())
        self.assertEqual(package["name"], "@skillhubhq/skillhub-protocol")
        self.assertEqual(lock["name"], package["name"])
        self.assertEqual(lock["packages"][""]["name"], package["name"])

    def test_web_downloads_and_app_updates_use_the_same_repository(self):
        module = re.search(r"^module (\S+)", (ROOT / "go.mod").read_text()).group(1)
        project = "https://" + module
        downloads = (ROOT / "desktop/frontend/src/Downloads.tsx").read_text()
        self.assertIn(f'export const projectURL = "{project}";', downloads)
        self.assertIn('export const releasesURL = `${projectURL}/releases`;', downloads)
        updater = (ROOT / "internal/appupdate/check.go").read_text()
        self.assertIn(f'const ReleasesURL = "{project}/releases"', updater)
        owner_repo = module.removeprefix("github.com/")
        self.assertIn(f'"https://api.github.com/repos/{owner_repo}/releases?per_page=100"', updater)


if __name__ == "__main__":
    unittest.main()
