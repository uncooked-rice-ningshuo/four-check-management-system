#!/usr/bin/env python3
"""Run PostgreSQL/MinIO integration checks using only disposable resources.

Requires local postgres:16-alpine Docker image, MinIO binary, Go and Python 3.
Never reads project .env or connects to an existing database. Credentials are
random and passed in child-process environments; JSON test reports are retained.
"""
import os, pathlib, secrets, subprocess, tempfile, time, socket, urllib.request, shutil, sys
repo = pathlib.Path(__file__).resolve().parents[1]
for tool in ('docker', 'minio', 'go'):
    if shutil.which(tool) is None:
        raise SystemExit(f'Missing required tool: {tool}')
result_dir = pathlib.Path(tempfile.mkdtemp(prefix='four-check-integration-results-'))
print(f'Test reports: {result_dir}', flush=True)
name = 'four-check-audit-' + secrets.token_hex(4)
env = dict(os.environ, POSTGRES_PASSWORD=secrets.token_hex(24), MINIO_ROOT_USER='isolated-test', MINIO_ROOT_PASSWORD=secrets.token_hex(24), GOTOOLCHAIN=os.environ.get('GOTOOLCHAIN', 'go1.25.0'))

def run(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
minio = None
created = False
failed = False
with tempfile.TemporaryDirectory(prefix='four-check-isolated-') as directory:
    try:
        run(['docker', 'run', '--pull=never', '-d', '--rm', '--name', name, '--tmpfs', '/var/lib/postgresql/data', '-e', 'POSTGRES_PASSWORD', '-p', '127.0.0.1::5432', 'postgres:16-alpine'], env=env)
        created = True
        port = run(['docker', 'port', name, '5432']).stdout.decode().strip().split(':')[-1]
        for _ in range(60):
            if subprocess.run(['docker', 'exec', name, 'pg_isready', '-U', 'postgres'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise RuntimeError('Disposable PostgreSQL did not become ready')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            mp = sock.getsockname()[1]
        with open(pathlib.Path(directory) / 'minio.log', 'w') as log:
            minio = subprocess.Popen([shutil.which('minio'), 'server', directory + '/objects', '--address', f'127.0.0.1:{mp}', '--console-address', '127.0.0.1:0'], env=env, stdout=log, stderr=log)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(60):
            try:
                if opener.open(f'http://127.0.0.1:{mp}/minio/health/ready', timeout=1).status == 200:
                    break
            except (OSError, urllib.error.URLError):
                time.sleep(0.5)
        else:
            raise RuntimeError('Disposable MinIO did not become ready')
        env.update(TEST_MINIO_USE_TLS='false', TEST_MINIO_ENDPOINT=f'127.0.0.1:{mp}', TEST_MINIO_ACCESS_KEY=env['MINIO_ROOT_USER'], TEST_MINIO_SECRET_KEY=env['MINIO_ROOT_PASSWORD'])

        def database(label):
            run(['docker', 'exec', name, 'createdb', '-U', 'postgres', label])
            return f"postgres://postgres:{env['POSTGRES_PASSWORD']}@127.0.0.1:{port}/{label}?sslmode=disable"
        for (key, label) in [('READINESS_TEST_DATABASE_URL', 'readiness'), ('INSPECTION_TEST_DATABASE_URL', 'inspection'), ('WORK_CONTEXT_TEST_DATABASE_URL', 'workcontext'), ('EXTENSION_TEST_DATABASE_URL', 'extensions'), ('MANUAL_TEST_DATABASE_URL', 'manual'), ('MUTATION_TEST_DATABASE_URL', 'mutations'), ('ORPHAN_SOURCE_TEST_DATABASE_URL', 'orphan_source')]:
            env[key] = database(label)
        env['TEST_DATABASE_URL'] = database('store_tests')
        for migration in sorted((repo / 'migrations').glob('*.sql')):
            run(['docker', 'exec', '-i', name, 'psql', '-U', 'postgres', '-d', 'store_tests', '-v', 'ON_ERROR_STOP=1'], input=migration.read_bytes())
        for (package, filename) in [('./internal/store', 'store'), ('./internal/storage', 'storage'), ('./internal/importjob', 'import'), ('./cmd/publish-attendance-template', 'template-cli')]:
            if filename == 'template-cli':
                env['TEMPLATE_CLI_TEST_DATABASE_URL'] = database('template_cli_tests')
            if filename == 'import':
                env['TEST_DATABASE_URL'] = database('import_tests')
            with open(result_dir / (filename + '.json'), 'w') as output:
                result = subprocess.run(['go', 'test', package, '-count=1', '-json'], cwd=repo, env=env, stdout=output, stderr=subprocess.STDOUT)
            failed = failed or result.returncode != 0
            print(package, 'exit', result.returncode, flush=True)
    finally:
        if minio:
            minio.terminate()
            minio.wait(timeout=10)
        if created:
            subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
sys.exit(1 if failed else 0)
