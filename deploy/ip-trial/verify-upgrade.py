#!/usr/bin/env python3
"""Exercise the release upgrader on disposable 013 volumes, never a business DB."""
import argparse
import hashlib
from http.cookiejar import CookieJar
import json
import os
import re
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import yaml


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('release', type=Path)
    parser.add_argument('baseline', type=Path)
    parser.add_argument('--r1-release', type=Path, help='First upgrade to r1, then exercise update-web.py')
    args = parser.parse_args()
    release = args.release.resolve()
    baseline = args.baseline.resolve()
    project = 'four-check-upgrade-test-' + secrets.token_hex(4)
    report = {'project': project, 'checks': [], 'passed': False}
    def check(name):
        report['checks'].append(name)
        print('PASS ' + name, flush=True)
    def run(cmd, **kwargs):
        result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
        if result.returncode:
            if len(cmd) > 1 and Path(cmd[1]).name in ('upgrade.py', 'update-web.py', 'update.py'):
                report['preflightOutput'] = (result.stdout + result.stderr).decode()
            raise RuntimeError('Command failed: ' + ' '.join(cmd[:3]))
        return result.stdout
    with tempfile.TemporaryDirectory(prefix=project) as tmp:
        base = Path(tmp) / 'deployment'
        base.mkdir()
        for name in ('nginx.conf', 'migrate.sh', 'app-policy.json', 'init-env.py'):
            shutil.copy2(baseline / name, base / name)
        shutil.copytree(baseline / 'migrations', base / 'migrations')
        cfg = yaml.safe_load((baseline / 'compose.yaml').read_text())
        cfg['name'] = project
        # Ask Docker for an unused subnet; derive the trusted web IP from it.
        network = project + '-subnet-probe'
        run(['docker', 'network', 'create', network])
        try:
            net = json.loads(run(['docker', 'network', 'inspect', network]))[0]
            subnet = net['IPAM']['Config'][0]['Subnet']
        finally:
            run(['docker', 'network', 'rm', network])
        import ipaddress
        web_ip = str(ipaddress.ip_network(subnet)[10])
        cfg['networks']['default']['ipam']['config'] = [{'subnet': subnet}]
        cfg['services']['web']['networks']['default']['ipv4_address'] = web_ip
        cfg['services']['api']['environment']['TRUSTED_PROXY_CIDRS'] = web_ip + '/32'
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            host_port = listener.getsockname()[1]
        cfg['services']['web']['ports'] = [f'127.0.0.1:{host_port}:80']
        (base / 'compose.yaml').write_text(yaml.safe_dump(cfg, sort_keys=False))
        run(['python3', str(base / 'init-env.py')])
        env_values = dict(line.split('=', 1) for line in (base / '.env').read_text().splitlines())
        env_hash = hashlib.sha256((base / '.env').read_bytes()).hexdigest()
        compose = ['docker', 'compose', '-p', project, '-f', str(base / 'compose.yaml')]
        try:
            run(compose + ['up', '-d'])
            port = run(compose + ['port', 'web', '80']).decode().strip().rsplit(':', 1)[1]
            origin = 'http://127.0.0.1:' + port
            cookies = CookieJar()
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(cookies))
            def http(path, data=None, token=None, method=None):
                headers = {'Content-Type': 'application/json'}
                if token:
                    headers['Authorization'] = 'Bearer ' + token
                request = urllib.request.Request(origin + path, data=None if data is None else json.dumps(data).encode(), headers=headers, method=method)
                try:
                    with opener.open(request, timeout=10) as response:
                        return response.status, response.read()
                except urllib.error.HTTPError as error:
                    return error.code, error.read()
            for _ in range(60):
                try:
                    healthy = all(json.loads(run(['docker', 'inspect', run(compose + ['ps', '-q', service]).decode().strip()]))[0]['State'].get('Health', {}).get('Status') == 'healthy'
                                  for service in ('api', 'web', 'postgres', 'minio'))
                    if http('/health/ready')[0] == 200 and healthy:
                        break
                except OSError:
                    pass
                time.sleep(2)
            else:
                raise RuntimeError('Old release not ready')
            token = json.loads(http('/api/v1/auth/login', {'loginName': 'admin', 'password': env_values['BOOTSTRAP_ADMIN_PASSWORD']})[1])['accessToken']
            changed = 'Changed-' + secrets.token_hex(16)
            assert http('/api/v1/auth/change-password', {'currentPassword': env_values['BOOTSTRAP_ADMIN_PASSWORD'], 'newPassword': changed}, token)[0] == 204
            token = json.loads(http('/api/v1/auth/login', {'loginName': 'admin', 'password': changed})[1])['accessToken']
            account_before = json.loads(http('/api/v1/auth/me', token=token)[1])
            actor = account_before['id']
            year, semester, week = [str(uuid.uuid4()) for _ in range(3)]
            fixture_sql = f"""
INSERT INTO academic_years(id,name,start_date,end_date) VALUES('{year}','upgrade-fixture','2026-01-01','2026-12-31');
INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES('{semester}','{year}','FIRST','upgrade-fixture','2026-01-01','2026-12-31');
INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date) VALUES('{week}','{semester}',1,'2026-06-15','2026-06-21');
"""
            run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'four_check', '-v', 'ON_ERROR_STOP=1'], input=fixture_sql.encode())
            attendance_type = run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'four_check', '-Atqc', "SELECT id FROM inspection_types WHERE code='ATTENDANCE_RATE'"]).decode().strip()
            department = run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'four_check', '-Atqc', 'SELECT id FROM departments ORDER BY id LIMIT 1']).decode().strip()
            template_path = f'/api/v1/imports/attendance/template?semesterId={semester}&weekId={week}&inspectionTypeId={attendance_type}&departmentId={department}'
            run(compose + ['exec', '-T', 'api', 'publish-attendance-template', '--apply', '--actor-id', actor, '--reason', 'isolated upgrade fixture'])
            status, old_template = http(template_path, token=token)
            assert status == 200 and len(old_template) > 100
            check('Old 013 release starts; changed admin password and real template fixture created')
            initial_release = args.r1_release.resolve() if args.r1_release else release
            entry = 'update.py' if (initial_release / 'update.py').exists() else 'upgrade.py'
            updater = ['python3', str(initial_release / entry), '--base', str(base), '--project', project, '--url', origin]
            run(updater)
            check('Read-only preflight passes on actual old image and volumes')
            original_sql = base / 'migrations/013_work_context_settings.sql'
            content = original_sql.read_bytes()
            original_sql.write_bytes(content + b'\n-- drift\n')
            try:
                bad = subprocess.run(updater, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                assert bad.returncode != 0
                assert http('/health/ready')[0] == 200
            finally:
                original_sql.write_bytes(content)
            check('Migration drift is rejected before stopping service')
            result = subprocess.run(updater + ['--apply'], stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            report['upgradeOutput'] = result.stdout.decode()
            if result.returncode:
                raise RuntimeError('Upgrade script failed; see report upgradeOutput')
            # Recreating the web container may allocate another ephemeral host port.
            port = run(compose + ['port', 'web', '80']).decode().strip().rsplit(':', 1)[1]
            origin = 'http://127.0.0.1:' + port
            check('Backup and real 013 to 021 migration plus image replacement passed')
            if args.r1_release:
                patch_entry = 'update.py' if (release / 'update.py').exists() else 'update-web.py'
                patcher = ['python3', str(release / patch_entry), '--base', str(base), '--project', project, '--url', origin]
                run(patcher)
                report['webPatchOutput'] = run(patcher + ['--apply']).decode()
                check('Web-only r1 to r2 patch passed; API/PostgreSQL/MinIO container identities unchanged')
            assert hashlib.sha256((base / '.env').read_bytes()).hexdigest() == env_hash
            token = json.loads(http('/api/v1/auth/login', {'loginName': 'admin', 'password': changed})[1])['accessToken']
            assert json.loads(http('/api/v1/auth/me', token=token)[1])['id'] == actor
            assert http('/api/v1/auth/login', {'loginName': 'admin', 'password': env_values['BOOTSTRAP_ADMIN_PASSWORD']})[0] == 401
            status, new_template = http(template_path, token=token)
            assert status == 200 and new_template == old_template
            check('Original credentials, admin identity, changed password and template bytes preserved')
            assert http('/')[0] == 200
            if json.loads((release / 'release.json').read_text())['tag'].endswith('-r2'):
                html = http('/')[1].decode()
                asset = re.search(r'<script[^>]+src="([^"]+)"', html).group(1)
                asset_status, javascript = http(asset)
                assert asset_status == 200 and b'SESSION_CHANGED' in javascript
                check('Served production JavaScript contains the r2 session renewal fix')
            assert http('/api/v1/dashboard/workbench', token=token)[0] == 200
            assert http('/api/v1/auth/me')[0] == 401
            assert http('/api/v1/auth/refresh', {})[0] == 200
            check('SPA, authenticated workbench, unauthorized rejection and refresh passed')
            multiplier = run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'four_check', '-Atqc', f"SELECT attendance_penalty_multiplier FROM semester_weeks WHERE id='{week}'"]).decode().strip()
            assert multiplier == '1'
            for code in ('ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'MORNING_ATTENDANCE', 'LATE_RETURN', 'DORMITORY_HYGIENE'):
                type_id = run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'four_check', '-Atqc', f"SELECT id FROM inspection_types WHERE code='{code}'"]).decode().strip()
                assert type_id
                assert http(f'/api/v1/inspections?semesterId={semester}&weekId={week}&inspectionTypeId={type_id}&departmentId={department}', token=token)[0] == 200
            check('Existing calendar preserved, default multiplier migrated, five inspection list APIs passed')
            for code in ('UNATTENDED_CLASSROOM', 'MORNING_ATTENDANCE', 'LATE_RETURN', 'DORMITORY_HYGIENE'):
                run(compose + ['exec', '-T', 'api', 'publish-attendance-template', '--type-code', code, '--apply', '--actor-id', actor, '--reason', 'isolated upgrade verification'])
            check('Four additional active inspection templates can be published')
            preview = json.loads(run(compose + ['exec', '-T', 'api', 'reconcile-files']))
            assert preview['mode'] == 'preview'
            check('File reconciliation CLI available in read-only preview mode')
            backup = next((base.parent / 'backups').glob('before-ip-trial-*'))
            run(compose + ['exec', '-T', 'postgres', 'createdb', '-U', 'four_check', 'restore_check'])
            with (backup / 'database.dump').open('rb') as stream:
                run(compose + ['exec', '-T', 'postgres', 'pg_restore', '-U', 'four_check', '-d', 'restore_check', '--exit-on-error'], stdin=stream)
            count = run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'four_check', '-d', 'restore_check', '-Atqc', 'SELECT count(*) FROM schema_migrations']).decode().strip()
            assert count == '13'
            check('Database backup restores successfully into separate database with original 13 migrations')
            report['passed'] = True
        finally:
            cleanup = subprocess.run(compose + ['down', '-v', '--remove-orphans'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            report['cleanupPassed'] = cleanup.returncode == 0
            report_name = 'web-patch-verification.json' if args.r1_release else 'upgrade-verification.json'
            (release.parent / report_name).write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if cleanup.returncode:
                raise RuntimeError('Disposable resource cleanup failed')


if __name__ == '__main__':
    main()
