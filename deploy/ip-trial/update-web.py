#!/usr/bin/env python3
"""Read-only preflight or an explicit r1 -> r2 Web-only update."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import urllib.request
from upgrade import digest, run


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--base', type=Path, default=Path('/opt/four-check/ip-trial'))
    parser.add_argument('--project', default='four-check-ip-trial')
    parser.add_argument('--url', default='http://127.0.0.1:8088')
    args = parser.parse_args()
    os.umask(0o077)
    base = args.base.resolve()
    release = Path(__file__).resolve().parent
    if args.apply:
        lock = (base / '.upgrade.lock').open('a')
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    for line in (release / 'SHA256SUMS').read_text().splitlines():
        expected, name = line.split('  ', 1)
        path = (release / name).resolve()
        if not path.is_relative_to(release) or digest(path) != expected:
            raise RuntimeError('更新包校验失败')
    manifest = json.loads((release / 'release.json').read_text())
    target = manifest['targetImages']['web']
    try:
        info = json.loads(run(['docker', 'image', 'inspect', '--platform', 'linux/amd64', target]))[0]
    except RuntimeError:
        info = json.loads(run(['docker', 'image', 'inspect', target]))[0]
    if info['Id'] not in manifest['images'][target] or info['Architecture'] != 'amd64' or info['Os'] != 'linux':
        raise RuntimeError('新版前端镜像不符，请先加载更新包镜像')
    compose = ['docker', 'compose', '--project-directory', str(base), '-p', args.project,
               '--env-file', str(base / '.env'), '-f', str(base / 'compose.yaml')]
    config = json.loads(run(compose + ['config', '--format', 'json']))
    ids = {}
    for service in ('api', 'web', 'postgres', 'minio'):
        ids[service] = run(compose + ['ps', '-q', service]).decode().strip()
        info = json.loads(run(['docker', 'inspect', ids[service]]))[0]
        if info['Config']['Labels'].get('com.docker.compose.project') != args.project or info['State'].get('Health', {}).get('Status') != 'healthy':
            raise RuntimeError('原编排身份或健康状态异常')
        if service in ('api', 'web'):
            old = f'four-check-{service}:ip-trial-20261006-r1'
            if info['Config']['Image'] != old or config['services'][service]['image'] != old:
                raise RuntimeError('仅支持成功部署 r1 后的前端补丁；旧版本请使用 upgrade.py')
    versions = run(['docker', 'exec', ids['postgres'], 'psql', '-U', 'four_check', '-d', 'four_check',
                    '-v', 'ON_ERROR_STOP=1', '-Atqc', 'SELECT version FROM schema_migrations ORDER BY version']).decode().splitlines()
    if versions != sorted(p.name for p in (release / 'migrations').glob('*.sql')):
        raise RuntimeError('数据库账本不是已确认的 001–021，停止前端更新')
    if shutil.disk_usage(base).free < 1024**3:
        raise RuntimeError('剩余空间不足 1 GiB')
    old = 'four-check-web:ip-trial-20261006-r1'
    original = (base / 'compose.yaml').read_text()
    if original.count(old) != 1:
        raise RuntimeError('旧前端镜像引用不唯一')
    print('预检通过：API/Web 均为 r1，账本 001–021，四个容器健康。仅更新 Web。', flush=True)
    if not args.apply:
        return
    backup = base.parent / 'backups' / ('before-web-r2-' + time.strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(parents=True, mode=0o700, exist_ok=False)
    shutil.copy2(base / 'compose.yaml', backup / 'compose.yaml')
    (backup / 'compose.yaml').chmod(0o600)
    temp = base / 'compose.yaml.web-update-tmp'
    with temp.open('x') as stream:
        stream.write(original.replace(old, target))
    temp.replace(base / 'compose.yaml')
    try:
        run(compose + ['config', '--quiet'])
        run(compose + ['up', '-d', '--no-deps', '--pull', 'never', 'web'])
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(60):
            try:
                current = run(compose + ['ps', '-q', 'web']).decode().strip()
                healthy = json.loads(run(['docker', 'inspect', current]))[0]['State'].get('Health', {}).get('Status') == 'healthy'
                with opener.open(args.url.rstrip('/') + '/health/ready', timeout=3) as response:
                    ready = response.status == 200
                if healthy and ready:
                    for service in ('api', 'postgres', 'minio'):
                        if run(compose + ['ps', '-q', service]).decode().strip() != ids[service]:
                            raise RuntimeError('其他服务容器发生了变化，请核对并发操作')
                    print('Web 已更新为 r2；API、PostgreSQL、MinIO 容器均保持原样。备份：' + str(backup), flush=True)
                    return
            except OSError:
                pass
            time.sleep(2)
        raise RuntimeError('新版 Web 未按时就绪')
    except Exception:
        print('更新未完成。原 compose.yaml 保存在 ' + str(backup) + '；不要执行数据库迁移或删除卷。', flush=True)
        raise


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error))
