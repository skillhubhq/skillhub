"""License inventory includes shipped documentation and rejects missing notices."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock
from collect_licenses import collect_npm


class LicenseInventoryTest(unittest.TestCase):
    def test_utf8_metadata_does_not_depend_on_host_encoding(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            package = root / 'node_modules/example'
            package.mkdir(parents=True)
            (package / 'package.json').write_text(json.dumps({
                'name': 'example', 'version': '1.0.0', 'license': 'MIT',
                'description': '描述',
            }, ensure_ascii=False), encoding='utf-8')
            (package / 'LICENSE').write_text('Fixture license', encoding='utf-8')
            (root / 'package-lock.json').write_text(json.dumps({'packages': {
                'node_modules/example': {'version': '1.0.0'},
            }}), encoding='utf-8')
            read_text = Path.read_text

            def legacy_locale_read(path, encoding=None, errors=None):
                return read_text(path, encoding=encoding or 'cp1252', errors=errors)

            with mock.patch.object(Path, 'read_text', new=legacy_locale_read):
                rows = collect_npm(root, root, 'npm-docs')
            self.assertEqual([row['name'] for row in rows], ['example'])
            self.assertEqual(json.loads((root / 'npm-docs-packages.json').read_text(encoding='utf-8')), rows)

    def test_static_docs_exclude_optional_native_build_tools(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            package = root / 'node_modules/native-tool'
            package.mkdir(parents=True)
            (package / 'package.json').write_text(json.dumps({'name': 'native-tool', 'version': '1.0.0', 'os': ['darwin']}))
            (root / 'package-lock.json').write_text(json.dumps({'packages': {
                'node_modules/native-tool': {'version': '1.0.0', 'optional': True},
            }}))
            self.assertEqual(collect_npm(root, root, 'npm-docs', static_assets=True), [])
            with self.assertRaisesRegex(ValueError, 'Missing reviewed license'):
                collect_npm(root, root, 'npm')

    def test_absent_optional_package_is_not_shipped(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            package = root / 'node_modules/example'
            package.mkdir(parents=True)
            (package / 'package.json').write_text(json.dumps({'name': 'example', 'version': '1.0.0', 'license': 'MIT'}))
            (package / 'LICENSE').write_text('Fixture license')
            (root / 'package-lock.json').write_text(json.dumps({'packages': {
                'node_modules/example': {'version': '1.0.0'},
                'node_modules/other-platform': {'version': '1.0.0', 'optional': True},
            }}))
            rows = collect_npm(root, root, 'npm-docs')
            self.assertEqual([row['name'] for row in rows], ['example'])
            self.assertTrue((root / 'npm-docs/example/LICENSE').is_file())
            (package / 'LICENSE').unlink()
            with self.assertRaisesRegex(ValueError, 'Missing reviewed license'):
                collect_npm(root, root, 'npm-docs')

    def test_missing_required_package_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'package-lock.json').write_text(json.dumps({'packages': {'node_modules/required': {'version': '1.0.0'}}}))
            with self.assertRaises(FileNotFoundError):
                collect_npm(root, root, 'npm-docs')
