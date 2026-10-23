#!/usr/bin/env python3
"""Upgrade the verified September IP trial, with a consistent private backup.

Default mode is read-only preflight. Run the copy inside a verified release
directory; --apply explicitly starts the maintenance window. A failed migration
is never followed by an automatic downgrade or by resuming the old application.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import time
import urllib.request


def run(args, **kwargs):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    if result.returncode:
        # Commands may involve credentials through Compose; never echo their output.
        raise RuntimeError(f"命令执行失败（退出码 {result.returncode}）：{args[0]} {args[1]}")
    return result.stdout


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(chunk)
    return value.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--base', type=Path, default=Path('/opt/four-check/ip-trial'))
    parser.add_argument('--project', default='four-check-ip-trial')
    parser.add_argument('--url', default='http://127.0.0.1:8088')
    args = parser.parse_args()
    os.umask(0o077)
    release = Path(__file__).resolve().parent
    base = args.base.resolve()
    if args.apply:
        # Keep the descriptor alive until process exit; unlinking a lock can allow races.
        lock = (base / '.upgrade.lock').open('a')
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('已有升级操作正在运行')
    for line in (release / 'SHA256SUMS').read_text().splitlines():
        expected, name = line.split('  ', 1)
        path = (release / name).resolve()
        if not path.is_relative_to(release) or digest(path) != expected:
            raise RuntimeError('更新包校验失败')
    manifest = json.loads((release / 'release.json').read_text())
    compose = ['docker', 'compose', '--project-directory', str(base), '-p', args.project,
               '--env-file', str(base / '.env'), '-f', str(base / 'compose.yaml')]
    config = json.loads(run(compose + ['config', '--format', 'json']))
    ids = {}
    original_images = {}
    for service in ('postgres', 'minio', 'api', 'web'):
        ids[service] = run(compose + ['ps', '-q', service]).decode().strip()
        if not ids[service] or '\n' in ids[service]:
            raise RuntimeError(f'{service} 不是唯一运行中的容器')
        info = json.loads(run(['docker', 'inspect', ids[service]]))[0]
        original_images[service] = {'image': info['Config']['Image'], 'imageId': info['Image']}
        if info['Config']['Labels'].get('com.docker.compose.project') != args.project:
            raise RuntimeError('编排身份不匹配')
        if info['State'].get('Health', {}).get('Status') != 'healthy':
            raise RuntimeError(f'{service} 尚未健康，请先排查')
        if service in ('postgres', 'minio'):
            target = '/data' if service == 'minio' else '/var/lib/postgresql/data'
            mounts = [m for m in info['Mounts'] if m['Destination'] == target]
            if len(mounts) != 1 or mounts[0].get('Name') != f'{args.project}_{service}_data':
                raise RuntimeError(f'{service} 数据卷不符合已确认部署')
        if service in ('api', 'web'):
            old = f'four-check-{service}:ip-trial-20260922'
            if info['Config']['Image'] != old or config['services'][service]['image'] != old:
                raise RuntimeError('此包只用于已确认的 20260922 版本升级')
    for image, expected_ids in manifest['images'].items():
        # Docker 29's containerd store identifies a platform manifest; Docker 27's
        # classic store identifies its config blob and has no --platform flag.
        try:
            raw = run(['docker', 'image', 'inspect', '--platform', 'linux/amd64', image])
        except RuntimeError:
            raw = run(['docker', 'image', 'inspect', image])
        info = json.loads(raw)[0]
        if info['Id'] not in expected_ids or info['Architecture'] != 'amd64' or info['Os'] != 'linux':
            raise RuntimeError('新版镜像身份或架构不符，请先 docker load')
    for name, expected in manifest['baselineFiles'].items():
        if digest(base / name) != expected:
            raise RuntimeError(f'旧部署文件已改变，需人工核对：{name}')
    def sql(query):
        return run(['docker', 'exec', ids['postgres'], 'psql', '-U', 'four_check',
                    '-d', 'four_check', '-v', 'ON_ERROR_STOP=1', '-Atqc', query]).decode().strip()
    expected_versions = sorted(Path(n).name for n in manifest['baselineFiles'] if n.startswith('migrations/'))
    if sql('SELECT version FROM schema_migrations ORDER BY version').splitlines() != expected_versions:
        raise RuntimeError('迁移账本不是已确认的 001–013，请勿重试迁移，先核对现状')
    db_size = int(sql('SELECT pg_database_size(current_database())'))
    file_size = int(run(['docker', 'exec', ids['minio'], 'du', '-sk', '/data']).split()[0]) * 1024
    if shutil.disk_usage(base).free < 3 * 1024**3 + 2 * (db_size + file_size):
        raise RuntimeError('磁盘余量不足：需要 3 GiB 加两倍数据体积')
    original = (base / 'compose.yaml').read_text()
    updated = original
    for service in ('api', 'web'):
        old = f'four-check-{service}:ip-trial-20260922'
        if updated.count(old) != 1:
            raise RuntimeError('镜像引用不唯一，需人工核对配置')
        target = manifest.get('targetImages', {}).get(service, f'four-check-{service}:' + manifest['tag'])
        if target not in manifest['images']:
            raise RuntimeError('目标镜像未包含在已校验清单中')
        updated = updated.replace(old, target)
    print('预检通过：原版本、数据卷、001–013 账本、镜像、配置和磁盘已核对。', flush=True)
    if not args.apply:
        print('未修改服务。执行 --apply 才会停写、备份和升级。')
        return
    backup = base.parent / 'backups' / ('before-' + manifest['tag'] + '-' + time.strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(parents=True, mode=0o700, exist_ok=False)
    print('备份目录：' + str(backup), flush=True)
    (backup / 'original-images.json').write_text(json.dumps(original_images, indent=2))
    for name in ('.env', 'compose.yaml', 'nginx.conf', 'migrate.sh', 'app-policy.json'):
        shutil.copy2(base / name, backup / name)
        (backup / name).chmod(0o600)
    shutil.copytree(base / 'migrations', backup / 'migrations')
    maintenance = False
    minio_stopped = False
    try:
        maintenance = True
        run(compose + ['stop', '-t', '30', 'web', 'api'])
        with (backup / 'database.dump').open('wb') as stream:
            result = subprocess.run(['docker', 'exec', ids['postgres'], 'pg_dump', '-U', 'four_check',
                                     '-d', 'four_check', '-Fc'], stdout=stream, stderr=subprocess.PIPE)
        if result.returncode:
            raise RuntimeError('数据库备份失败')
        with (backup / 'database.dump').open('rb') as stream:
            run(['docker', 'exec', '-i', ids['postgres'], 'pg_restore', '--list'], stdin=stream)
        minio_stopped = True
        run(compose + ['stop', '-t', '30', 'minio'])
        pg_image = config['services']['postgres']['image']
        with (backup / 'minio-data.tar.gz').open('wb') as stream:
            result = subprocess.run(['docker', 'run', '--rm', '--pull=never', '--network', 'none',
                                     '--user', '0', '--entrypoint', 'tar', '--mount',
                                     f'type=volume,src={args.project}_minio_data,dst=/data,readonly',
                                     pg_image, '-C', '/data', '-czf', '-', '.'],
                                    stdout=stream, stderr=subprocess.PIPE)
        if result.returncode:
            raise RuntimeError('文件存储备份失败')
        with tarfile.open(backup / 'minio-data.tar.gz', 'r:gz') as archive:
            for member in archive:
                if member.isfile():
                    with archive.extractfile(member) as stream:
                        while stream.read(1024 * 1024):
                            pass
        with (backup / 'SHA256SUMS').open('w') as stream:
            for path in sorted(backup.rglob('*')):
                if path.is_file() and path.name != 'SHA256SUMS':
                    stream.write(f'{digest(path)}  {path.relative_to(backup)}\n')
        run(compose + ['start', 'minio'])
        minio_stopped = False
        print('数据库与文件备份校验通过；开始应用缺失迁移。', flush=True)
        for path in sorted((release / 'migrations').glob('*.sql')):
            target = base / 'migrations' / path.name
            if target.exists():
                if digest(target) != digest(path):
                    raise RuntimeError('迁移内容冲突')
            else:
                shutil.copy2(path, target)
        # A fresh one-off container is essential; the original exited job will not rerun itself.
        result = subprocess.run(compose + ['run', '--rm', '--no-deps', '-T', 'migrate'],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (backup / 'migration.log').write_bytes(result.stdout)
        if result.returncode:
            raise RuntimeError('迁移失败，应用保持停止；请检查备份目录中的 migration.log')
        versions = sorted(p.name for p in (release / 'migrations').glob('*.sql'))
        if sql('SELECT version FROM schema_migrations ORDER BY version').splitlines() != versions:
            raise RuntimeError('迁移完成账本不匹配')
        temporary = base / 'compose.yaml.upgrade-tmp'
        with temporary.open('x') as stream:
            stream.write(updated)
        temporary.replace(base / 'compose.yaml')
        run(compose + ['config', '--quiet'])
        run(compose + ['up', '-d', '--no-deps', '--pull', 'never', 'api', 'web'])
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(90):
            try:
                with opener.open(args.url.rstrip('/') + '/health/ready', timeout=3) as response:
                    ready = response.status == 200
                healthy = all(json.loads(run(['docker', 'inspect',
                    run(compose + ['ps', '-q', s]).decode().strip()]))[0]['State'].get('Health', {}).get('Status') == 'healthy'
                    for s in ('api', 'web', 'minio', 'postgres'))
                if ready and healthy:
                    print('升级启动验证通过：001–021 已应用，四个容器健康。请继续浏览器业务验收。', flush=True)
                    return
            except (OSError, RuntimeError, ValueError):
                pass
            time.sleep(2)
        raise RuntimeError('新版服务未在等待期限内就绪')
    except Exception:
        if minio_stopped:
            subprocess.run(compose + ['start', 'minio'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if maintenance:
            subprocess.run(compose + ['stop', '-t', '30', 'web', 'api'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        print('升级未完成，API/Web 保持停止。不要运行旧版本或删除数据卷。备份：' + str(backup), flush=True)
        raise


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error))
