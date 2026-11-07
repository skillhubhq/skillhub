#!/usr/bin/env python3
"""Build a local, ad-hoc-signed DMG preview; never publish or modify client data."""
import argparse
import hashlib
import json
from pathlib import Path
import plistlib
import shutil
import subprocess
import tempfile
from project_identity import check_project_references

ROOT = Path(__file__).resolve().parents[1]


def build(app, output):
    app, output = app.resolve(), output.absolute()
    if output.exists() or output.with_suffix('.dmg.json').exists():
        raise ValueError('Output already exists; choose a new filename')
    with (app / 'Contents/Info.plist').open('rb') as source:
        info = plistlib.load(source)
    if info.get('CFBundleIdentifier') != 'net.silkage.skillhub' or app.name != 'SkillHub.app':
        raise ValueError('Expected the production SkillHub.app bundle, not a fixture')
    subprocess.run(['codesign', '--verify', '--strict', str(app)], check=True)
    subprocess.run(['test', '-x', str(app / 'Contents/MacOS/SkillHub')], check=True)
    third_party = ROOT / 'desktop/bin/third-party'
    if not third_party.is_dir() or not any(third_party.iterdir()):
        raise ValueError('Run scripts/collect_licenses.py --desktop --output desktop/bin/third-party first')
    for directory in (app, third_party):
        for path in directory.rglob('*'):
            if path.is_file() and not path.is_symlink():
                check_project_references(path.read_bytes(), path.name)
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary_root = ROOT / '.local/dmg-build'
    temporary_root.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=temporary_root) as temporary:
        stage = Path(temporary) / 'image'
        stage.mkdir()
        subprocess.run(['ditto', str(app), str(stage / 'SkillHub.app')], check=True)
        (stage / 'Applications').symlink_to('/Applications', target_is_directory=True)
        for name in ['LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md']:
            shutil.copyfile(ROOT / name, stage / name)
        shutil.copytree(third_party, stage / 'third-party')
        (stage / 'Read Me.txt').write_text('''SkillHub — manual preview installation

1. Quit SkillHub before replacing an existing copy.
2. Drag SkillHub.app onto Applications. Choose Replace when updating.
3. Eject this disk image and launch SkillHub from Applications.

This preview uses local ad-hoc signing. It is not Developer ID signed or
Apple-notarized; macOS or your organization's device policy may block it.
Verify the download source and SHA-256 before installing. For a trusted
download, follow macOS Privacy & Security guidance or contact your IT team.
Do not disable system-wide security protections.

Replacing the App preserves existing connections, credentials, preferences,
drafts and installed skills outside the app bundle. Do not delete client data
folders. Follow version-specific migration notes before downgrading.
App settings → App updates checks GitHub only when requested. Downloading and
installing remain manual. A private repository may require GitHub access.

手动安装预览版：先退出 SkillHub，将 SkillHub.app 拖到 Applications；更新时选择替换。
弹出磁盘映像，再从 Applications 启动。配置和本地数据位于 App 外，替换不会删除它们。
此版本未使用 Apple Developer ID 签名或公证，系统或企业策略可能阻止首次运行。
核实来源和校验和后按系统安全设置处理；不要关闭整机安全保护。
设置 → App 更新可主动检查 GitHub，新版本仍需手动下载和安装。
''', encoding='utf-8')
        candidate = Path(temporary) / 'candidate.dmg'
        subprocess.run(['hdiutil', 'create', '-volname', 'SkillHub', '-srcfolder', str(stage), '-format', 'UDZO', str(candidate)], check=True)
        subprocess.run(['hdiutil', 'verify', str(candidate)], check=True)
        shutil.move(candidate, output)
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    manifest = {'file': output.name, 'sha256': digest, 'version': info.get('SkillHubVersion'),
                'revision': info.get('SkillHubRevision'), 'build': info.get('SkillHubBuild'),
                'developer_id_signed': False, 'notarized': False, 'installation': 'manual'}
    output.with_suffix('.dmg.json').write_text(json.dumps(manifest, indent=2) + '\n')
    output.with_suffix('.dmg.sha256').write_text(f'{digest}  {output.name}\n')
    print(json.dumps(manifest, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--app', type=Path, default=ROOT / 'desktop/bin/SkillHub.app')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    build(args.app, args.output)
