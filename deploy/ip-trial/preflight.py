#!/usr/bin/env python3
"""Read-only first-install checks on the Ubuntu target; no credentials are read."""
import ipaddress
import json
import platform
import shutil
import socket
import subprocess
from pathlib import Path

errors = []


def docker(*args):
    result = subprocess.run(["docker", *args], capture_output=True, text=True)
    if result.returncode:
        raise SystemExit("Docker 检查失败，请确认 Docker 已启动且当前账号有访问权限。")
    return result.stdout


if platform.system() != "Linux" or platform.machine() != "x86_64":
    errors.append("此发布包面向 Linux x86_64。")
print("Docker:", docker("version", "--format", "{{.Server.Version}}").strip())
print("Compose:", docker("compose", "version", "--short").strip())
network_ids = docker("network", "ls", "-q").split()
target = ipaddress.ip_network("172.30.88.0/24")
if network_ids:
    for network in json.loads(docker("network", "inspect", *network_ids)):
        for item in network.get("IPAM", {}).get("Config") or []:
            if item.get("Subnet") and target.overlaps(ipaddress.ip_network(item["Subnet"])):
                errors.append("部署网段与已有 Docker 网络重叠：" + network["Name"])
routes = subprocess.run(["ip", "-j", "route", "show"], capture_output=True, text=True)
if routes.returncode:
    errors.append("无法检查主机路由，请先核对 172.30.88.0/24 是否与 VPC/VPN 冲突。")
else:
    for route in json.loads(routes.stdout):
        destination = route.get("dst", "default")
        if destination != "default":
            network = ipaddress.ip_network(destination, strict=False)
            if network.prefixlen and target.overlaps(network):
                errors.append("部署网段与主机路由重叠：" + destination)
try:
    with socket.socket() as listener:
        listener.bind(("0.0.0.0", 8088))
except OSError:
    errors.append("8088 端口已被占用。")
gib = 1024 ** 3
root = Path(__file__).resolve().parent
docker_root = Path(docker("info", "--format", "{{.DockerRootDir}}").strip())
for label, path in (("发布目录", root), ("Docker 数据目录", docker_root)):
    free = shutil.disk_usage(path).free
    print(f"{label}可用空间：{free / gib:.1f} GiB")
    if free < 3 * gib:
        errors.append(label + "可用空间不足 3 GiB，请先扩容；不要删除现有业务数据。")
print("本检查不证明内存容量或业务上线验收通过。")
if errors:
    for error in errors:
        print("需要处理：", error)
    raise SystemExit(1)
print("首次安装预检通过：架构、8088 端口、网段和磁盘检查通过。")
