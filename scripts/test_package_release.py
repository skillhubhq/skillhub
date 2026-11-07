import json
import hashlib
from pathlib import Path
import tarfile
import tempfile
import unittest
import zipfile

from package_release import ROOT, package


class PackageReleaseTests(unittest.TestCase):
    def fixture(self, root):
        revision = "a" * 40
        for runner, goos, arch in [("ubuntu-latest", "linux", "amd64"), ("macos-latest", "darwin", "arm64"), ("windows-latest", "windows", "amd64")]:
            for kind in (["hub-cli"] if goos == "linux" else ["hub-cli", "desktop"]):
                path = root / f"{kind}-{runner}"
                (path / "third-party").mkdir(parents=True)
                (path / "third-party/LICENSE.example").write_text("Fixture license")
                (path / "build.json").write_text(json.dumps({"revision": revision, "dirty": False, "goos": goos, "goarch": arch}))
                if kind == "hub-cli":
                    for name in ["skillhub", "skillhub-server"]:
                        (path / (name + (".exe" if goos == "windows" else ""))).write_bytes(b"fixture executable")
                elif goos == "windows":
                    (path / "SkillHub.exe").write_bytes(b"fixture desktop")
                else:
                    image = path / "SkillHub-macos.dmg"
                    image.write_bytes(b"fixture disk image")
                    (path / "SkillHub-macos.dmg.json").write_text(json.dumps({"version": json.loads((ROOT / "desktop/frontend/package.json").read_text())["version"], "revision": revision, "build": "source", "sha256": hashlib.sha256(image.read_bytes()).hexdigest()}))
                    with zipfile.ZipFile(path / "SkillHub-macos.zip", "w") as z:
                        info = zipfile.ZipInfo("SkillHub.app/Contents/MacOS/SkillHub")
                        info.external_attr = 0o100755 << 16
                        z.writestr(info, b"fixture desktop")
            if goos != "linux":
                report = root / f"native-runtime-{runner}"
                report.mkdir()
                (report / "native-smoke.json").write_text(json.dumps({"passed": True, "checks": ["fixture check"]}))
        tag = "v" + json.loads((ROOT / "desktop/frontend/package.json").read_text())["version"]
        return tag, revision

    def test_packages_preserve_modes_notices_and_checksums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            tag, revision = self.fixture(root)
            dest = root / "release"
            result = package(tag, revision, "123", root, dest)
            self.assertEqual(len(result["artifacts"]), 6)
            with tarfile.open(dest / f"skillhub-{tag}-linux-amd64.tar.gz") as archive:
                self.assertEqual(archive.getmember("skillhub").mode & 0o777, 0o755)
                self.assertIn("third-party/LICENSE.example", archive.getnames())
                self.assertIn("LICENSE", archive.getnames())
            with zipfile.ZipFile(dest / f"skillhub-app-{tag}-macos-arm64.zip") as archive:
                self.assertEqual(archive.getinfo("SkillHub.app/Contents/MacOS/SkillHub").external_attr >> 16 & 0o777, 0o755)
                self.assertIn("NOTICE", archive.namelist())
            self.assertEqual(len((dest / "SHA256SUMS").read_text().splitlines()), 6)
            with self.assertRaises(FileExistsError):
                package(tag, revision, "123", root, dest)

    def test_rejects_path_and_shell_tokens_in_release_identity(self):
        for tag in ["../../escape", "v0.1.0/../../escape", "v0.1.0;echo bad", "v0.1.0$(echo bad)"]:
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                with self.assertRaises(ValueError):
                    package(tag, "a" * 40, "123", root, root / "output")
                self.assertEqual(list(root.iterdir()), [])

    def test_rejects_mismatched_source_or_failed_native_report(self):
        for failure in ["revision", "native", "dmg"]:
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                tag, revision = self.fixture(root)
                if failure == "revision":
                    revision = "b" * 40
                elif failure == "native":
                    (root / "native-runtime-macos-latest/native-smoke.json").write_text('{"passed":false}')
                else:
                    (root / "desktop-macos-latest/SkillHub-macos.dmg").write_bytes(b"modified image")
                with self.assertRaises(ValueError):
                    package(tag, revision, "123", root, root / "release")

    def test_rejects_stale_repository_in_binaries_notices_and_app_archives(self):
        stale = b"github.com/" + b"previous-owner" + b"/skillhub"
        for target in ("binary", "notices", "app-zip"):
            with self.subTest(target=target), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                tag, revision = self.fixture(root)
                if target == "binary":
                    (root / "hub-cli-ubuntu-latest/skillhub").write_bytes(b"\x00" + stale + b"\x00")
                elif target == "notices":
                    (root / "desktop-macos-latest/third-party/go-modules.json").write_bytes(stale)
                else:
                    with zipfile.ZipFile(root / "desktop-macos-latest/SkillHub-macos.zip", "w") as z:
                        z.writestr("SkillHub.app/Contents/MacOS/SkillHub", stale)
                with self.assertRaisesRegex(ValueError, "Noncanonical SkillHub"):
                    package(tag, revision, "123", root, root / "release")


if __name__ == "__main__":
    unittest.main()
