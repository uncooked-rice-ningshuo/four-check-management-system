#!/usr/bin/env python3
"""Upgrade the current Tencent Cloud IP trial from r5 to the 2026-11-02 release."""

from __future__ import annotations

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

from clean_test_business_data import (
    CleanupError,
    DEFAULT_BACKUP_ROOT,
    DEFAULT_BASE,
    DEFAULT_PROJECT,
    PURGE_ORDER,
    inspect as inspect_cleanup,
)


GIB = 1024**3
TEMPLATE_TYPES = (
    "ATTENDANCE_RATE",
    "UNATTENDED_CLASSROOM",
    "CLASSROOM_HYGIENE",
    "MORNING_ATTENDANCE",
    "DORMITORY_HYGIENE",
    "LATE_RETURN",
    "PUBLIC_DORMITORY_HYGIENE",
    "EVENING_SELF_STUDY",
)


class UpdateError(RuntimeError):
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
        raise UpdateError(f"命令失败（退出码 {result.returncode}）：{command[0]} {command[1]}")
    return result.stdout or b""


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def compose_command(base: Path, project: str, compose_file: Path | None = None) -> list[str]:
    return [
        "docker", "compose", "--project-directory", str(base), "-p", project,
        "--env-file", str(base / ".env"), "-f", str(compose_file or (base / "compose.yaml")),
    ]


def verify_package(release: Path) -> set[str]:
    manifest = release / "SHA256SUMS"
    if not manifest.is_file():
        raise UpdateError("更新包缺少 SHA256SUMS")
    expected_files = set()
    for line in manifest.read_text().splitlines():
        try:
            checksum, name = line.split("  ", 1)
        except ValueError as error:
            raise UpdateError("SHA256SUMS 格式错误") from error
        path = (release / name).resolve()
        if not path.is_relative_to(release.resolve()) or not path.is_file() or sha256(path) != checksum:
            raise UpdateError(f"更新包校验失败：{name}")
        expected_files.add(name)
    return expected_files


def service_container(compose: list[str], project: str, service: str) -> tuple[str, dict]:
    ids = run(compose + ["ps", "-a", "-q", service]).decode().split()
    if len(ids) != 1:
        raise UpdateError(f"{service} 服务容器数量不是 1")
    info = json.loads(run(["docker", "inspect", ids[0]]))[0]
    if info["Config"]["Labels"].get("com.docker.compose.project") != project:
        raise UpdateError(f"{service} 容器不属于目标 Compose 项目")
    if info["State"].get("Status") != "running" or info["State"].get("Health", {}).get("Status") != "healthy":
        raise UpdateError(f"{service} 容器未 healthy")
    return ids[0], info


def psql(pg_id: str, query: str) -> str:
    return run([
        "docker", "exec", pg_id, "psql", "-X", "-q", "-U", "four_check",
        "-d", "four_check", "-v", "ON_ERROR_STOP=1", "-Atqc", query,
    ]).decode().strip()


def verify_image(image: str, expected: dict) -> None:
    # Docker 27 does not implement `docker image inspect --platform`; the
    # unique tag plus source revision label and architecture are portable.
    info = json.loads(run(["docker", "image", "inspect", image]))[0]
    labels = info.get("Config", {}).get("Labels") or {}
    if (
        image not in (info.get("RepoTags") or [])
        or info.get("Os") != "linux"
        or info.get("Architecture") != "amd64"
        or labels.get("org.opencontainers.image.revision") != expected["sourceCommit"]
    ):
        raise UpdateError(f"镜像缺失、标签、来源提交或架构不符：{image}")


def verify_mount(info: dict, destination: str, name: str, service: str) -> None:
    mounts = [item for item in info["Mounts"] if item["Destination"] == destination]
    if len(mounts) != 1 or mounts[0].get("Name") != name:
        raise UpdateError(f"{service} 数据卷与当前腾讯云部署不一致")


def migration_names(release: Path) -> list[str]:
    return sorted(path.name for path in (release / "migrations").glob("*.sql"))


def preflight(release: Path, base: Path, project: str, actor_id: str) -> tuple[dict, list[str]]:
    files = verify_package(release)
    manifest = json.loads((release / "release.json").read_text())
    expected_files = {
        "images.tar.gz", "release.json", "baseline-migrations.txt", "UPDATE.md",
        "clean_test_business_data.py", "test-business-data-cleanup.md", "upgrade-r6.py",
    }
    expected_files.update(f"migrations/{name}" for name in manifest["allMigrations"])
    if not expected_files.issubset(files):
        raise UpdateError("更新包缺少清单、脚本或迁移文件")
    if not (base / ".env").is_file() or not (base / "compose.yaml").is_file():
        raise UpdateError("目标部署目录或 .env 缺失")

    compose = compose_command(base, project)
    config = json.loads(run(compose + ["config", "--format", "json"]))
    current = {}
    for service in ("postgres", "minio", "api", "web"):
        cid, info = service_container(compose, project, service)
        current[service] = {"id": cid, "info": info}
    for service, destination, volume in (
        ("postgres", "/var/lib/postgresql/data", f"{project}_postgres_data"),
        ("minio", "/data", f"{project}_minio_data"),
    ):
        verify_mount(current[service]["info"], destination, volume, service)

    for service in ("api", "web"):
        old_image = manifest["baselineImages"][service]
        if current[service]["info"]["Config"]["Image"] != old_image:
            raise UpdateError(f"运行中的 {service} 镜像不符合更新包基线：{old_image}")
        if config["services"][service].get("image") != old_image:
            raise UpdateError(f"Compose 中的 {service} 镜像不符合更新包基线：{old_image}")
    for image, details in manifest["imageMetadata"].items():
        verify_image(image, details)

    baseline = (release / "baseline-migrations.txt").read_text().splitlines()
    current_migrations = psql(current["postgres"]["id"], "SELECT version FROM schema_migrations ORDER BY version").splitlines()
    if current_migrations != baseline:
        raise UpdateError(
            "数据库迁移账本与更新包基线不一致（期望 "
            f"{baseline[0]}–{baseline[-1]}）；停止并核对，不要强行迁移"
        )

    for name in baseline:
        target = base / "migrations" / name
        packaged = release / "migrations" / name
        if not target.is_file() or sha256(target) != sha256(packaged):
            raise UpdateError(f"服务器已有迁移缺失或内容不符：{name}")
    for name in manifest["pendingMigrations"]:
        target = base / "migrations" / name
        packaged = release / "migrations" / name
        if target.exists() and sha256(target) != sha256(packaged):
            raise UpdateError(f"服务器存在同名但内容不同的迁移：{name}")

    try:
        from clean_test_business_data import check_actor

        check_actor(current["postgres"]["id"], actor_id)
        cleanup = inspect_cleanup(base, project)
    except CleanupError as error:
        raise UpdateError(str(error)) from error
    not_empty = {name: cleanup["counts"].get(name, 0) for name in PURGE_ORDER
                 if cleanup["counts"].get(name, 0) != 0}
    if cleanup["counts"].get("files_to_remove", 0):
        not_empty["files_to_remove"] = cleanup["counts"]["files_to_remove"]
    if not_empty:
        raise UpdateError("业务数据尚未清理；先运行 clean_test_business_data.py --apply：" + json.dumps(not_empty, ensure_ascii=False))

    db_size = int(psql(current["postgres"]["id"], "SELECT pg_database_size(current_database())"))
    minio_bytes = int(run(["docker", "exec", current["minio"]["id"], "du", "-sk", "/data"]).split()[0]) * 1024
    if shutil.disk_usage(base).free < max(3 * GIB, 2 * (db_size + minio_bytes) + GIB):
        raise UpdateError("磁盘空间不足以保存部署备份并升级")

    check_summary = {
        "compose": compose,
        "containers": current,
        "manifest": manifest,
        "currentMigrations": current_migrations,
        "cleanupCounts": cleanup["counts"],
    }
    return check_summary, baseline


def verify_tar(path: Path) -> None:
    try:
        with tarfile.open(path, "r:gz") as archive:
            for member in archive:
                if member.isfile():
                    stream = archive.extractfile(member)
                    if stream is None:
                        raise UpdateError("MinIO 备份归档包含不可读取文件")
                    with stream:
                        while stream.read(1024 * 1024):
                            pass
    except (OSError, tarfile.TarError) as error:
        raise UpdateError("MinIO 备份校验失败") from error


def write_checksums(directory: Path) -> None:
    lines = []
    for path in sorted(directory.rglob("*")):
        if path.is_file() and path.name != "SHA256SUMS":
            lines.append(f"{sha256(path)}  {path.relative_to(directory)}")
    (directory / "SHA256SUMS").write_text("\n".join(lines) + "\n")


def make_backup(summary: dict, base: Path, project: str, backup_root: Path) -> Path:
    backup_root.mkdir(parents=True, mode=0o700, exist_ok=True)
    os.chmod(backup_root, 0o700)
    tag = summary["manifest"]["tag"]
    backup = backup_root / f"before-{tag}-{time.strftime('%Y%m%d-%H%M%S', time.gmtime())}"
    backup.mkdir(mode=0o700, exist_ok=False)
    os.chmod(backup, 0o700)
    images = {
        service: {"image": data["info"]["Config"]["Image"], "imageId": data["info"]["Image"]}
        for service, data in summary["containers"].items()
    }
    (backup / "original-images.json").write_text(json.dumps(images, indent=2) + "\n")
    for name in (".env", "compose.yaml", "nginx.conf", "migrate.sh", "app-policy.json"):
        source = base / name
        if source.exists():
            shutil.copy2(source, backup / name)
            (backup / name).chmod(0o600)
    shutil.copytree(base / "migrations", backup / "migrations")

    compose = summary["compose"]
    pg_id = summary["containers"]["postgres"]["id"]
    minio_id = summary["containers"]["minio"]["id"]
    try:
        run(compose + ["stop", "-t", "30", "web", "api"])
        with (backup / "database.dump.part").open("wb") as stream:
            run(["docker", "exec", pg_id, "pg_dump", "-U", "four_check", "-d", "four_check", "-Fc"], stdout=stream)
        with (backup / "database.dump.part").open("rb") as stream:
            run(["docker", "exec", "-i", pg_id, "pg_restore", "--list"], input_bytes=stream.read())
        (backup / "database.dump.part").replace(backup / "database.dump")

        run(compose + ["stop", "-t", "30", "minio"])
        with (backup / "minio-data.tar.gz.part").open("wb") as archive:
            run([
                "docker", "run", "--rm", "--pull=never", "--network", "none", "--user", "0",
                "--mount", f"type=volume,src={project}_minio_data,dst=/data,readonly",
                "postgres:16-alpine", "tar", "-C", "/data", "-czf", "-", ".",
            ], stdout=archive)
        verify_tar(backup / "minio-data.tar.gz.part")
        (backup / "minio-data.tar.gz.part").replace(backup / "minio-data.tar.gz")
        run(compose + ["start", "minio"])
    except Exception:
        subprocess.run(compose + ["up", "-d", "--no-deps", "--pull", "never", "api", "web"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
        subprocess.run(compose + ["start", "minio"], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False)
        raise
    metadata = {
        "tag": tag,
        "sourceCommit": summary["manifest"]["sourceCommit"],
        "createdAtUtc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "databaseSha256": sha256(backup / "database.dump"),
        "minioArchiveSha256": sha256(backup / "minio-data.tar.gz"),
    }
    (backup / "backup.json").write_text(json.dumps(metadata, indent=2) + "\n")
    write_checksums(backup)
    return backup


def wait_for_ready(compose: list[str], project: str, url: str) -> None:
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for _ in range(90):
        try:
            with opener.open(url.rstrip("/") + "/health/ready", timeout=3) as response:
                if response.status != 200:
                    raise OSError("ready check is not 200")
            for service in ("postgres", "minio", "api", "web"):
                service_container(compose, project, service)
            return
        except (OSError, UpdateError, ValueError):
            time.sleep(2)
    raise UpdateError("新服务未在 3 分钟内通过 healthy 和 ready 检查")


def publish_templates(summary: dict, actor_id: str, backup: Path) -> None:
    output = []
    for type_code in TEMPLATE_TYPES:
        reason = f"r6 部署后按当前检查规则重建 {type_code} 导入模板"
        result = subprocess.run(
            summary["compose"] + [
                "exec", "-T", "api", "publish-attendance-template", "--type-code", type_code,
                "--apply", "--actor-id", actor_id, "--reason", reason,
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        output.append(f"=== {type_code} exit={result.returncode} ===\n" + result.stdout.decode(errors="replace"))
        if result.returncode:
            (backup / "template-publication.log").write_text("\n".join(output))
            raise UpdateError(f"{type_code} 模板发布失败；日志保存在备份目录 template-publication.log")
    (backup / "template-publication.log").write_text("\n".join(output))


def verify_templates(pg_id: str) -> None:
    codes = "'" + "','".join(TEMPLATE_TYPES) + "'"
    query = f"""
SELECT count(*) || '|' || count(*) FILTER (WHERE t.status='PUBLISHED' AND f.status='AVAILABLE' AND f.retention_type='TEMPLATE')
FROM import_templates t
JOIN inspection_types i ON i.id=t.inspection_type_id
JOIN files f ON f.id=t.file_id
WHERE i.code IN ({codes}) AND i.enabled
"""
    values = psql(pg_id, query).split("|")
    if values != [str(len(TEMPLATE_TYPES)), str(len(TEMPLATE_TYPES))]:
        raise UpdateError("八类检查模板发布状态未全部达到 PUBLISHED/AVAILABLE")


def apply_update(summary: dict, base: Path, project: str, actor_id: str,
                 backup_root: Path, url: str) -> None:
    compose = summary["compose"]
    manifest = summary["manifest"]
    backup = None
    started = False
    compose_changed = False
    app_healthy = False
    try:
        backup = make_backup(summary, base, project, backup_root)
        started = True
        for name in manifest["pendingMigrations"]:
            source = Path(__file__).resolve().parent / "migrations" / name
            target = base / "migrations" / name
            if not target.exists():
                shutil.copy2(source, target)
        migration = subprocess.run(
            compose + ["run", "--rm", "--no-deps", "-T", "migrate"],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        (backup / "migration.log").write_bytes(migration.stdout)
        if migration.returncode:
            raise UpdateError("数据库迁移失败；迁移日志和验证过的备份已保留")

        pg_id = summary["containers"]["postgres"]["id"]
        expected = manifest["allMigrations"]
        actual = psql(pg_id, "SELECT version FROM schema_migrations ORDER BY version").splitlines()
        if actual != expected:
            raise UpdateError("迁移后账本与 001–037 目标不一致")

        compose_path = base / "compose.yaml"
        original = compose_path.read_text()
        updated = original
        for service, old_image in manifest["baselineImages"].items():
            target_image = manifest["targetImages"][service]
            if updated.count(old_image) != 1:
                raise UpdateError(f"Compose 中 {service} 基线镜像引用不唯一")
            updated = updated.replace(old_image, target_image)
        temporary = base / "compose.yaml.r6.tmp"
        with temporary.open("x") as stream:
            stream.write(updated)
        temp_compose = compose_command(base, project, temporary) + ["config", "--quiet"]
        run(temp_compose)
        os.replace(temporary, compose_path)
        compose_changed = True
        compose = compose_command(base, project)
        run(compose + ["up", "-d", "--no-deps", "--pull", "never", "api", "web"])
        wait_for_ready(compose, project, url)
        app_healthy = True
        publish_templates(summary, actor_id, backup)
        verify_templates(summary["containers"]["postgres"]["id"])
        wait_for_ready(compose, project, url)
        write_checksums(backup)
        print(f"r6 部署成功；备份：{backup}", flush=True)
        pending = manifest["pendingMigrations"]
        migration_range = f"{pending[0].split('_', 1)[0]}–{pending[-1].split('_', 1)[0]}"
        print(
            f"迁移 {migration_range}、API/Web healthy、ready=200 和八类导入模板发布均已核验。",
            flush=True,
        )
    except Exception:
        if started and not app_healthy:
            try:
                active_compose = compose_command(base, project)
                subprocess.run(active_compose + ["stop", "-t", "30", "web", "api"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
            except Exception:
                pass
        if backup is not None:
            print(f"升级未完成；备份目录：{backup}", flush=True)
            if compose_changed:
                if app_healthy:
                    print("Compose 已切到 r6，API/Web healthy；模板发布不完整。只补发日志中失败的模板，不重跑迁移。", flush=True)
                else:
                    print("Compose 已切到 r6。API/Web 保持停止；根据 migration.log 恢复，勿重跑旧版本。", flush=True)
        raise


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="执行迁移、镜像切换和模板发布")
    parser.add_argument("--base", type=Path, default=DEFAULT_BASE)
    parser.add_argument("--project", default=DEFAULT_PROJECT)
    parser.add_argument("--actor-id", required=True, help="启用中超级管理员 UUID，用于模板发布审计")
    parser.add_argument("--backup-root", type=Path, default=DEFAULT_BACKUP_ROOT)
    parser.add_argument("--url", default="http://127.0.0.1:8088")
    args = parser.parse_args()
    os.umask(0o077)
    base = args.base.resolve()
    if base != DEFAULT_BASE or args.project != DEFAULT_PROJECT:
        raise UpdateError("此更新包仅用于当前对话中的腾讯云 IP 试运行部署")
    try:
        normalized_actor = str(__import__("uuid").UUID(args.actor_id))
    except ValueError as error:
        raise UpdateError("actor-id 必须是 UUID") from error

    release = Path(__file__).resolve().parent
    lock_path = base / ".upgrade-r6.lock"
    if args.apply:
        lock = lock_path.open("a")
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise UpdateError("已有更新任务正在执行") from error
    summary, _ = preflight(release, base, args.project, normalized_actor)
    print("只读预检通过。", flush=True)
    baseline = summary["manifest"]["baselineMigrations"]
    pending = summary["manifest"]["pendingMigrations"]
    migration_range = f"{pending[0].split('_', 1)[0]}–{pending[-1].split('_', 1)[0]}"
    print(
        "当前镜像："
        + " / ".join(summary["manifest"]["baselineImages"].values())
        + f"；数据库：{baseline[0].split('_', 1)[0]}–{baseline[-1].split('_', 1)[0]}；"
        + f"目标：r6 + 迁移 {migration_range}。",
        flush=True,
    )
    print("已清洗数据计数：" + json.dumps(summary["cleanupCounts"], ensure_ascii=False), flush=True)
    if not args.apply:
        print("未修改服务。维护窗口确认后附加 --apply 执行。", flush=True)
        return
    apply_update(summary, base, args.project, normalized_actor, args.backup_root.resolve(), args.url)


if __name__ == "__main__":
    try:
        main()
    except (UpdateError, CleanupError) as error:
        raise SystemExit(str(error))
