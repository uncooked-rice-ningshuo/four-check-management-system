#!/usr/bin/env python3
"""Preflight or apply the 2026-10-25 r4 offline update to the IP trial."""

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


GIB = 1024**3


class UpdateError(RuntimeError):
    pass


def run(command, *, input_bytes=None, stdout=None):
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


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify_package(release):
    manifest_path = release / "SHA256SUMS"
    if not manifest_path.is_file():
        raise UpdateError("找不到包内 SHA256SUMS")
    expected = set()
    for line in manifest_path.read_text().splitlines():
        checksum, name = line.split("  ", 1)
        path = (release / name).resolve()
        if not path.is_relative_to(release.resolve()) or not path.is_file():
            raise UpdateError("更新包文件清单无效")
        if sha256(path) != checksum:
            raise UpdateError(f"更新包校验失败：{name}")
        expected.add(name)
    return expected


def compose_command(base, project, compose_file=None):
    return [
        "docker", "compose", "--project-directory", str(base), "-p", project,
        "--env-file", str(base / ".env"), "-f", str(compose_file or base / "compose.yaml"),
    ]


def one_container(compose, service):
    # Include stopped/created containers so an unavailable service is not
    # misreported as a duplicate merely because `compose ps -q` omits it.
    ids = run(compose + ["ps", "-a", "-q", service]).decode().split()
    if not ids:
        raise UpdateError(f"{service} 服务没有容器")
    if len(ids) != 1:
        raise UpdateError(f"{service} 服务有 {len(ids)} 个容器，要求恰好 1 个")
    info = json.loads(run(["docker", "inspect", ids[0]]))[0]
    status = info["State"].get("Status", "unknown")
    if status != "running":
        raise UpdateError(f"{service} 容器状态为 {status}，要求 running")
    if info["State"].get("Health", {}).get("Status") != "healthy":
        raise UpdateError(f"{service} 容器尚未 healthy")
    return ids[0], info


def sql(postgres_id, query):
    return run([
        "docker", "exec", postgres_id, "psql", "-U", "four_check", "-d", "four_check",
        "-v", "ON_ERROR_STOP=1", "-Atqc", query,
    ]).decode().strip()


def verify_image(image, expected):
    try:
        raw = run(["docker", "image", "inspect", "--platform", "linux/amd64", image])
    except UpdateError:
        raw = run(["docker", "image", "inspect", image])
    info = json.loads(raw)[0]
    acceptable_ids = expected.get("ids", [expected.get("id")])
    if info["Id"] not in acceptable_ids or info["Os"] != "linux" or info["Architecture"] != "amd64":
        raise UpdateError(f"镜像缺失、ID 不符或架构不符：{image}；请先 docker load")


def preflight(release, base, project, manifest):
    expected_files = verify_package(release)
    required = {"images.tar", "release.json", "baseline-migrations.txt", "UPDATE.md"}
    required.update(f"migrations/{item['name']}" for item in manifest["pendingMigrations"])
    if not required.issubset(expected_files):
        raise UpdateError("更新包缺少必要文件")
    if not (base / ".env").is_file() or not (base / "migrations").is_dir():
        raise UpdateError("目标部署目录或 .env 不完整")

    compose = compose_command(base, project)
    config = json.loads(run(compose + ["config", "--format", "json"]))
    containers = {}
    for service in ("postgres", "minio", "api", "web"):
        container_id, info = one_container(compose, service)
        if info["Config"]["Labels"].get("com.docker.compose.project") != project:
            raise UpdateError(f"{service} 不属于目标 Compose 项目")
        containers[service] = (container_id, info)

    for service, old_image in manifest["baselineImages"].items():
        if config["services"][service].get("image") != old_image:
            raise UpdateError(f"Compose 的 {service} 镜像不是 r3 基线")
        if containers[service][1]["Config"]["Image"] != old_image:
            raise UpdateError(f"运行中的 {service} 镜像不是 r3 基线")

    for service, target_image in manifest["targetImages"].items():
        verify_image(target_image, manifest["images"][target_image])

    for service, destination in (("postgres", "/var/lib/postgresql/data"), ("minio", "/data")):
        mounts = [m for m in containers[service][1]["Mounts"] if m["Destination"] == destination]
        if len(mounts) != 1 or mounts[0].get("Name") != f"{project}_{service}_data":
            raise UpdateError(f"{service} 数据卷与已确认部署不一致")

    pending = manifest["pendingMigrations"]
    for item in pending:
        local = base / "migrations" / item["name"]
        packaged = release / "migrations" / item["name"]
        if local.exists() and sha256(local) != sha256(packaged):
            raise UpdateError(f"服务器已有同名但内容不同的迁移：{item['name']}")

    current = sql(containers["postgres"][0], "SELECT version FROM schema_migrations ORDER BY version").splitlines()
    baseline = (release / "baseline-migrations.txt").read_text().splitlines()
    if current != baseline:
        raise UpdateError("数据库迁移账本不精确匹配 r3 基线；请停止并核对现状")

    db_size = int(sql(containers["postgres"][0], "SELECT pg_database_size(current_database())"))
    file_size = int(run(["docker", "exec", containers["minio"][0], "du", "-sk", "/data"]).split()[0]) * 1024
    if shutil.disk_usage(base).free < 3 * GIB + 2 * (db_size + file_size):
        raise UpdateError("磁盘空间不足：至少需要 3 GiB 加两倍数据库和文件数据大小")

    return compose, containers, current


def backup_and_apply(release, base, project, manifest, compose, containers, original_versions):
    backup = base.parent / "backups" / ("before-" + manifest["tag"] + "-" + time.strftime("%Y%m%d-%H%M%S"))
    backup.mkdir(parents=True, mode=0o700, exist_ok=False)
    os.chmod(backup, 0o700)
    (backup / "original-images.json").write_text(json.dumps({
        service: {"image": info["Config"]["Image"], "imageId": info["Image"]}
        for service, (_, info) in containers.items()
    }, indent=2))
    for name in (".env", "compose.yaml", "nginx.conf", "migrate.sh", "app-policy.json"):
        source = base / name
        if source.exists():
            shutil.copy2(source, backup / name)
            (backup / name).chmod(0o600)
    shutil.copytree(base / "migrations", backup / "migrations")
    print(f"备份目录：{backup}", flush=True)

    maintenance = False
    migration_started = False
    minio_stopped = False
    updated_compose = False
    try:
        maintenance = True
        run(compose + ["stop", "-t", "30", "web", "api"])

        with (backup / "database.dump").open("wb") as dump:
            run(["docker", "exec", containers["postgres"][0], "pg_dump", "-U", "four_check", "-d", "four_check", "-Fc"], stdout=dump)
        with (backup / "database.dump").open("rb") as dump:
            run(["docker", "exec", "-i", containers["postgres"][0], "pg_restore", "--list"], input_bytes=dump.read())

        run(compose + ["stop", "-t", "30", "minio"])
        minio_stopped = True
        volume = f"{project}_minio_data"
        with (backup / "minio-data.tar.gz").open("wb") as archive:
            run([
                "docker", "run", "--rm", "--pull=never", "--network", "none", "--user", "0",
                "--entrypoint", "tar", "--mount", f"type=volume,src={volume},dst=/data,readonly",
                "postgres:16-alpine", "-C", "/data", "-czf", "-", ".",
            ], stdout=archive)
        with tarfile.open(backup / "minio-data.tar.gz", "r:gz") as archive:
            for member in archive:
                if member.isfile():
                    with archive.extractfile(member) as stream:
                        while stream.read(1024 * 1024):
                            pass
        run(compose + ["start", "minio"])
        minio_stopped = False

        for item in manifest["pendingMigrations"]:
            source = release / "migrations" / item["name"]
            target = base / "migrations" / item["name"]
            if not target.exists():
                shutil.copy2(source, target)

        # The migration runner commits each file separately; any started run can
        # therefore leave earlier migrations applied, so never auto-resume r3.
        migration_started = True
        result = subprocess.run(compose + ["run", "--rm", "--no-deps", "-T", "migrate"],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        (backup / "migration.log").write_bytes(result.stdout)
        if result.returncode:
            raise UpdateError("迁移失败；迁移输出保存在备份目录 migration.log")

        expected_versions = sorted(original_versions + [item["name"] for item in manifest["pendingMigrations"]])
        actual_versions = sql(containers["postgres"][0], "SELECT version FROM schema_migrations ORDER BY version").splitlines()
        if actual_versions != expected_versions:
            raise UpdateError("迁移后账本与目标版本不一致")

        compose_path = base / "compose.yaml"
        original = compose_path.read_text()
        updated = original
        for service, old in manifest["baselineImages"].items():
            target = manifest["targetImages"][service]
            if updated.count(old) != 1:
                raise UpdateError(f"Compose 中 {service} 镜像标签不是唯一引用")
            updated = updated.replace(old, target)
        temporary = base / "compose.yaml.r4.tmp"
        with temporary.open("x") as stream:
            stream.write(updated)
        test_compose = compose_command(base, project, temporary)
        run(test_compose + ["config", "--quiet"])
        os.replace(temporary, compose_path)
        updated_compose = True
        compose = compose_command(base, project)
        run(compose + ["up", "-d", "--no-deps", "--pull", "never", "api", "web"])

        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(90):
            try:
                with opener.open("http://127.0.0.1:8088/health/ready", timeout=3) as response:
                    ready = response.status == 200
                healthy = all(one_container(compose, service)[1]["State"].get("Health", {}).get("Status") == "healthy"
                              for service in ("postgres", "minio", "api", "web"))
                if ready and healthy:
                    write_backup_checksums(backup)
                    print("r4 已启动：数据库迁移、API/Web 与四个服务健康检查通过。请继续浏览器业务验收。", flush=True)
                    return
            except (OSError, UpdateError, ValueError):
                pass
            time.sleep(2)
        raise UpdateError("新版服务未在等待期限内就绪")
    except Exception:
        if minio_stopped:
            subprocess.run(compose + ["start", "minio"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if maintenance:
            subprocess.run(compose + ["stop", "-t", "30", "web", "api"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if not migration_started:
                start_compose = compose_command(base, project)
                subprocess.run(start_compose + ["start", "api", "web"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        print(f"升级未完成；备份：{backup}", flush=True)
        if updated_compose:
            print("Compose 已指向 r4；API/Web 保持停止，请根据 migration.log 与备份处理。", flush=True)
        raise


def write_backup_checksums(backup):
    lines = []
    for path in sorted(backup.rglob("*")):
        if path.is_file() and path.name != "SHA256SUMS":
            lines.append(f"{sha256(path)}  {path.relative_to(backup)}")
    (backup / "SHA256SUMS").write_text("\n".join(lines) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="执行备份、迁移并切换 API/Web")
    parser.add_argument("--base", type=Path, default=Path("/opt/four-check/ip-trial"))
    parser.add_argument("--project", default="four-check-ip-trial")
    args = parser.parse_args()
    os.umask(0o077)
    release = Path(__file__).resolve().parent
    base = args.base.resolve()
    manifest = json.loads((release / "release.json").read_text())
    if args.apply:
        lock = (base / ".upgrade-r4.lock").open("a")
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise UpdateError("已有更新任务正在运行") from error
    compose, containers, versions = preflight(release, base, args.project, manifest)
    print("只读预检通过：r3 镜像、四服务健康、数据卷、迁移账本、r4 镜像与磁盘已核对。", flush=True)
    if not args.apply:
        print("尚未修改服务器。执行 python3 upgrade-r4.py --apply 才会备份并更新。")
        return
    backup_and_apply(release, base, args.project, manifest, compose, containers, versions)


if __name__ == "__main__":
    try:
        main()
    except (UpdateError, OSError, ValueError, KeyError) as error:
        raise SystemExit(str(error))
