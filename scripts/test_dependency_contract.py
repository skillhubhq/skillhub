"""Keep independently updated manifests compatible across the desktop bridge."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DependencyContractTests(unittest.TestCase):
    def test_wails_runtime_matches_native_library(self):
        module = (ROOT / "desktop/go.mod").read_text()
        version = re.search(r"(?m)^\s*github\.com/wailsapp/wails/v3 v(\S+)$", module)
        self.assertIsNotNone(version)
        frontend = json.loads((ROOT / "desktop/frontend/package.json").read_text())
        self.assertEqual(frontend["dependencies"]["@wailsio/runtime"], version.group(1))

    def test_shared_types_use_the_same_compiler(self):
        frontend = json.loads((ROOT / "desktop/frontend/package.json").read_text())
        sdk = json.loads((ROOT / "sdk/typescript/package.json").read_text())
        self.assertEqual(frontend["devDependencies"]["typescript"], sdk["devDependencies"]["typescript"])

    def test_reviewed_license_sources_match_shipped_versions(self):
        sources = json.loads((ROOT / "third_party/npm/sources.json").read_text())
        locks = [json.loads((ROOT / directory / "package-lock.json").read_text())
                 for directory in ["desktop/frontend", "website"]]
        for name, source in sources.items():
            with self.subTest(package=name):
                versions = {lock["packages"][f"node_modules/{name}"]["version"]
                            for lock in locks if f"node_modules/{name}" in lock["packages"]}
                self.assertEqual({source["version"]}, versions)


if __name__ == "__main__":
    unittest.main()
