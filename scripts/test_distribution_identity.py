"""Keep package generation and user-facing distribution on the same repository."""
import json
from pathlib import Path
import re
import unittest
import tempfile
import plistlib
from unittest.mock import patch
import build_dmg
from project_identity import check_project_references, check_application_references

ROOT = Path(__file__).resolve().parents[1]


class DistributionIdentityTests(unittest.TestCase):
    def test_dmg_rejects_stale_executable_or_dependency_inventory_before_packaging(self):
        for target in ("executable", "inventory"):
            with self.subTest(target=target), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                app = root / "SkillHub.app"
                binary = app / "Contents/MacOS/SkillHub"
                binary.parent.mkdir(parents=True)
                binary.write_bytes(b"fixture binary")
                (app / "Contents/Info.plist").write_bytes(plistlib.dumps({"CFBundleIdentifier": "io.github.skillhubhq.skillhub"}))
                inventory = root / "desktop/bin/third-party/go-modules.json"
                inventory.parent.mkdir(parents=True)
                inventory.write_text("[]")
                (binary if target == "executable" else inventory).write_bytes(b"github.com/" + b"previous-owner" + b"/skillhub")
                output = root / "candidate.dmg"
                with patch.object(build_dmg, "ROOT", root), patch.object(build_dmg.subprocess, "run") as run:
                    with self.assertRaisesRegex(ValueError, "Noncanonical SkillHub"):
                        build_dmg.build(app, output)
                    self.assertFalse(output.exists())
                    self.assertFalse(any(call.args[0][0] == "hdiutil" for call in run.call_args_list))

    def test_repository_guard_covers_transport_and_binary_references(self):
        old = b"previous-owner" + b"/skillhub"
        for prefix in (b"https://github.com/", b"git@github.com:", b"https://api.github.com/repos/",
                       b"https://raw.githubusercontent.com/", b"https://codeload.github.com/", b"ghcr.io/"):
            for suffix in (b"", b".git", b"/releases", b"/internal/client", b"?tag=v1"):
                with self.subTest(prefix=prefix, suffix=suffix), self.assertRaises(ValueError):
                    check_project_references(b"\x00\xff" + prefix + old + suffix + b"\x00", "fixture")
        for reference in (b"https://github.com/skillhubhq/skillhub/releases", b"github.com/skillhubhq/skillhub/internal/client",
                          b"github.com/wailsapp/wails", b"https://github.com/example/skillhub-example", b"io.github.skillhubhq.skillhub"):
            check_project_references(reference, "fixture")

    def test_application_identity_rejects_unrelated_owner(self):
        for suffix in (b"", b".smoke"):
            check_application_references(b"io.github.skillhubhq.skillhub" + suffix, "fixture")
            with self.assertRaises(ValueError):
                check_application_references(b"org.previous-owner." + b"skillhub" + suffix, "fixture")

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
