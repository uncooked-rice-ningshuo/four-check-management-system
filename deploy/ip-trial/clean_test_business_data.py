#!/usr/bin/env python3
"""Preview or clear test inspection/report transactions.

This is an operational-data reset, not a database rebuild. Keep this table list
and the incoming-FK guard in sync whenever a migration adds inspection, import,
submission, report, or file-reference tables. Master data, accounts, permissions,
academic calendar, weekly report obligations, morning schedules, dictionaries,
and audit logs are deliberately retained. The default target remains the IP
trial; local Compose is an explicit, separately fenced target.
"""

from __future__ import annotations

import argparse
import fcntl
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import time
import uuid


DEFAULT_BASE = Path("/opt/four-check/ip-trial")
DEFAULT_PROJECT = "four-check-ip-trial"
DEFAULT_BACKUP_ROOT = Path("/opt/four-check/backups")
LOCAL_BASE = Path(__file__).resolve().parents[2]
LOCAL_PROJECT = LOCAL_BASE.name
LOCAL_BACKUP_ROOT = Path("/private/tmp/four-check-local-cleanups")
LOCAL_POSTGRES_TARGET = "__local_postgres__"
LOCAL_POSTGRES_SOCKET = "/tmp"
LOCAL_POSTGRES_PORT = "5432"
LOCAL_POSTGRES_USER = getpass.getuser()
CONFIRM_TEXT = "清理日常检查和通报测试数据"
GIB = 1024**3
ALL_SERVICES = ("postgres", "minio", "api", "web")
LOCAL_SERVICES = ("postgres", "minio")

# Delete children before their referenced parents. Update this list with every
# new inspection/report transaction table; unknown incoming FKs stop the reset.
PURGE_ORDER = (
    # Report output attempts reference both immutable report versions and files.
    "report_output_files",
    "weekly_report_version_zero_receipts",
    "weekly_report_version_files",
    "weekly_report_version_adjustments",
    "weekly_report_correction_responses",
    "weekly_report_reviews",
    "weekly_report_corrections",
    "weekly_report_version_records",
    "weekly_report_versions",
    "weekly_reports",
    "import_rows",
    "inspection_responsibility_dormitories",
    "inspection_responsibility_students",
    "inspection_responsibility_classes",
    "attendance_details",
    "morning_exercise_details",
    "late_return_details",
    "dorm_hygiene_details",
    "classroom_hygiene_details",
    "public_dormitory_hygiene_details",
    "evening_self_study_details",
    "inspection_records",
    "department_submission_revisions",
    "department_submissions",
    "import_batches",
    "import_templates",
)
PURGE_SET = set(PURGE_ORDER) | {"files"}
REQUIRED_TABLES = {
    "schema_migrations",
    "audit_logs",
    "files",
    "import_templates",
    "import_batches",
    "import_rows",
    "inspection_records",
    "department_submissions",
    "department_submission_revisions",
}
PRESERVED_FILE_REFERENCES = {
    ("master_data_import_batches", "files"),
    ("academic_week_import_batches", "files"),
}
PRESERVED_REPORT_CONFIG_TABLES = (
    "report_output_settings",
    "report_output_week_dates",
    "semester_report_grade_scopes",
    "semester_report_grades",
)


class CleanupError(RuntimeError):
    pass


def run(command: list[str], *, input_bytes: bytes | None = None, stdout=None) -> bytes:
    result = subprocess.run(
        command,
        input=input_bytes,
        stdout=subprocess.PIPE if stdout is None else stdout,
        stderr=subprocess.PIPE,
        check=False,
    )
    if result.returncode:
        raise CleanupError(f"命令失败（退出码 {result.returncode}）：{command[0]} {command[1]}")
    return result.stdout or b""


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def compose_command(base: Path, project: str) -> list[str]:
    return [
        "docker", "compose", "--project-directory", str(base), "-p", project,
        "--env-file", str(base / ".env"), "-f", str(base / "compose.yaml"),
    ]


def service_container(compose: list[str], project: str, service: str) -> tuple[str, dict]:
    ids = run(compose + ["ps", "-a", "-q", service]).decode().split()
    if len(ids) != 1:
        raise CleanupError(f"{service} 服务容器数量不是 1，停止清理")
    info = json.loads(run(["docker", "inspect", ids[0]]))[0]
    if info["Config"]["Labels"].get("com.docker.compose.project") != project:
        raise CleanupError(f"{service} 容器不属于目标 Compose 项目")
    if info["State"].get("Status") != "running":
        raise CleanupError(f"{service} 容器未运行")
    if info["State"].get("Health", {}).get("Status") != "healthy":
        raise CleanupError(f"{service} 容器未 healthy")
    return ids[0], info


def psql(pg_id: str, query: str, *, variables: dict[str, str] | None = None) -> str:
    if pg_id == LOCAL_POSTGRES_TARGET:
        command = [
            "psql", "-X", "-q", "-w", "-h", LOCAL_POSTGRES_SOCKET,
            "-p", LOCAL_POSTGRES_PORT, "-U", LOCAL_POSTGRES_USER, "-d", "four_check",
            "-v", "ON_ERROR_STOP=1",
        ]
    else:
        command = [
            "docker", "exec", "-i", pg_id, "psql", "-X", "-q", "-U", "four_check",
            "-d", "four_check", "-v", "ON_ERROR_STOP=1",
        ]
    for name, value in (variables or {}).items():
        command.extend(["-v", f"{name}={value}"])
    command.extend(["-f", "-"])
    return run(command, input_bytes=query.encode()).decode().strip()


def psql_scalar(pg_id: str, query: str) -> str:
    if pg_id == LOCAL_POSTGRES_TARGET:
        command = [
            "psql", "-X", "-q", "-w", "-h", LOCAL_POSTGRES_SOCKET,
            "-p", LOCAL_POSTGRES_PORT, "-U", LOCAL_POSTGRES_USER, "-d", "four_check",
            "-v", "ON_ERROR_STOP=1", "-Atqc", query,
        ]
    else:
        command = [
            "docker", "exec", pg_id, "psql", "-X", "-q", "-U", "four_check",
            "-d", "four_check", "-v", "ON_ERROR_STOP=1", "-Atqc", query,
        ]
    return run(command).decode().strip()


def table_names(pg_id: str) -> set[str]:
    return set(psql_scalar(pg_id, "SELECT tablename FROM pg_tables WHERE schemaname='public'").splitlines())


def count_table(pg_id: str, table: str, existing: set[str]) -> int:
    if table not in existing:
        return 0
    return int(psql_scalar(pg_id, f'SELECT count(*) FROM public."{table}"'))


def verify_incoming_foreign_keys(pg_id: str, existing: set[str]) -> None:
    targets = sorted(PURGE_SET & existing)
    if not targets:
        raise CleanupError("清理范围表不存在")
    quoted = ",".join("'" + name + "'" for name in targets)
    query = f"""
SELECT child.relname || '|' || parent.relname
FROM pg_constraint c
JOIN pg_class child ON child.oid=c.conrelid
JOIN pg_namespace child_ns ON child_ns.oid=child.relnamespace
JOIN pg_class parent ON parent.oid=c.confrelid
JOIN pg_namespace parent_ns ON parent_ns.oid=parent.relnamespace
WHERE c.contype='f' AND child_ns.nspname='public' AND parent_ns.nspname='public'
  AND parent.relname IN ({quoted})
ORDER BY 1
"""
    for edge in filter(None, psql_scalar(pg_id, query).splitlines()):
        child, parent = edge.split("|", 1)
        if child not in PURGE_SET and (child, parent) not in PRESERVED_FILE_REFERENCES:
            raise CleanupError(
                f"发现清理清单外的外键依赖 {child} -> {parent}；先评估并维护清理脚本"
            )


def file_candidates(pg_id: str, existing: set[str], bucket: str | None) -> list[dict]:
    if "files" not in existing:
        return []
    exclusions = []
    for table in ("master_data_import_batches", "academic_week_import_batches"):
        if table in existing:
            exclusions.append(f"NOT EXISTS (SELECT 1 FROM public.{table} b WHERE b.source_file_id=f.id)")
    preserved_files_clause = " AND ".join(exclusions) if exclusions else "TRUE"
    query = f"""
SELECT id::text || '|' || bucket || '|' || object_key || '|' || retention_type || '|' || size_bytes::text
FROM public.files f
WHERE (
    (f.retention_type='TEMPLATE' AND EXISTS (SELECT 1 FROM public.import_templates t WHERE t.file_id=f.id))
    OR f.retention_type IN ('REPORT_DRAFT','REPORT_FINAL','ARCHIVE')
    OR (f.retention_type='IMPORT_SOURCE')
)
AND ({preserved_files_clause})
ORDER BY id
"""
    files = []
    for line in filter(None, psql_scalar(pg_id, query).splitlines()):
        file_id, file_bucket, object_key, retention, size = line.split("|", 4)
        try:
            uuid.UUID(file_id)
        except ValueError as error:
            raise CleanupError("文件清理候选包含无效 ID") from error
        if not re.fullmatch(r"[a-z0-9.-]{3,96}", file_bucket):
            raise CleanupError("待清理文件包含无效 Bucket 名称")
        if bucket is not None and file_bucket != bucket:
            raise CleanupError("待清理文件不属于目标 MinIO Bucket")
        if not object_key or any(ord(char) < 32 for char in object_key) or "\n" in object_key:
            raise CleanupError("待清理对象键格式不安全")
        files.append({"id": file_id, "bucket": file_bucket, "objectKey": object_key,
                      "retention": retention, "sizeBytes": int(size)})
    return files


def target_state(compose: list[str], project: str, services: tuple[str, ...] = ALL_SERVICES) -> dict:
    base_config = json.loads(run(compose + ["config", "--format", "json"]))
    base = Path(compose[3])
    if not (base / ".env").is_file() or not (base / "compose.yaml").is_file():
        raise CleanupError("目标 .env 或 compose.yaml 不存在")
    containers = {}
    for service in services:
        cid, info = service_container(compose, project, service)
        containers[service] = {"id": cid, "info": info}
    for service, destination, volume in (
        ("postgres", "/var/lib/postgresql/data", f"{project}_postgres_data"),
        ("minio", "/data", f"{project}_minio_data"),
    ):
        mounts = [item for item in containers[service]["info"]["Mounts"] if item["Destination"] == destination]
        if len(mounts) != 1 or mounts[0].get("Name") != volume:
            raise CleanupError(f"{service} 数据卷不属于目标 Compose 项目")
    if psql_scalar(containers["postgres"]["id"], "SELECT current_database()") != "four_check":
        raise CleanupError("目标数据库名称不是 four_check")
    api_env = base_config["services"]["api"].get("environment", {})
    bucket = api_env.get("MINIO_BUCKET", "four-check-files")
    if not re.fullmatch(r"[a-z0-9.-]{3,96}", bucket):
        raise CleanupError("Compose 中 MINIO_BUCKET 无效")
    return {"containers": containers, "bucket": bucket, "composeConfig": base_config}


def snapshot(compose: list[str], project: str, pg_id: str, minio_id: str | None,
             backup_root: Path, counts: dict, files: list[dict]) -> Path:
    backup_root.mkdir(parents=True, mode=0o700, exist_ok=True)
    os.chmod(backup_root, 0o700)
    stamp = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
    backup = backup_root / f"before-inspection-report-cleanup-{stamp}"
    backup.mkdir(mode=0o700, exist_ok=False)
    os.chmod(backup, 0o700)
    (backup / "counts-before.json").write_text(json.dumps(counts, ensure_ascii=False, indent=2) + "\n")
    (backup / "target-files.json").write_text(json.dumps(files, ensure_ascii=False, indent=2) + "\n")
    (backup / "target-files.json").chmod(0o600)

    database_dump = backup / "database.dump.part"
    with database_dump.open("wb") as dump:
        if pg_id == LOCAL_POSTGRES_TARGET:
            run([
                "pg_dump", "-w", "-h", LOCAL_POSTGRES_SOCKET, "-p", LOCAL_POSTGRES_PORT,
                "-U", LOCAL_POSTGRES_USER, "-d", "four_check", "-Fc",
            ], stdout=dump)
        else:
            run(["docker", "exec", pg_id, "pg_dump", "-U", "four_check", "-d", "four_check", "-Fc"], stdout=dump)
    if pg_id == LOCAL_POSTGRES_TARGET:
        run(["pg_restore", "--list", str(database_dump)])
    else:
        with database_dump.open("rb") as dump:
            run(["docker", "exec", "-i", pg_id, "pg_restore", "--list"], input_bytes=dump.read())
    (backup / "database.dump.part").replace(backup / "database.dump")

    if minio_id is not None:
        run(compose + ["stop", "-t", "30", "minio"])
        volume = f"{project}_minio_data"
        try:
            with (backup / "minio-data.tar.gz.part").open("wb") as archive:
                run([
                    "docker", "run", "--rm", "--pull=never", "--network", "none", "--user", "0",
                    "--mount", f"type=volume,src={volume},dst=/data,readonly",
                    "postgres:16-alpine", "tar", "-C", "/data", "-czf", "-", ".",
                ], stdout=archive)
            verify_tar(backup / "minio-data.tar.gz.part")
            (backup / "minio-data.tar.gz.part").replace(backup / "minio-data.tar.gz")
        finally:
            try:
                run(compose + ["start", "minio"])
                wait_for_service_healthy(minio_id, "MinIO")
            except CleanupError:
                print(f"MinIO 未恢复到 healthy；完整备份保留于：{backup}", flush=True)
                raise

    metadata = {
        "database": "four_check",
        "composeProject": None if pg_id == LOCAL_POSTGRES_TARGET else project,
        "createdAtUtc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "databaseSha256": sha256(backup / "database.dump"),
        "databaseBytes": (backup / "database.dump").stat().st_size,
    }
    if minio_id is not None:
        metadata["minioArchiveSha256"] = sha256(backup / "minio-data.tar.gz")
        metadata["minioArchiveBytes"] = (backup / "minio-data.tar.gz").stat().st_size
    else:
        metadata["target"] = "host PostgreSQL over local Unix socket"
    (backup / "backup.json").write_text(json.dumps(metadata, indent=2) + "\n")
    write_checksums(backup)
    return backup


def verify_tar(path: Path) -> None:
    try:
        with tarfile.open(path, "r:gz") as archive:
            for member in archive:
                if member.isfile():
                    stream = archive.extractfile(member)
                    if stream is None:
                        raise CleanupError("MinIO 备份 tar 中存在无法读取的文件")
                    with stream:
                        while stream.read(1024 * 1024):
                            pass
    except (OSError, tarfile.TarError) as error:
        raise CleanupError("MinIO 备份归档校验失败") from error


def wait_for_service_healthy(container_id: str, service: str, timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        info = json.loads(run(["docker", "inspect", container_id]))[0]
        state = info.get("State", {})
        if state.get("Status") == "running" and state.get("Health", {}).get("Status") == "healthy":
            return
        if state.get("Status") in {"exited", "dead"}:
            raise CleanupError(f"{service} 容器已退出，备份已保留")
        time.sleep(2)
    raise CleanupError(f"{service} 在 {timeout_seconds} 秒内未恢复 healthy，备份已保留")


def write_checksums(directory: Path) -> None:
    lines = []
    for path in sorted(directory.rglob("*")):
        if path.is_file() and path.name != "SHA256SUMS":
            lines.append(f"{sha256(path)}  {path.relative_to(directory)}")
    (directory / "SHA256SUMS").write_text("\n".join(lines) + "\n")
    (directory / "SHA256SUMS").chmod(0o600)


def remove_objects(compose: list[str], files: list[dict]) -> None:
    keys = [item["objectKey"] for item in files]
    if not keys:
        return
    script = (
        'set -eu; mc alias set cleanup http://minio:9000 "$MINIO_ROOT_USER" '
        '"$MINIO_ROOT_PASSWORD" >/dev/null; '
        'while IFS= read -r key; do [ -z "$key" ] || '
        'mc rm --versions --force "cleanup/$MINIO_BUCKET/$key" >/dev/null; done'
    )
    run(
        compose + ["run", "--rm", "--no-deps", "-T", "--entrypoint", "/bin/sh",
                   "minio-init", "-ec", script],
        input_bytes=("\n".join(keys) + "\n").encode(),
    )


def purge_sql(existing: set[str], file_ids: list[str]) -> str:
    tables = [name for name in PURGE_ORDER if name in existing]
    lock_tables = tables + (["files"] if "files" in existing else [])
    lines = ["BEGIN;", "SET LOCAL lock_timeout='10s';", "SET LOCAL statement_timeout='15min';"]
    if lock_tables:
        lines.append("LOCK TABLE " + ", ".join(f'public."{name}"' for name in lock_tables) + " IN ACCESS EXCLUSIVE MODE;")
    lines += [
        "INSERT INTO public.audit_logs(id,actor_id,action,resource_type,resource_id,before_json,after_json,reason)",
        "VALUES (:'cleanup_id'::uuid, :'actor_id'::uuid, 'TEST_BUSINESS_DATA_CLEANED',",
        "        'deployment_cleanup', :'cleanup_id', :'before_json'::jsonb,",
        "        jsonb_build_object('cleanupRunId', :'cleanup_id', 'removedFileCount', :'file_count'::integer),",
        "        :'reason');",
    ]
    for name in tables:
        lines.append(f'ALTER TABLE public."{name}" DISABLE TRIGGER USER;')
    if "weekly_reports" in existing and "inspection_records" in existing:
        lines.append("UPDATE public.inspection_records SET locked_by_report_id=NULL WHERE locked_by_report_id IS NOT NULL;")
    for name in tables:
        lines.append(f'DELETE FROM public."{name}";')
    if "files" in existing:
        lines.append(
            "DELETE FROM public.files WHERE id::text IN "
            "(SELECT jsonb_array_elements_text(:'file_ids_json'::jsonb));"
        )
    for name in tables:
        lines.append(f'ALTER TABLE public."{name}" ENABLE TRIGGER USER;')
    lines += ["COMMIT;", ""]
    return "\n".join(lines)


def check_actor(pg_id: str, actor_id: str) -> None:
    try:
        normalized = str(uuid.UUID(actor_id))
    except ValueError as error:
        raise CleanupError("actor-id 必须是 UUID") from error
    allowed = psql_scalar(
        pg_id,
        f"SELECT EXISTS(SELECT 1 FROM accounts WHERE id='{normalized}' AND status='ACTIVE' AND is_super_admin)",
    )
    if allowed != "t":
        raise CleanupError("actor-id 必须是启用中的超级管理员")


def inspect_database(pg_id: str, bucket: str | None) -> dict:
    existing = table_names(pg_id)
    if not REQUIRED_TABLES.issubset(existing):
        missing = sorted(REQUIRED_TABLES - existing)
        raise CleanupError("当前数据库缺少必要业务表，不能按该脚本清理：" + ",".join(missing))
    preserved_prefixed_tables = {
        "inspection_types", "weekly_report_obligations", "master_data_import_batches",
        "academic_week_import_batches", *PRESERVED_REPORT_CONFIG_TABLES,
    }
    unknown_prefixed = sorted(
        name for name in existing
        if name.startswith((
            "weekly_report_", "department_submission_", "inspection_", "import_",
            "report_output_", "semester_report_",
        ))
        and name not in PURGE_SET and name not in preserved_prefixed_tables
    )
    if unknown_prefixed:
        raise CleanupError(
            "发现尚未纳入清理脚本的检查/通报表：" + ",".join(unknown_prefixed)
            + "；先评估并维护清理清单"
        )
    verify_incoming_foreign_keys(pg_id, existing)
    database = psql_scalar(pg_id, "SELECT current_database()")
    if database != "four_check":
        raise CleanupError("目标数据库不是 four_check")
    versions = psql_scalar(pg_id, "SELECT version FROM schema_migrations ORDER BY version").splitlines()
    counts = {name: count_table(pg_id, name, existing) for name in PURGE_ORDER}
    file_rows = file_candidates(pg_id, existing, bucket)
    counts["files_to_remove"] = len(file_rows)
    counts["file_object_bytes_to_remove"] = sum(item["sizeBytes"] for item in file_rows)
    counts["audit_logs_preserved"] = count_table(pg_id, "audit_logs", existing)
    counts["weekly_report_obligations_preserved"] = count_table(pg_id, "weekly_report_obligations", existing)
    for table in PRESERVED_REPORT_CONFIG_TABLES:
        counts[f"{table}_preserved"] = count_table(pg_id, table, existing)
    return {
        "pgId": pg_id,
        "existingTables": existing,
        "versions": versions,
        "counts": counts,
        "files": file_rows,
    }


def inspect(base: Path, project: str, services: tuple[str, ...] = ALL_SERVICES) -> dict:
    compose = compose_command(base, project)
    state = target_state(compose, project, services)
    if not (base / ".env").is_file() or not (base / "compose.yaml").is_file():
        raise CleanupError("目标部署文件缺失")
    target = inspect_database(state["containers"]["postgres"]["id"], state["bucket"])
    target["compose"] = compose
    target["state"] = state
    return target


def inspect_local_postgres(base: Path) -> dict:
    if base.resolve() != LOCAL_BASE.resolve():
        raise CleanupError("本机 PostgreSQL 清理仅允许当前仓库根目录")
    identity = psql_scalar(
        LOCAL_POSTGRES_TARGET,
        """
SELECT current_database() || '|' || current_user || '|' ||
       CASE WHEN inet_server_addr() IS NULL THEN 'unix_socket' ELSE inet_server_addr()::text END || '|' ||
       (SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()) || '|' ||
       (SELECT rolsuper::text FROM pg_roles WHERE rolname=current_user)
""",
    )
    expected_identity = f"four_check|{LOCAL_POSTGRES_USER}|unix_socket|four_check|true"
    if identity != expected_identity:
        raise CleanupError(
            "本机 PostgreSQL 身份或权限不匹配；只允许 Unix socket 上 owner 为 four_check、"
            "且当前 macOS 用户为本地 PostgreSQL 超级用户的 four_check 库"
        )
    target = inspect_database(LOCAL_POSTGRES_TARGET, None)
    target["compose"] = []
    target["state"] = {"bucket": None, "localPostgres": True}
    return target


def ensure_no_competing_local_postgres_sessions(pg_id: str) -> None:
    sessions = psql_scalar(
        pg_id,
        """
SELECT count(*)
FROM pg_stat_activity
WHERE datname=current_database() AND pid <> pg_backend_pid()
""",
    )
    if sessions != "0":
        raise CleanupError("本机 four_check 库还有其他连接；先停止 API/客户端，再重新预览和清理")


def check_space(base: Path, pg_id: str, minio_id: str | None, backup_root: Path | None = None) -> None:
    database_bytes = int(psql_scalar(pg_id, "SELECT pg_database_size(current_database())"))
    if minio_id is None:
        backup_location = backup_root or base
        required = max(128 * 1024**2, 2 * database_bytes + 64 * 1024**2)
        if shutil.disk_usage(backup_location.parent).free < required:
            raise CleanupError("磁盘余量不足以制作并校验本机 PostgreSQL 备份")
        return
    minio_kib = int(run(["docker", "exec", minio_id, "du", "-sk", "/data"]).split()[0])
    minio_bytes = minio_kib * 1024
    required = max(1024**3, 2 * (database_bytes + minio_bytes) + 512 * 1024**2)
    if shutil.disk_usage(base).free < required:
        raise CleanupError("磁盘余量不足以制作数据库与 MinIO 可验证备份")


def restore_minio(compose: list[str], project: str, backup: Path) -> None:
    run(compose + ["stop", "-t", "30", "minio"])
    run([
        "docker", "run", "--rm", "--pull=never", "--network", "none", "--user", "0",
        "--mount", f"type=volume,src={project}_minio_data,dst=/data",
        "--mount", f"type=bind,src={backup.resolve()},dst=/backup,readonly",
        "postgres:16-alpine", "sh", "-ec",
        "find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + && tar -xzf /backup/minio-data.tar.gz -C /data",
    ])
    run(compose + ["start", "minio"])


def health_check(compose: list[str], project: str, url: str,
                  services: tuple[str, ...] = ALL_SERVICES) -> None:
    import urllib.request

    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for _ in range(60):
        try:
            ready = "api" not in services
            if "api" in services:
                with opener.open(url.rstrip("/") + "/health/ready", timeout=3) as response:
                    ready = response.status == 200
            for service in services:
                service_container(compose, project, service)
            if "api" not in services or ready:
                return
        except (OSError, CleanupError, ValueError):
            time.sleep(2)
    raise CleanupError("清理完成但服务未恢复到 healthy/ready；备份已保留")


def ensure_local_compose_context() -> None:
    if os.environ.get("DOCKER_HOST") or os.environ.get("DOCKER_CONTEXT"):
        raise CleanupError("本地清理不接受 DOCKER_HOST/DOCKER_CONTEXT 覆盖，请使用本机 Docker context")
    context = run(["docker", "context", "show"]).decode().strip()
    host = run(["docker", "context", "inspect", context,
                "--format", "{{.Endpoints.docker.Host}}"]).decode().strip()
    if not host.startswith("unix://"):
        raise CleanupError("本地清理只允许连接本机 Unix socket Docker context")


def apply_cleanup(base: Path, project: str, backup_root: Path, actor_id: str,
                  reason: str, url: str, *, local_compose: bool = False,
                  local_postgres: bool = False) -> dict:
    base.mkdir(parents=True, exist_ok=True)
    lock_root = backup_root if local_compose or local_postgres else base
    lock_root.mkdir(parents=True, mode=0o700, exist_ok=True)
    lock_path = lock_root / ".clean-test-business-data.lock"
    with lock_path.open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise CleanupError("已有数据清理任务正在执行") from error

        services = LOCAL_SERVICES if local_compose else ALL_SERVICES
        target = inspect_local_postgres(base) if local_postgres else inspect(base, project, services)
        if local_postgres:
            target["counts"]["file_object_bytes_not_removed"] = target["counts"].pop(
                "file_object_bytes_to_remove"
            )
        if local_compose:
            ensure_local_compose_context()
            running_apps = [
                service for service in ("api", "web")
                if run(target["compose"] + ["ps", "-q", service]).decode().strip()
            ]
            if running_apps:
                raise CleanupError("本地 api/web 必须保持停止状态；请先清理数据，再启动含数据库迁移的 Compose 全栈")
        if local_postgres:
            ensure_no_competing_local_postgres_sessions(target["pgId"])
        check_actor(target["pgId"], actor_id)
        minio_id = None if local_postgres else target["state"]["containers"]["minio"]["id"]
        check_space(base, target["pgId"], minio_id, backup_root if local_postgres else None)
        backup = None
        services_stopped = False
        object_removal_started = False
        committed = False
        cleanup_id = str(uuid.uuid4())
        before_json = json.dumps(target["counts"], ensure_ascii=False, separators=(",", ":"))
        file_ids_json = json.dumps([item["id"] for item in target["files"]], separators=(",", ":"))
        try:
            if not local_compose and not local_postgres:
                run(target["compose"] + ["stop", "-t", "30", "web", "api"])
                services_stopped = True
            backup = snapshot(target["compose"], project, target["pgId"], minio_id,
                              backup_root, target["counts"], target["files"])
            object_removal_started = bool(target["files"]) and not local_postgres
            if not local_postgres:
                remove_objects(target["compose"], target["files"])
            sql = purge_sql(target["existingTables"], [item["id"] for item in target["files"]])
            psql(target["pgId"], sql, variables={
                "cleanup_id": cleanup_id,
                "actor_id": str(uuid.UUID(actor_id)),
                "before_json": before_json,
                "file_ids_json": file_ids_json,
                "file_count": str(len(target["files"])),
                "reason": reason,
            })
            committed = True
            after_tables = table_names(target["pgId"])
            after_counts = {name: count_table(target["pgId"], name, after_tables) for name in PURGE_ORDER}
            after_files = file_candidates(target["pgId"], after_tables, target["state"]["bucket"])
            after_counts["files_to_remove"] = len(after_files)
            after_counts["file_object_bytes_to_remove"] = sum(item["sizeBytes"] for item in after_files)
            after_counts["audit_logs_preserved"] = count_table(target["pgId"], "audit_logs", after_tables)
            after_counts["weekly_report_obligations_preserved"] = count_table(
                target["pgId"], "weekly_report_obligations", after_tables
            )
            for table in PRESERVED_REPORT_CONFIG_TABLES:
                after_counts[f"{table}_preserved"] = count_table(target["pgId"], table, after_tables)
            remaining = {
                key: value for key, value in after_counts.items()
                if not key.endswith("_preserved") and value != 0
            }
            if remaining:
                raise CleanupError("清理后发现业务数据未清空，备份已保留")
            if not local_compose and not local_postgres:
                run(target["compose"] + ["up", "-d", "--no-deps", "--pull", "never", "api", "web"])
                services_stopped = False
            if not local_postgres:
                health_check(target["compose"], project, url, services)
            result = {
                "mode": "applied-local-postgres" if local_postgres else (
                    "applied-local-compose" if local_compose else "applied-ip-trial"
                ),
                "cleanupRunId": cleanup_id,
                "database": "four_check",
                "migrationLedger": target["versions"],
                "deletedCounts": target["counts"],
                "templateRepublishRequired": True,
                "preserved": ["accounts", "permissions", "master data", "academic calendar",
                              "inspection types", "reason dictionaries", "report obligations",
                              "report output settings/week dates", "semester report grade scopes",
                              "audit logs", "master-data/academic-week import files"],
                "backup": str(backup),
            }
            if local_compose:
                result["nextStep"] = "清理已完成；现在可运行 docker compose up --build 以应用迁移并更新本地 API/Web。"
            if local_postgres:
                result["minioCleanupPerformed"] = False
                result["objectsNotRemoved"] = {
                    "count": len(target["files"]),
                    "sizeBytes": sum(item["sizeBytes"] for item in target["files"]),
                }
                result["nextStep"] = (
                    '清理已完成；从仓库根目录运行 '
                    'PGHOST=/tmp PGPORT=5432 PGUSER="$(id -un)" PGDATABASE=four_check '
                    'MIGRATIONS_DIR="$PWD/server/migrations" sh server/scripts/migrate.sh '
                    "应用数据库迁移。MinIO 对象未触碰。"
                )
            (backup / "cleanup-result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
            (backup / "cleanup-result.json").chmod(0o600)
            write_checksums(backup)
            return result
        except Exception:
            if backup is not None and object_removal_started and not committed:
                try:
                    restore_minio(target["compose"], project, backup)
                except Exception:
                    print(f"MinIO 需要从备份手工恢复：{backup}", flush=True)
            if services_stopped and not local_compose and not local_postgres:
                subprocess.run(target["compose"] + ["up", "-d", "--no-deps", "--pull", "never", "api", "web"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
            if backup is not None:
                print(f"清理未完整完成，备份保留于：{backup}", flush=True)
            raise


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", choices=("ip-trial", "local-compose", "local-postgres"), default="ip-trial",
                        help="清理目标；默认仍是受严格路径保护的腾讯云 IP 试运行环境")
    parser.add_argument("--apply", action="store_true", help="备份后清除检查/通报业务数据")
    parser.add_argument("--confirm", default="", help=f"执行时必须原样填写：{CONFIRM_TEXT}")
    parser.add_argument("--actor-id", default="", help="执行审计使用的启用中超级管理员 UUID")
    parser.add_argument("--reason", default="用户授权清理测试环境日常检查和通报数据")
    parser.add_argument("--base", type=Path)
    parser.add_argument("--project")
    parser.add_argument("--backup-root", type=Path)
    parser.add_argument("--url")
    args = parser.parse_args()
    os.umask(0o077)
    if args.target == "local-postgres":
        base = (args.base or LOCAL_BASE).resolve()
        project = args.project or "local-postgres"
        backup_root = (args.backup_root or LOCAL_BACKUP_ROOT).resolve()
        url = args.url or "http://127.0.0.1:8088"
        if (base != LOCAL_BASE.resolve() or project != "local-postgres"
                or backup_root != LOCAL_BACKUP_ROOT.resolve()):
            raise CleanupError("本机 PostgreSQL 清理仅允许当前仓库、本地 four_check 数据库和固定临时备份目录")
        services = ()
    elif args.target == "local-compose":
        base = (args.base or LOCAL_BASE).resolve()
        project = args.project or LOCAL_PROJECT
        backup_root = (args.backup_root or LOCAL_BACKUP_ROOT).resolve()
        url = args.url or "http://127.0.0.1:8088"
        if base != LOCAL_BASE.resolve() or project != LOCAL_PROJECT:
            raise CleanupError("本地清理仅允许当前仓库根目录和由仓库目录名确定的 Compose 项目")
        ensure_local_compose_context()
        services = LOCAL_SERVICES
    else:
        base = (args.base or DEFAULT_BASE).resolve()
        project = args.project or DEFAULT_PROJECT
        backup_root = (args.backup_root or DEFAULT_BACKUP_ROOT).resolve()
        url = args.url or "http://127.0.0.1:8088"
        if project != DEFAULT_PROJECT or base != DEFAULT_BASE:
            raise CleanupError("此清理脚本仅允许当前腾讯云 IP 试运行部署路径和 Compose 项目")
        services = ALL_SERVICES
    if not args.reason.strip() or len(args.reason) > 255:
        raise CleanupError("reason 必须为 1–255 字符")
    target = inspect_local_postgres(base) if args.target == "local-postgres" else inspect(base, project, services)
    if not args.apply:
        counts = dict(target["counts"])
        storage_note = None
        if args.target == "local-postgres":
            counts["file_object_bytes_left_in_object_storage"] = counts.pop("file_object_bytes_to_remove")
            storage_note = "只清理本地 PostgreSQL 行与文件元数据；MinIO/对象存储不可通过此目标访问，相关对象会保留。"
        print(json.dumps({
            "mode": f"preview-{args.target}",
            "composeProject": project if args.target != "local-postgres" else None,
            "database": "four_check",
            "migrations": target["versions"],
            "counts": counts,
            "filesByRetention": {
                kind: sum(1 for item in target["files"] if item["retention"] == kind)
                for kind in sorted({item["retention"] for item in target["files"]})
            },
            "objectStorageNote": storage_note,
            "templateRepublishRequired": True,
            "willPreserve": ["accounts", "permissions", "master data", "academic calendar",
                             "inspection types", "reason dictionaries", "report obligations",
                             "morning schedules", "audit logs", "master-data/academic-week import files"],
        }, ensure_ascii=False, indent=2))
        print(f"\n只读预览。要清理，请附加 --apply --confirm '{CONFIRM_TEXT}' --actor-id '<管理员 UUID>'。")
        return
    if args.confirm != CONFIRM_TEXT:
        raise CleanupError(f"执行必须提供 --confirm '{CONFIRM_TEXT}'")
    if not args.actor_id:
        raise CleanupError("执行必须提供 --actor-id")
    result = apply_cleanup(base, project, backup_root, args.actor_id,
                           args.reason.strip(), url, local_compose=args.target == "local-compose",
                           local_postgres=args.target == "local-postgres")
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except CleanupError as error:
        raise SystemExit(str(error))
