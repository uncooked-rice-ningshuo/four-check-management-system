#!/usr/bin/env python3
"""Generate trial credentials on the target host without displaying secrets."""
import os
from pathlib import Path
import secrets

root = Path(__file__).resolve().parent
config = {
    "POSTGRES_PASSWORD": secrets.token_hex(24),
    "JWT_SECRET": secrets.token_hex(48),
    "BOOTSTRAP_ADMIN_LOGIN": "admin",
    "BOOTSTRAP_ADMIN_NAME": "系统管理员",
    "BOOTSTRAP_ADMIN_PASSWORD": secrets.token_hex(16),
    "MINIO_ROOT_USER": "root-" + secrets.token_hex(8),
    "MINIO_ROOT_PASSWORD": secrets.token_hex(24),
    "MINIO_ACCESS_KEY": "app-" + secrets.token_hex(8),
    "MINIO_SECRET_KEY": secrets.token_hex(24),
    "MINIO_BUCKET": "four-check-files",
}
try:
    fd = os.open(root / ".env", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    raise SystemExit(".env 已存在，未覆盖。请保留既有凭据。")
with os.fdopen(fd, "w", encoding="utf-8") as output:
    for key, value in config.items():
        output.write(f"{key}={value}\n")
print("已创建 .env（权限 600），未输出凭据。初始管理员信息请在服务器上私下查看。")
