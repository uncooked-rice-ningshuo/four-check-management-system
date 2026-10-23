#!/usr/bin/env python3
"""Select a supported update path from actual container images; never guess state."""
import argparse
from pathlib import Path
import subprocess
import sys
from upgrade import run


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--base', type=Path, default=Path('/opt/four-check/ip-trial'))
    parser.add_argument('--project', default='four-check-ip-trial')
    parser.add_argument('--url', default='http://127.0.0.1:8088')
    args = parser.parse_args()
    base = args.base.resolve()
    compose = ['docker', 'compose', '--project-directory', str(base), '-p', args.project,
               '--env-file', str(base / '.env'), '-f', str(base / 'compose.yaml')]
    images = {}
    for service in ('api', 'web'):
        container = run(compose + ['ps', '-q', service]).decode().strip()
        images[service] = run(['docker', 'inspect', '--format', '{{.Config.Image}}', container]).decode().strip()
    if all(images[s] == f'four-check-{s}:ip-trial-20260922' for s in images):
        script = 'upgrade.py'
        print('检测到 20260922：先备份，再升级数据库和两个应用容器。', flush=True)
    elif all(images[s] == f'four-check-{s}:ip-trial-20261006-r1' for s in images):
        script = 'update-web.py'
        print('检测到 r1：只替换 Web 容器，不执行数据库迁移。', flush=True)
    else:
        raise RuntimeError('版本组合不属于已验证的旧版或 r1；可能已更新或上次未完成，请先核对，未修改服务。')
    command = [sys.executable, str(Path(__file__).resolve().parent / script),
               '--base', str(base), '--project', args.project, '--url', args.url]
    if args.apply:
        command.append('--apply')
    raise SystemExit(subprocess.call(command))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error))
