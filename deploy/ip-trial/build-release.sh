#!/usr/bin/env bash
# Run on the development Mac/build host, never on the small target server.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
docker info >/dev/null
proxy_args=(--progress=plain)
if [ -n "${CLIENT_PROXY:-}" ]; then
  export HTTP_PROXY="$CLIENT_PROXY" HTTPS_PROXY="$CLIENT_PROXY"
fi
if [ -n "${BUILD_PROXY:-}" ]; then
  proxy_args+=(--build-arg "HTTP_PROXY=$BUILD_PROXY" --build-arg "HTTPS_PROXY=$BUILD_PROXY")
fi

docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  -f deploy/ip-trial/Dockerfile.api -t four-check-api:ip-trial-20260922 .
docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  -f deploy/ip-trial/Dockerfile.web -t four-check-web:ip-trial-20260922 .
docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  -f deploy/ip-trial/Dockerfile.minio -t four-check-minio:RELEASE.2025-10-15T17-29-55Z .
docker pull --platform linux/amd64 registry-1.docker.io/library/postgres:16-alpine
docker tag registry-1.docker.io/library/postgres:16-alpine postgres:16-alpine
docker buildx build --platform linux/amd64 --load "${proxy_args[@]}" \
  -f deploy/ip-trial/Dockerfile.mc -t four-check-mc:RELEASE.2025-08-13T08-35-41Z .

images=(
  four-check-api:ip-trial-20260922
  four-check-web:ip-trial-20260922
  four-check-minio:RELEASE.2025-10-15T17-29-55Z
  postgres:16-alpine
  four-check-mc:RELEASE.2025-08-13T08-35-41Z
)
for image in "${images[@]}"; do
  test "$(docker image inspect --platform linux/amd64 "$image" --format '{{.Os}}/{{.Architecture}}')" = linux/amd64
done

mkdir -p "$root/output/releases"
stage="$(mktemp -d "$root/output/releases/ip-trial-XXXXXXXX")"
cp deploy/ip-trial/{compose.yaml,nginx.conf,init-env.py,preflight.py,README.md} "$stage/"
cp -R server/migrations "$stage/migrations"
cp server/scripts/migrate.sh server/minio/app-policy.json "$stage/"
git rev-parse HEAD > "$stage/source-commit.txt"
git status --short > "$stage/source-worktree-status.txt"
docker image inspect --platform linux/amd64 "${images[@]}" --format '{{.Id}} {{.Os}}/{{.Architecture}} {{json .RepoTags}}' > "$stage/image-manifest.txt"

# Stream compression avoids keeping an additional uncompressed image archive.
docker image save --platform linux/amd64 "${images[@]}" | gzip > "$stage/images.tar.gz"
python3 - "$stage" <<'PY'
import hashlib
from pathlib import Path
import sys
root = Path(sys.argv[1])
with (root / 'SHA256SUMS').open('w') as manifest:
    for path in sorted(root.rglob('*')):
        if path.is_file() and path.name != 'SHA256SUMS':
            digest = hashlib.sha256()
            with path.open('rb') as source:
                for chunk in iter(lambda: source.read(1024 * 1024), b''):
                    digest.update(chunk)
            manifest.write(f'{digest.hexdigest()}  {path.relative_to(root)}\n')
PY
printf '发布目录（未包含任何 .env）：%s\n' "$stage"
printf '上传前仍须进行隔离启动与业务验证。\n'
