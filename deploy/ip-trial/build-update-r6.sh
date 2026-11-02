#!/usr/bin/env bash
# Build the offline 2026-11-02 update package on the development Mac, never on the 2 GB server.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"

release_tag="20261102-r6"
package_id="20261102-r6a"
api_image="four-check-api:ip-trial-$release_tag"
web_image="four-check-web:ip-trial-$release_tag"
source_commit="$(git rev-parse HEAD)"
package_root="$repo_root/output/releases/four-check-update-$package_id"
release_dir="$package_root/release"

if [ -e "$package_root" ]; then
  echo "发布目录已存在，请先核对：$package_root" >&2
  exit 1
fi

docker info >/dev/null
mkdir -p "$release_dir/migrations"

proxy_args=(--progress=plain)
if [ -n "${CLIENT_PROXY:-}" ]; then
  export HTTP_PROXY="$CLIENT_PROXY" HTTPS_PROXY="$CLIENT_PROXY"
fi
if [ -n "${BUILD_PROXY:-}" ]; then
  proxy_args+=(--build-arg "HTTP_PROXY=$BUILD_PROXY" --build-arg "HTTPS_PROXY=$BUILD_PROXY")
fi

docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  --build-arg "SOURCE_COMMIT=$source_commit" \
  -f deploy/ip-trial/Dockerfile.api -t "$api_image" .
docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  --build-arg "SOURCE_COMMIT=$source_commit" \
  -f deploy/ip-trial/Dockerfile.web -t "$web_image" .

for image in "$api_image" "$web_image"; do
  details="$(docker image inspect --format '{{.Os}}/{{.Architecture}} {{index .Config.Labels "org.opencontainers.image.revision"}} {{json .RepoTags}}' "$image")"
  case "$details" in
    "linux/amd64 $source_commit "*) ;;
    *) echo "镜像架构、来源提交或 tag 检查失败：$image $details" >&2; exit 1 ;;
  esac
done

cp deploy/ip-trial/{upgrade-r6.py,clean_test_business_data.py,UPGRADE-R6.md} "$release_dir/"
cp docs/deployment/test-business-data-cleanup.md "$release_dir/"
for migration in server/migrations/*.sql; do
  cp "$migration" "$release_dir/migrations/"
done
git rev-parse HEAD > "$release_dir/source-commit.txt"
git status --short > "$release_dir/source-worktree-status.txt"
{
  for image in "$api_image" "$web_image"; do
    docker image inspect --format 'tag={{index .RepoTags 0}} id={{.Id}} platform={{.Os}}/{{.Architecture}} revision={{index .Config.Labels "org.opencontainers.image.revision"}}' "$image"
  done
} > "$release_dir/image-manifest.txt"

# Stream image export to gzip so the build host and the target need only one archive.
docker image save "$api_image" "$web_image" | gzip -n > "$release_dir/images.tar.gz"

python3 - "$release_dir" "$api_image" "$web_image" "$source_commit" "$release_tag" "$package_id" <<'PY'
import json
from pathlib import Path
import sys

release = Path(sys.argv[1])
images = sys.argv[2:4]
source_commit = sys.argv[4]
tag = sys.argv[5]
package_id = sys.argv[6]
all_migrations = sorted(path.name for path in (release / "migrations").glob("*.sql"))
baseline = [name for name in all_migrations if int(name.split("_", 1)[0]) <= 35]
pending = [name for name in all_migrations if int(name.split("_", 1)[0]) >= 36]
if not baseline or pending != ["036_report_office_correction.sql", "037_daily_inspection_adjustments.sql"]:
    raise SystemExit("迁移清单未覆盖 r5 基线 001–035 和当前迁移 036–037")
(release / "baseline-migrations.txt").write_text("\n".join(baseline) + "\n")
manifest = {
    "tag": tag,
    "sourceCommit": source_commit,
    "baselineImages": {
        "api": "four-check-api:ip-trial-20261031-r5",
        "web": "four-check-web:ip-trial-20261031-r5",
    },
    "targetImages": {
        "api": images[0],
        "web": images[1],
    },
    "imageMetadata": {
        image: {"sourceCommit": source_commit, "architecture": "linux/amd64"}
        for image in images
    },
    "allMigrations": all_migrations,
    "baselineMigrations": baseline,
    "pendingMigrations": pending,
    "templateTypes": [
        "ATTENDANCE_RATE", "UNATTENDED_CLASSROOM", "CLASSROOM_HYGIENE",
        "MORNING_ATTENDANCE", "DORMITORY_HYGIENE", "LATE_RETURN",
        "PUBLIC_DORMITORY_HYGIENE", "EVENING_SELF_STUDY",
    ],
    "cleanupScript": "clean_test_business_data.py",
}
(release / "release.json").write_text(json.dumps(manifest, indent=2) + "\n")

update = f"""# 腾讯云 IP 试运行更新包 {tag}

来源提交：`{source_commit}`。目标是当前 `/opt/four-check/ip-trial` r5 部署，不是新服务器。
本包包含 API/Web `linux/amd64` 离线镜像、迁移 036–037、数据清理脚本及 r6 更新器；不包含 `.env`。

## 更新顺序

1. 把外层包放入服务器 `/opt/four-check/updates/{package_id}`，校验外层 SHA-256，解压至全新 `release` 目录。
2. 在 `release` 中执行 `sha256sum -c SHA256SUMS`，再执行 `docker load -i images.tar.gz`。
3. 先预览清理范围：`python3 clean_test_business_data.py`。
4. 你已授权本次测试数据清洗。实际清理需使用管理员 UUID 和确认短语；脚本自动备份 PostgreSQL 与 MinIO，删除记录后保留审计，并把旧检查模板和其对象一并清理。
5. 清理完成后执行 `python3 upgrade-r6.py --actor-id '<启用中超级管理员 UUID>'` 做只读预检；输出确认基线 r5、迁移 001–035 和清理结果。
6. 维护窗口执行同一命令并附加 `--apply`。更新器备份后应用 036–037、切换 API/Web、验证健康状态，再用当前代码重新发布八类检查模板。
7. 以数据库迁移账本 001–037、四个常驻服务 healthy、`http://127.0.0.1:8088/health/ready` 返回 200 和八类模板 PUBLISHED/AVAILABLE 作为成功条件。

## 清理保留范围

清除日常检查记录、导入批次/明细、部门提交及不可变修订、周报和版本/审核/更正快照、旧检查导入模板及对应对象文件。保留账号、权限、主数据、学期/周次、检查类型、原因字典、周报应交配置、早签周计划、主数据/校历导入文件和审计日志。

清理脚本检查外键引用并拒绝清理清单外的新依赖；需要扩展业务表时，先更新 `deploy/ip-trial/clean_test_business_data.py` 的 `PURGE_ORDER`、文件引用范围和保留规则。清理不能替代数据库重建，失败时以备份目录恢复。
"""
(release / "UPDATE.md").write_text(update)

import hashlib
lines = []
for path in sorted(release.rglob("*")):
    if path.is_file() and path.name != "SHA256SUMS":
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        lines.append(f"{digest}  {path.relative_to(release)}")
(release / "SHA256SUMS").write_text("\n".join(lines) + "\n")
PY

archive="$repo_root/output/releases/four-check-update-$package_id.tar.gz"
tar -czf "$archive" -C "$package_root" release
(cd "$(dirname "$archive")" && shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256")
echo "更新目录：$release_dir"
echo "外层包：$archive"
echo "校验文件：$archive.sha256"
