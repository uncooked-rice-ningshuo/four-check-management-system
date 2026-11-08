#!/usr/bin/env bash
# Build an offline Linux/amd64 update package on the development Mac/build host.
# App tests are intentionally not run here; the user has already confirmed them.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"

release_tag="${RELEASE_TAG:-20261109-r7}"
package_id="${PACKAGE_ID:-20261109-r7a}"
baseline_tag="20261102-r6"
api_image="four-check-api:ip-trial-$release_tag"
web_image="four-check-web:ip-trial-$release_tag"
source_commit="$(git rev-parse HEAD)"
package_root="$repo_root/output/releases/four-check-update-$package_id"
release_dir="$package_root/release"

if [[ ! "$release_tag" =~ ^[0-9]{8}-r[0-9]+$ || ! "$package_id" =~ ^[0-9]{8}-r[0-9]+[a-z]?$ ]]; then
  echo "release tag 或 package ID 格式无效" >&2
  exit 1
fi
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

cp deploy/ip-trial/{upgrade-r7.py,clean_test_business_data.py,UPGRADE-R7.md} "$release_dir/"
cp deploy/ip-trial/UPGRADE-R7.md "$release_dir/UPDATE.md"
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

docker image save "$api_image" "$web_image" | gzip -n > "$release_dir/images.tar.gz"

python3 - "$release_dir" "$api_image" "$web_image" "$source_commit" "$release_tag" "$package_id" "$baseline_tag" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

release = Path(sys.argv[1])
images = sys.argv[2:4]
source_commit, tag, package_id, baseline_tag = sys.argv[4:8]
all_migrations = sorted(path.name for path in (release / "migrations").glob("*.sql"))
baseline = [name for name in all_migrations if int(name.split("_", 1)[0]) <= 37]
pending = [name for name in all_migrations if int(name.split("_", 1)[0]) >= 38]
expected_pending = [
    "038_rename_sports_department.sql",
    "039_inspection_field_alignment.sql",
    "040_unattended_penalty_and_dedup.sql",
    "041_unattended_student_period_dedup.sql",
    "042_semester_report_grades.sql",
    "043_report_output_files.sql",
    "044_report_output_word_layout.sql",
    "045_report_output_cover_metadata.sql",
    "046_report_output_format_defaults.sql",
    "047_classroom_hygiene_report_output.sql",
    "048_public_dormitory_hygiene_report_output.sql",
    "049_report_output_week_dates.sql",
]
if not baseline or baseline[-1] != "037_daily_inspection_adjustments.sql" or pending != expected_pending:
    raise SystemExit("迁移清单未覆盖 r6a 基线 001–037 和当前迁移 038–049")
(release / "baseline-migrations.txt").write_text("\n".join(baseline) + "\n")

manifest = {
    "tag": tag,
    "packageId": package_id,
    "sourceCommit": source_commit,
    "baselineImages": {
        "api": f"four-check-api:ip-trial-{baseline_tag}",
        "web": f"four-check-web:ip-trial-{baseline_tag}",
    },
    "targetImages": {"api": images[0], "web": images[1]},
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

lines = []
for path in sorted(release.rglob("*")):
    if path.is_file() and path.name != "SHA256SUMS":
        lines.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.relative_to(release)}")
(release / "SHA256SUMS").write_text("\n".join(lines) + "\n")
PY

archive="$repo_root/output/releases/four-check-update-$package_id.tar.gz"
# The extracted upload directory is already named `release`; archive its
# contents at the tar root so server paths are not nested as release/release.
tar -czf "$archive" -C "$release_dir" .
(cd "$(dirname "$archive")" && shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256")
echo "更新目录：$release_dir"
echo "外层包：$archive"
echo "校验文件：$archive.sha256"
