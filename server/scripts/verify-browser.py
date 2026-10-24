#!/usr/bin/env python3
"""Run browser regressions on disposable services; never read the business .env.

Requires PostgreSQL 16 via Docker (default) or disposable native binaries
(BROWSER_DB_MODE=native), plus Go, Node 24, MinIO, mc and Google Chrome.
PLAYWRIGHT_CLI_JS may select a preinstalled official CLI; otherwise npx uses the
pinned CLI version. Only sanitized results/screenshots remain in output/playwright.
"""
import datetime
import base64
import hmac
import json
import uuid
import zipfile
import xml.etree.ElementTree as ET
import hashlib
import os
from pathlib import Path
import secrets
import signal
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
RUN_ID = secrets.token_hex(5)
OUTPUT = ROOT / 'output' / 'playwright' / RUN_ID
OUTPUT.mkdir(parents=True)
ENV = dict(os.environ, GOTOOLCHAIN=os.environ.get('GOTOOLCHAIN', 'go1.25.0'))
DB_MODE = os.environ.get('BROWSER_DB_MODE', 'docker')
if DB_MODE not in ('docker', 'native'):
    raise SystemExit('BROWSER_DB_MODE must be docker or native')
for tool in (('docker',) if DB_MODE == 'docker' else ('initdb', 'postgres', 'pg_isready', 'createdb', 'psql')) + ('go', 'node', 'minio', 'mc'):
    if not shutil.which(tool):
        raise SystemExit(f'Missing required tool: {tool}')
CLI = ['node', ENV['PLAYWRIGHT_CLI_JS']] if ENV.get('PLAYWRIGHT_CLI_JS') else ['npx', '--yes', '--package', '@playwright/cli@0.1.21', 'playwright-cli']
CLI += ['-s=calendar-' + RUN_ID]
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))
PRIVATE_VALUES = []


def sanitize(text):
    for value in PRIVATE_VALUES + [value for key, value in ENV.items() if any(part in key for part in ('PASSWORD', 'SECRET', 'DATABASE_URL', 'MC_HOST'))]:
        if value:
            text = text.replace(value, '[redacted]')
    return text


def run(args, **kwargs):
    result = subprocess.run(args, capture_output=True, env=ENV, **kwargs)
    if result.returncode:
        diagnostic = result.stderr if isinstance(result.stderr, str) else result.stderr.decode(errors='replace')
        stdout = result.stdout if isinstance(result.stdout, str) else result.stdout.decode(errors='replace')
        if '### Error' in stdout:
            diagnostic += stdout.split('### Error', 1)[1].split('###', 1)[0]
        diagnostic = sanitize(diagnostic)
        raise RuntimeError(f'{Path(args[0]).name} failed: {diagnostic[:1500]}')
    return result


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def wait_url(url):
    for _ in range(100):
        try:
            with OPENER.open(url, timeout=1) as response:
                if response.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            time.sleep(0.2)
    raise RuntimeError('Disposable service did not become ready: ' + url)


def cli(*args, allow_error=False):
    result = run(CLI + list(args), cwd=OUTPUT, text=True)
    # The CLI can return exit 0 with a tool error. Treat its error marker as failure.
    if not allow_error and '### Error' in result.stdout:
        raise RuntimeError(result.stdout.split('### Error', 1)[1].split('###', 1)[0].strip())
    return result.stdout


processes = []
container_created = False
session_open = False
container = 'four-check-browser-' + RUN_ID
report = {'status': 'failed', 'cases': [], 'environment': f'disposable {DB_MODE} PostgreSQL + MinIO + real API + Vite + Chrome'}
with tempfile.TemporaryDirectory(prefix='four-check-browser-') as temporary:
    temp = Path(temporary)
    try:
        ENV.update(POSTGRES_PASSWORD=secrets.token_hex(24), MINIO_ROOT_USER='browser-root', MINIO_ROOT_PASSWORD=secrets.token_hex(24))
        if DB_MODE == 'docker':
            run(['docker', 'run', '--pull=never', '-d', '--rm', '--name', container, '--tmpfs', '/var/lib/postgresql/data', '-e', 'POSTGRES_PASSWORD', '-p', '127.0.0.1::5432', 'postgres:16-alpine'])
            container_created = True
            pg_port = run(['docker', 'port', container, '5432'], text=True).stdout.strip().split(':')[-1]
        else:
            pg_port = str(free_port())
            pg_data = temp / 'postgres'
            run(['initdb', '-D', str(pg_data), '-A', 'trust', '--no-instructions'])
            with (temp / 'postgres.log').open('w') as log:
                processes.append(subprocess.Popen(['postgres', '-D', str(pg_data), '-h', '127.0.0.1', '-p', pg_port, '-k', str(temp)], env=ENV, stdout=log, stderr=log))
        for _ in range(100):
            ready = ['docker', 'exec', container, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'] if DB_MODE == 'docker' else ['pg_isready', '-h', '127.0.0.1', '-p', pg_port, '-U', os.environ['USER']]
            if subprocess.run(ready, capture_output=True).returncode == 0:
                break
            time.sleep(0.2)
        else:
            raise RuntimeError('Disposable PostgreSQL did not become ready')
        if DB_MODE == 'docker':
            run(['docker', 'exec', container, 'createdb', '-U', 'postgres', 'browser'])
        else:
            run(['createdb', '-h', '127.0.0.1', '-p', pg_port, 'browser'])
        def sql_command(*args):
            return ['docker', 'exec', '-i', container, 'psql', '-U', 'postgres', '-d', 'browser', *args] if DB_MODE == 'docker' else ['psql', '-h', '127.0.0.1', '-p', pg_port, '-d', 'browser', *args]
        for migration in sorted((ROOT / 'server' / 'migrations').glob('*.sql')):
            run(sql_command('-v', 'ON_ERROR_STOP=1'), input=migration.read_bytes())
        today = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).date()
        monday = today - datetime.timedelta(days=today.weekday())
        # Dates move with the real API clock; expected values come from this fixture.
        anchor = today.year if today.month >= 9 else today.year - 1
        fixture = {'yearStart': f'{anchor}-09-01', 'yearEnd': f'{anchor+1}-08-31', 'weekStart': str(monday), 'weekEnd': str(monday+datetime.timedelta(days=6)), 'otherStart': str(monday+datetime.timedelta(days=7)), 'otherEnd': str(monday+datetime.timedelta(days=13))}
        sql = f"""
INSERT INTO academic_years(id,name,start_date,end_date) VALUES('b0000000-0000-0000-0000-000000000001','{anchor}-{anchor+1} 学年','{fixture['yearStart']}','{fixture['yearEnd']}');
INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES('b0000000-0000-0000-0000-000000000002','b0000000-0000-0000-0000-000000000001','FIRST','浏览器回归学期','{fixture['yearStart']}','{fixture['yearEnd']}');
INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date,note,attendance_penalty_multiplier) VALUES
('b0000000-0000-0000-0000-000000000003','b0000000-0000-0000-0000-000000000002',1,'{fixture['weekStart']}','{fixture['weekEnd']}','浏览器回归 A',1),
('b0000000-0000-0000-0000-000000000004','b0000000-0000-0000-0000-000000000002',2,'{fixture['otherStart']}','{fixture['otherEnd']}','浏览器回归 B',1);
"""
        run(sql_command('-v', 'ON_ERROR_STOP=1'), input=sql.encode())
        ids = {key: str(uuid.uuid4()) for key in ('faculty', 'grade', 'class', 'building', 'room', 'roomB', 'course', 'studentA', 'studentB', 'dormArea', 'dormBuilding', 'dormA', 'dormB')}
        fixture.update(ids)
        fixture['date'] = str(monday)
        fixture['expiry'] = (datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))) + datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M')
        fixture_sql = f"""
INSERT INTO faculties(id,name) VALUES('{ids['faculty']}','E2E faculty');
INSERT INTO grades(id,name) VALUES('{ids['grade']}','2026级');
INSERT INTO classes(id,name,grade_id,faculty_id) VALUES('{ids['class']}','E2E class','{ids['grade']}','{ids['faculty']}');
INSERT INTO teaching_buildings(id,name) VALUES('{ids['building']}','E2E building');
INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES('{ids['room']}','{ids['building']}','101');
INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES('{ids['roomB']}','{ids['building']}','102');
INSERT INTO courses(id,name) VALUES('{ids['course']}','E2E course');
INSERT INTO course_classes(course_id,class_id) VALUES('{ids['course']}','{ids['class']}');
INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section) VALUES('{uuid.uuid4()}','{ids['course']}','b0000000-0000-0000-0000-000000000002','{ids['room']}',1,1,2);
INSERT INTO dormitory_areas(id,name) VALUES('{ids['dormArea']}','E2E dorm area');
INSERT INTO dormitory_buildings(id,dormitory_area_id,name) VALUES('{ids['dormBuilding']}','{ids['dormArea']}','E2E dorm building');
INSERT INTO dormitories(id,dormitory_building_id,room_no) VALUES('{ids['dormA']}','{ids['dormBuilding']}','101'),('{ids['dormB']}','{ids['dormBuilding']}','102');
INSERT INTO students(id,student_no,name,grade_id,faculty_id,class_id) VALUES
('{ids['studentA']}','000001','E2E student A','{ids['grade']}','{ids['faculty']}','{ids['class']}'),
('{ids['studentB']}','000002','E2E student B','{ids['grade']}','{ids['faculty']}','{ids['class']}');
UPDATE students SET dormitory_id='{ids['dormA']}' WHERE id='{ids['studentA']}';
UPDATE students SET dormitory_id='{ids['dormB']}' WHERE id='{ids['studentB']}';
INSERT INTO dictionary_types(id,code,name) VALUES('{uuid.uuid4()}','DORMITORY_HYGIENE_REASON','宿舍卫生不合格原因');
INSERT INTO dictionary_items(id,type_id,value,label) SELECT '{uuid.uuid4()}',id,'FLOOR','地面' FROM dictionary_types WHERE code='DORMITORY_HYGIENE_REASON';
INSERT INTO dictionary_types(id,code,name) VALUES('{uuid.uuid4()}','PUBLIC_DORMITORY_HYGIENE_REASON','公共宿舍卫生不合格原因');
INSERT INTO dictionary_types(id,code,name) VALUES('{uuid.uuid4()}','CLASSROOM_HYGIENE_REASON','教室卫生不合格原因');
INSERT INTO dictionary_items(id,type_id,value,label) SELECT '{uuid.uuid4()}',id,'FLOOR','地面' FROM dictionary_types WHERE code='PUBLIC_DORMITORY_HYGIENE_REASON';
INSERT INTO dictionary_items(id,type_id,value,label) SELECT '{uuid.uuid4()}',id,'FLOOR','地面' FROM dictionary_types WHERE code='CLASSROOM_HYGIENE_REASON';
"""
        run(sql_command('-v', 'ON_ERROR_STOP=1'), input=fixture_sql.encode())
        if os.environ.get('BROWSER_SUITE') == 'classroom-hygiene-import':
            run(sql_command('-v', 'ON_ERROR_STOP=1'), input=f"INSERT INTO classroom_hygiene_areas(id,classroom_id,name) VALUES('{uuid.uuid4()}','{ids['room']}','前排'),('{uuid.uuid4()}','{ids['roomB']}','前排');".encode())



        def start(args, name, cwd=ROOT):
            with (temp / (name + '.log')).open('w') as log:
                process = subprocess.Popen(args, cwd=cwd, env=ENV, stdout=log, stderr=log)
            processes.append(process)
            return process

        storage_port, api_port, web_port = free_port(), free_port(), free_port()
        minio_process = start(['minio', 'server', str(temp / 'objects'), '--address', f'127.0.0.1:{storage_port}', '--console-address', '127.0.0.1:0'], 'minio')
        wait_url(f'http://127.0.0.1:{storage_port}/minio/health/ready')
        ENV['MC_HOST_fixture'] = f"http://{ENV['MINIO_ROOT_USER']}:{ENV['MINIO_ROOT_PASSWORD']}@127.0.0.1:{storage_port}"
        mc = ['mc', '--config-dir', str(temp / 'mc')]
        run(mc + ['mb', 'fixture/four-check-files'])
        run(mc + ['version', 'enable', 'fixture/four-check-files'])
        app_secret = secrets.token_hex(24)
        ENV['MINIO_SECRET_KEY'] = app_secret
        run(mc + ['admin', 'user', 'add', 'fixture', 'browser-app', app_secret])
        policy = temp / 'policy.json'
        policy.write_text((ROOT / 'server/minio/app-policy.json').read_text().replace('BUCKET_PLACEHOLDER', 'four-check-files'))
        run(mc + ['admin', 'policy', 'create', 'fixture', 'browser-policy', str(policy)])
        run(mc + ['admin', 'policy', 'attach', 'fixture', 'browser-policy', '--user', 'browser-app'])
        password, changed_password = secrets.token_hex(20), secrets.token_hex(20)
        PRIVATE_VALUES.extend([password, changed_password])
        database_url = f"postgres://postgres:{ENV['POSTGRES_PASSWORD']}@127.0.0.1:{pg_port}/browser?sslmode=disable" if DB_MODE == 'docker' else f"postgres://{os.environ['USER']}@127.0.0.1:{pg_port}/browser?sslmode=disable"
        ENV.update(DATABASE_URL=database_url, HTTP_ADDR=f'127.0.0.1:{api_port}', JWT_SECRET=secrets.token_hex(32), BOOTSTRAP_ADMIN_LOGIN='browser-admin', BOOTSTRAP_ADMIN_NAME='浏览器回归管理员', BOOTSTRAP_ADMIN_PASSWORD=password, MINIO_ENDPOINT=f'127.0.0.1:{storage_port}', MINIO_ACCESS_KEY='browser-app', MINIO_SECRET_KEY=app_secret, MINIO_BUCKET='four-check-files', MINIO_USE_TLS='false', MINIO_CONNECT_TIMEOUT='1s', MINIO_OPERATION_TIMEOUT='3s', MINIO_MAX_FILE_SIZE_BYTES='16777216', COOKIE_SECURE='false', TRUSTED_PROXY_CIDRS='')
        # API gets application credentials only, never the MinIO bootstrap credentials.
        for key in ('MINIO_ROOT_USER', 'MINIO_ROOT_PASSWORD', 'MC_HOST_fixture'):
            ENV.pop(key, None)
        run(['go', 'build', '-o', str(temp / 'api'), './cmd/api'], cwd=ROOT / 'server')
        start([str(temp / 'api')], 'api')
        wait_url(f'http://127.0.0.1:{api_port}/health/ready')
        vite = temp / 'vite.mjs'
        vite.write_text('import { createServer } from ' + json.dumps((ROOT / 'web/node_modules/vite/dist/node/index.js').as_uri()) + ';\nconst server=await createServer(' + json.dumps({'configFile': str(ROOT / 'web/vite.config.ts'), 'root': str(ROOT / 'web'), 'server': {'host': '127.0.0.1', 'port': web_port, 'strictPort': True, 'proxy': {'/api': f'http://127.0.0.1:{api_port}'}}}) + '); await server.listen();\n')
        start(['node', str(vite)], 'vite', ROOT / 'web')
        wait_url(f'http://127.0.0.1:{web_port}/login')
        config = temp / 'cli.json'
        config.write_text(json.dumps({'browser': {'browserName': 'chromium', 'launchOptions': {'channel': 'chrome', 'headless': True}, 'contextOptions': {'viewport': {'width': 1440, 'height': 1000}}}, 'outputDir': str(OUTPUT)}))
        cli('open', f'http://127.0.0.1:{web_port}/login', '--config', str(config))
        session_open = True
        (OUTPUT / 'initial-snapshot.txt').write_text(cli('snapshot'))
        # No traces or raw CLI output are retained while entering disposable secrets.
        login = temp / 'login.js'
        login.write_text('''async (page) => {
  await page.getByRole('textbox', {name:'账号', exact:true}).fill('browser-admin');
  await page.getByLabel('密码', {exact:true}).fill(PASSWORD);
  await page.getByRole('button', {name:/登\s*录/}).click();
  await page.getByRole('heading', {name:'首次登录，请修改密码'}).waitFor();
  await page.getByLabel('当前密码', {exact:true}).fill(PASSWORD);
  await page.getByLabel('新密码', {exact:true}).fill(CHANGED);
  await page.getByLabel('确认新密码', {exact:true}).fill(CHANGED);
  await page.getByRole('button', {name:'更新密码'}).click();
  await page.waitForURL('**/login');
  await page.getByRole('textbox', {name:'账号', exact:true}).fill('browser-admin');
  await page.getByLabel('密码', {exact:true}).fill(CHANGED);
  await page.getByRole('button', {name:/登\s*录/}).click();
  await page.getByRole('region', {name:'当前工作上下文'}).waitFor();
  return 'login, mandatory password change and relogin passed';
}'''.replace('PASSWORD', json.dumps(password)).replace('CHANGED', json.dumps(changed_password)))
        cli('run-code', '--filename', str(login))
        report['cases'].append('真实登录、首次改密、重新登录')
        print('PASS: login and password change', flush=True)
        if ENV.get('BROWSER_SUITE') == 'department-submission':
            fixture.update(authPassword=changed_password, matrixPassword=secrets.token_hex(20))
            PRIVATE_VALUES.append(fixture['matrixPassword'])
            suite = temp / 'department-submission.js'
            suite.write_text((ROOT / 'web/e2e/department-submission.js').read_text().replace('__FIXTURE__', json.dumps(fixture, ensure_ascii=False)).replace('__OUTPUT__', json.dumps(str(OUTPUT))))
            payload = json.loads(cli('--raw', 'run-code', '--filename', str(suite)))
            (OUTPUT / 'department-submission.json').write_text(json.dumps(payload, ensure_ascii=False, indent=2))
            report['suites'] = {'department-submission': payload}
            report['cases'] += payload['cases']
            report['browserVersion'] = payload['browserVersion']
            report['browserErrors'] = payload['browserErrors']
            report['faultInjection'] = payload.get('faultInjection', [])
            report['status'] = 'passed'
            print('PASS: department-submission', flush=True)
            raise SystemExit(0)
        if ENV.get('BROWSER_SUITE') == 'master-data-import':
            fixture['invalidMasterDataImportFile'] = str(temp / 'master-data-malformed.xlsx')
            Path(fixture['invalidMasterDataImportFile']).write_bytes(b'not a workbook')
            suite = temp / 'master-data-import.js'
            suite.write_text((ROOT / 'web/e2e/master-data-import.js').read_text().replace('__FIXTURE__', json.dumps(fixture, ensure_ascii=False)))
            payload = json.loads(cli('--raw', 'run-code', '--filename', str(suite)))
            (OUTPUT / 'master-data-import.json').write_text(json.dumps(payload, ensure_ascii=False, indent=2))
            report['suites'] = {'master-data-import': payload}
            report['cases'] += payload['cases']
            report['browserVersion'] = payload['browserVersion']
            report['browserErrors'] = payload['browserErrors']
            counts = run(sql_command('-tAc', "SELECT count(*),count(*) FILTER (WHERE status='FAILED'),count(*) FILTER (WHERE object_removed_at IS NOT NULL) FROM files WHERE original_name='master-data-malformed.xlsx'"), text=True).stdout.strip()
            if counts != '1|1|1':
                raise RuntimeError('Malformed master-data upload source compensation mismatch: ' + counts)
            report['cases'].append('无效工作簿被拒绝后，PostgreSQL 记录失败状态且 MinIO 对象已清理')
            report['status'] = 'passed'
            print('PASS: master-data-import', flush=True)
            raise SystemExit(0)
        if ENV.get('AUTH_NAVIGATION_DIAGNOSTIC') == '1':
            auth_fixture = dict(fixture, authPassword=changed_password)
            diagnostic = temp / 'auth-navigation.js'
            diagnostic.write_text((ROOT / 'web/e2e/auth-navigation-diagnostic.js').read_text().replace('__FIXTURE__', json.dumps(auth_fixture)))
            payload = json.loads(cli('--raw', 'run-code', '--filename', str(diagnostic)))
            report['authNavigation'] = payload
            report['browserVersion'] = payload['browserVersion']
            report['cases'] += payload['cases']
            report['faultInjection'] = payload.get('faultInjection', [])
            counts = run(sql_command('-tAc', "SELECT count(*) FILTER (WHERE revoked_at IS NULL),count(*) FILTER (WHERE revoked_at IS NOT NULL) FROM refresh_sessions"), text=True).stdout.strip()
            report['sessionCountsActiveRevoked'] = counts
            report['status'] = 'diagnostic-complete'
            raise SystemExit(0)
        source = (ROOT / 'web/e2e/calendar-regression.js').read_text()
        script = temp / 'calendar.js'
        script.write_text(source.replace('__FIXTURE__', json.dumps(fixture, ensure_ascii=False)).replace('__OUTPUT__', json.dumps(str(OUTPUT))))
        result = cli('--raw', 'run-code', '--filename', str(script))
        (OUTPUT / 'calendar-result.txt').write_text(result)
        payload = json.loads(result)
        if payload.get('marker') != 'CALENDAR_REGRESSION_PASSED':
            raise RuntimeError('Browser regression did not return its completion marker')
        report['browserVersion'] = payload['browserVersion']
        report['faultInjection'] = payload['faultInjection']
        report['browserErrors'] = payload['browserErrors']
        report['knownWarnings'] = payload['knownWarnings']
        report['unexpectedConsoleErrors'] = payload['unexpectedConsoleErrors']
        report['status'] = 'passed'
        report['cases'] += ['学年/学期/周次新建关闭后首次编辑与重新打开', '周次 A/B 切换不串值', '真实 409 冲突保留输入并修正重试', '保存立即更新当前工作周及临时周选项', '注入 500/异常响应的安全提示与保留输入', '重载后真实数据库持久化结果']
        (OUTPUT / 'final-snapshot.txt').write_text(cli('snapshot'))
        print('PASS: calendar browser regression', flush=True)
        def browser_suite(name):
            script = temp / (name + '.js')
            script.write_text((ROOT / ('web/e2e/' + name + '.js')).read_text().replace('__FIXTURE__', json.dumps(fixture, ensure_ascii=False)).replace('__OUTPUT__', json.dumps(str(OUTPUT))))
            payload = json.loads(cli('--raw', 'run-code', '--filename', str(script)))
            (OUTPUT / (name + '.json')).write_text(json.dumps(payload, ensure_ascii=False, indent=2))
            report.setdefault('suites', {})[name] = payload
            report['cases'] += payload['cases']
            report['faultInjection'] += payload.get('faultInjection', [])
            report['browserErrors'] += payload.get('browserErrors', [])
            print('PASS: ' + name, flush=True)
            return payload

        if os.environ.get('BROWSER_SUITE') == 'session-renewal':
            # Only the disposable signing key and synthetic account are used here.
            # Force real JWT expiry without waiting 15 minutes or weakening the running API.
            actor = run(sql_command('-tAc', "SELECT id || '|' || session_version FROM accounts WHERE login_name='browser-admin'"), text=True).stdout.strip().split('|')
            def jwt_part(value):
                return base64.urlsafe_b64encode(json.dumps(value).encode()).rstrip(b'=').decode()
            unsigned = jwt_part({'alg': 'HS256', 'typ': 'JWT'}) + '.' + jwt_part({'sub': actor[0], 'ver': int(actor[1]), 'iss': 'four-check-management-system', 'iat': int(time.time()) - 1000, 'exp': int(time.time()) - 60})
            expired = unsigned + '.' + base64.urlsafe_b64encode(hmac.new(ENV['JWT_SECRET'].encode(), unsigned.encode(), hashlib.sha256).digest()).rstrip(b'=').decode()
            PRIVATE_VALUES.append(expired)
            fixture.update(expiredAccessToken=expired, authPassword=changed_password)
            browser_suite('session-renewal')
            report['status'] = 'passed'
            raise SystemExit(0)

        if os.environ.get('BROWSER_SUITE') in ('account-write-conflict', 'admin-read-contract'):
            browser_suite(os.environ['BROWSER_SUITE'])
            report['status'] = 'passed'
            raise SystemExit(0)

        if os.environ.get('BROWSER_SUITE') == 'auth-errors':
            fixture.update(authPassword=changed_password, nextPassword=secrets.token_hex(20))
            PRIVATE_VALUES.append(fixture['nextPassword'])
            run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', "CREATE FUNCTION reject_auth_session_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated auth persistence failure'; END $$; CREATE TRIGGER reject_auth_session_mutation BEFORE INSERT OR UPDATE ON refresh_sessions FOR EACH ROW EXECUTE FUNCTION reject_auth_session_mutation()"))
            try:
                browser_suite('auth-errors-failure')
            finally:
                run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', 'DROP TRIGGER reject_auth_session_mutation ON refresh_sessions; DROP FUNCTION reject_auth_session_mutation()'))
            browser_suite('auth-errors-retry')
            report['status'] = 'passed'
            raise SystemExit(0)

        if os.environ.get('BROWSER_SUITE') == 'auth-refresh-atomic':
            run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', "CREATE FUNCTION reject_refresh_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated refresh save failure'; END $$; CREATE TRIGGER reject_refresh_insert BEFORE INSERT ON refresh_sessions FOR EACH ROW EXECUTE FUNCTION reject_refresh_insert()"))
            try:
                browser_suite('auth-refresh-failure')
            finally:
                run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', 'DROP TRIGGER reject_refresh_insert ON refresh_sessions; DROP FUNCTION reject_refresh_insert()'))
            browser_suite('auth-refresh-retry')
            report['status'] = 'passed'
            raise SystemExit(0)

        if os.environ.get('BROWSER_SUITE') in ('unattended-classroom', 'late-return', 'morning-attendance', 'dormitory-hygiene', 'hygiene-areas', 'public-dormitory-hygiene', 'classroom-hygiene', 'attendance-filter-scope'):
            fixture.update(authPassword=changed_password, matrixPassword=secrets.token_hex(20))
            PRIVATE_VALUES.append(fixture['matrixPassword'])
            browser_suite(os.environ['BROWSER_SUITE'])
            if os.environ['BROWSER_SUITE'] == 'hygiene-areas':
                audit_counts = run(sql_command('-tAc', "SELECT count(*),count(*) FILTER (WHERE actor_id IS NOT NULL AND request_id <> '' AND after_json IS NOT NULL),count(*) FILTER (WHERE before_json IS NOT NULL AND before_json <> 'null'::jsonb) FROM audit_logs WHERE action='MASTER_DATA_CHANGED' AND after_json->>'kind' IN ('classroom-hygiene-areas','public-dormitory-hygiene-areas')"), text=True).stdout.strip()
                if audit_counts != '10|10|4':
                    raise RuntimeError('Area audit transaction coverage mismatch: ' + audit_counts)
                report['cases'].append('真实 PostgreSQL 区域十条审计包含操作者、请求标识、后值，四次修改保留前值')
            report['status'] = 'passed'
            raise SystemExit(0)

        report['status'] = 'failed'
        browser_suite('template-missing')
        actor = run(sql_command('-tAc', "SELECT id FROM accounts WHERE login_name='browser-admin'"), text=True).stdout.strip()
        run(['go', 'build', '-o', str(temp / 'publisher'), './cmd/publish-attendance-template'], cwd=ROOT / 'server')
        template_path = temp / 'template.xlsx'
        template_type = {'unattended-import': 'UNATTENDED_CLASSROOM', 'morning-import': 'MORNING_ATTENDANCE', 'late-return-import': 'LATE_RETURN', 'dormitory-hygiene-import': 'DORMITORY_HYGIENE', 'classroom-hygiene-import': 'CLASSROOM_HYGIENE', 'import-scoped-targets': 'DORMITORY_HYGIENE'}.get(os.environ.get('BROWSER_SUITE'), 'ATTENDANCE_RATE')
        publication = json.loads(run([str(temp / 'publisher'), '--type-code', template_type, '--apply', '--actor-id', actor, '--reason', 'isolated browser regression', '--output', str(template_path)], text=True).stdout)
        fixture['templateSHA'] = publication['sha256']
        if template_type != 'ATTENDANCE_RATE':
            wrong_template = OUTPUT / 'attendance-template.xlsx'
            run([str(temp / 'publisher'), '--output', str(wrong_template)])
            fixture['wrongTemplateFile'] = str(wrong_template)
        namespace = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'
        ET.register_namespace('', namespace)
        for kind in ('valid', 'invalid', 'corrected'):
            destination = OUTPUT / (kind + '.xlsx')
            with zipfile.ZipFile(template_path) as source, zipfile.ZipFile(destination, 'w', zipfile.ZIP_DEFLATED) as target:
                for item in source.infolist():
                    data = source.read(item.filename)
                    if item.filename == 'xl/worksheets/sheet1.xml':
                        sheet = ET.fromstring(data)
                        rows = sheet.find('{'+namespace+'}sheetData')
                        for row in list(rows):
                            if int(row.attrib['r']) >= 5: rows.remove(row)
                        for index in range(2):
                            values = [f'00000{index+1}', f'E2E student {"A" if index == 0 else "B"}', 'E2E class', 'E2E course', fixture['date'], '08:00', '1-2', '2', 'E2E building 101', 'E2E import']
                            if kind != 'valid': values[5] = '10:00'
                            if template_type == 'MORNING_ATTENDANCE': values = values[:3] + [fixture['date'], 'E2E import']
                            if template_type == 'LATE_RETURN': values = values[:3] + [fixture['date'], '23:05:00', f'E2E dorm area E2E dorm building {101+index}', 'E2E import']
                            if template_type == 'DORMITORY_HYGIENE': values = [f'E2E dorm area E2E dorm building {101+index}', fixture['date'], '', '首轮', '优秀' if index == 0 else '不合格', 'E2E class', '' if index == 0 else '000001', '' if index == 0 else '地面', '对象检查', 'E2E import']
                            if template_type == 'CLASSROOM_HYGIENE': values = [f'E2E building {101+index}', '前排', fixture['date'], '', '首轮', '优秀' if index == 0 else '不合格', 'E2E class', '' if index == 0 else '000001', '' if index == 0 else '地面', '区域检查', 'E2E import']
                            if kind == 'invalid' and index == 1: values[0] = 'UNKNOWN'
                            row = ET.SubElement(rows, '{'+namespace+'}row', {'r':str(index+5)})
                            for col, value in enumerate(values):
                                cell = ET.SubElement(row, '{'+namespace+'}c', {'r':chr(65+col)+str(index+5), 't':'inlineStr'})
                                ET.SubElement(ET.SubElement(cell, '{'+namespace+'}is'), '{'+namespace+'}t').text = value
                        data = ET.tostring(sheet, encoding='utf-8', xml_declaration=True)
                    target.writestr(item, data)
            fixture[kind+'File'] = str(destination)
        (OUTPUT / 'bad.txt').write_text('invalid')
        fixture['badFile'] = str(OUTPUT / 'bad.txt')
        if os.environ.get('BROWSER_SUITE') == 'import-source-compensation':
            run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', "CREATE FUNCTION reject_import_batch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated batch failure'; END $$; CREATE TRIGGER reject_import_batch BEFORE INSERT ON import_batches FOR EACH ROW EXECUTE FUNCTION reject_import_batch()"))
            try:
                browser_suite('import-source-failure')
            finally:
                run(sql_command('-v', 'ON_ERROR_STOP=1', '-c', "DROP TRIGGER reject_import_batch ON import_batches; DROP FUNCTION reject_import_batch()"))
            counts = run(sql_command('-tAc', "SELECT (SELECT count(*) FROM files WHERE retention_type='IMPORT_SOURCE' AND status='FAILED' AND failure_code='ORPHANED_IMPORT_SOURCE'),(SELECT count(*) FROM import_batches),(SELECT count(*) FROM audit_logs WHERE action='FILE_UPLOAD_FAILED')"), text=True).stdout.strip()
            if counts != '1|0|1': raise RuntimeError('Source compensation persistence mismatch: '+counts)
            object_key = run(sql_command('-tAc', "SELECT object_key FROM files WHERE retention_type='IMPORT_SOURCE' AND status='FAILED'"), text=True).stdout.strip()
            stat = subprocess.run(mc + ['stat', '--json', 'fixture/four-check-files/'+object_key], env=ENV, capture_output=True, text=True)
            stat_output = stat.stdout + stat.stderr
            stat_codes = []
            def collect_codes(value):
                if isinstance(value, dict):
                    for key, nested in value.items():
                        if key.lower() == 'code' and isinstance(nested, str): stat_codes.append(nested)
                        if key == 'message' and nested == 'Object does not exist': stat_codes.append('ObjectDoesNotExist')
                        collect_codes(nested)
                elif isinstance(value, list):
                    for nested in value: collect_codes(nested)
            for line in stat_output.splitlines():
                try: collect_codes(json.loads(line))
                except json.JSONDecodeError: pass
            if stat.returncode == 0 or not any(code in ('NoSuchKey', 'NoSuchObject', 'ObjectDoesNotExist') for code in stat_codes):
                raise RuntimeError('Compensated source absence not proven; structured codes: '+','.join(stat_codes))
            browser_suite('import-source-retry')
            counts = run(sql_command('-tAc', "SELECT count(*) FROM import_batches b JOIN files f ON f.id=b.original_file_id WHERE b.status='PREVIEW_READY' AND f.status='AVAILABLE'"), text=True).stdout.strip()
            if counts != '1': raise RuntimeError('Retry source binding mismatch')
            report['cases'].append('实际数据库补偿状态和审计、MinIO对象缺失、重试新源文件关联均已核对')
            report['status'] = 'passed'
            raise SystemExit(0)
        if os.environ.get('BROWSER_SUITE') == 'import-scoped-targets':
            fixture['authPassword'] = changed_password
            attendance_template = temp / 'attendance-published.xlsx'
            run([str(temp / 'publisher'), '--apply', '--actor-id', actor, '--reason', 'isolated attendance scope recovery', '--output', str(attendance_template)])
            attendance_invalid = OUTPUT / 'attendance-invalid.xlsx'
            with zipfile.ZipFile(attendance_template) as source, zipfile.ZipFile(attendance_invalid, 'w', zipfile.ZIP_DEFLATED) as target:
                for item in source.infolist():
                    data = source.read(item.filename)
                    if item.filename == 'xl/worksheets/sheet1.xml':
                        sheet = ET.fromstring(data)
                        rows = sheet.find('{'+namespace+'}sheetData')
                        for row in list(rows):
                            if int(row.attrib['r']) >= 5: rows.remove(row)
                        row = ET.SubElement(rows, '{'+namespace+'}row', {'r': '5'})
                        values = ['UNKNOWN', 'E2E student A', 'E2E class', 'E2E course', fixture['date'], '08:00', '1-2', '2', 'E2E building 101', 'E2E invalid recovery']
                        for col, value in enumerate(values):
                            cell = ET.SubElement(row, '{'+namespace+'}c', {'r':chr(65+col)+'5', 't':'inlineStr'})
                            ET.SubElement(ET.SubElement(cell, '{'+namespace+'}is'), '{'+namespace+'}t').text = value
                        data = ET.tostring(sheet, encoding='utf-8', xml_declaration=True)
                    target.writestr(item, data)
            fixture['attendanceInvalidFile'] = str(attendance_invalid)
            browser_suite('import-scoped-targets')
            report['status'] = 'passed'
            raise SystemExit(0)
        if os.environ.get('BROWSER_SUITE') in ('import-rollback', 'unattended-import', 'morning-import', 'late-return-import', 'dormitory-hygiene-import', 'classroom-hygiene-import'):
            fixture['authPassword'] = changed_password
            browser_suite(os.environ['BROWSER_SUITE'])
            counts = run(sql_command('-tAc', "SELECT count(*),count(*) FILTER (WHERE rolled_back_at IS NOT NULL) FROM import_batches WHERE status='ROLLED_BACK'"), text=True).stdout.strip()
            if counts != '1|1':
                raise RuntimeError('Rollback batch persistence mismatch: ' + counts)
            if os.environ['BROWSER_SUITE'] in ('unattended-import', 'morning-import', 'late-return-import', 'dormitory-hygiene-import', 'classroom-hygiene-import'):
                type_counts = run(sql_command('-tAc', f"SELECT (SELECT count(*) FROM inspection_records r JOIN inspection_types t ON t.id=r.inspection_type_id WHERE t.code='ATTENDANCE_RATE'),(SELECT count(*) FROM inspection_records r JOIN inspection_types t ON t.id=r.inspection_type_id WHERE t.code='{template_type}' AND r.deleted_at IS NULL),(SELECT count(*) FROM inspection_records r JOIN inspection_types t ON t.id=r.inspection_type_id WHERE t.code='{template_type}' AND r.deleted_at IS NOT NULL AND r.import_batch_id IS NOT NULL),(SELECT count(*) FROM import_batches WHERE status='PARSE_FAILED')"), text=True).stdout.strip()
                if type_counts != '0|2|2|1':
                    raise RuntimeError('Course import type/rollback persistence mismatch: ' + type_counts)
                report['cases'].append('真实数据库无到课率串入；目标类型两条有效记录、两条回滚留痕及一个错误模板批次保留')
            report['status'] = 'passed'
            raise SystemExit(0)
        if os.environ.get('BROWSER_SUITE') == 'attendance-regression':
            fixture['authPassword'] = changed_password
            browser_suite('attendance-regression')
            report['status'] = 'passed'
            raise SystemExit(0)
        fixture['authPassword'] = changed_password
        browser_suite('attendance-regression')
        recovery = browser_suite('import-recovery')
        # IDs came from the real browser responses; validate before SQL interpolation.
        invalid_id = str(uuid.UUID(recovery['invalidBatchId']))
        corrected_id = str(uuid.UUID(recovery['correctedBatchId']))
        verification_sql = f"SELECT (SELECT count(*) FROM import_batches WHERE id='{invalid_id}' AND status='PREVIEW_READY' AND invalid_rows=1),(SELECT count(*) FROM inspection_records WHERE import_batch_id='{invalid_id}'),(SELECT count(*) FROM inspection_records WHERE import_batch_id='{corrected_id}'),(SELECT count(*) FROM audit_logs WHERE resource_id='{corrected_id}' AND action='IMPORT_BATCH_CONFIRMED')"
        counts = run(sql_command('-tAc', verification_sql), text=True).stdout.strip()
        if counts != '1|0|2|1': raise RuntimeError('Recovery persistence/audit invariant failed: ' + counts)
        report['recoveryDatabaseCheck'] = 'Original error batch retained with zero records; corrected batch has two records and one confirmation audit'
        fixture.update(permissionBatchId=invalid_id, authPassword=changed_password, matrixPassword=secrets.token_hex(20))
        PRIVATE_VALUES.append(fixture['matrixPassword'])
        browser_suite('import-permission-matrix')
        browser_suite('unattended-classroom')
        browser_suite('late-return')
        browser_suite('morning-attendance')
        browser_suite('dormitory-hygiene')
        fixture.pop('authPassword')
        fixture.pop('matrixPassword')
        browser_suite('platform-regression')
        fixture['memberPassword'] = secrets.token_hex(20)
        PRIVATE_VALUES.append(fixture['memberPassword'])
        browser_suite('account-regression')
        # Pause only our disposable process: inject a real storage timeout while
        # leaving API/PostgreSQL running. Always resume before normal cleanup.
        minio_process.send_signal(signal.SIGSTOP)
        try:
            try:
                OPENER.open(f'http://127.0.0.1:{api_port}/health/ready', timeout=10)
                raise RuntimeError('Readiness stayed healthy during storage outage')
            except urllib.error.HTTPError as error:
                if error.code != 503: raise
            browser_suite('object-storage-failure')
        finally:
            minio_process.send_signal(signal.SIGCONT)
        wait_url(f'http://127.0.0.1:{api_port}/health/ready')
        browser_suite('object-storage-recovery')
        downloaded = OUTPUT / 'downloaded-template.xlsx'
        if hashlib.sha256(downloaded.read_bytes()).hexdigest() != fixture['templateSHA']:
            raise RuntimeError('Downloaded template differs from published bytes')
        report['status'] = 'passed'

    except Exception as error:
        report['status'] = 'failed'
        # Never serialize subprocess arguments: mc/login invocations contain secrets.
        report['failureType'] = type(error).__name__
        if not isinstance(error, subprocess.CalledProcessError):
            report['failure'] = sanitize(str(error))
        if session_open and report['cases']:
            try:
                cli('screenshot')
                (OUTPUT / 'failure-snapshot.txt').write_text(sanitize(cli('snapshot')))
            except Exception:
                pass
        raise RuntimeError('Browser regression failed; see sanitized summary.json') from None
    finally:
        cleanup_errors = []
        if session_open:
            try:
                cli('close')
            except Exception:
                cleanup_errors.append('browser session close failed')
        for process in reversed(processes):
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if container_created:
            cleanup = subprocess.run(['docker', 'rm', '-f', container], capture_output=True)
            if cleanup.returncode: cleanup_errors.append('disposable database cleanup failed')
        report['cleanupErrors'] = cleanup_errors
        report['consoleCoverage'] = 'calendar suite; other suites assert observable UI/API behavior and uncaught errors where reported'
        if cleanup_errors: report['status'] = 'failed'
        (OUTPUT / 'summary.json').write_text(json.dumps(report, ensure_ascii=False, indent=2))
        print('Browser report:', OUTPUT, flush=True)
        if cleanup_errors: raise RuntimeError('Disposable resource cleanup failed; see summary.json')
