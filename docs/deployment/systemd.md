# 使用 systemd 运行 mihomo-exporter

本文适用于使用 systemd 的 Linux 发行版。示例将程序安装到 `/usr/local/bin`，使用专用系统用户运行，并把含有 Mihomo Secret 的配置保存在权限受限的环境文件中。

## 1. 安装程序

从 [GitHub Releases](https://github.com/tiancheng91/mihomo-exporter/releases) 下载适合当前架构的压缩包并解压，然后安装二进制：

```bash
sudo install -m 0755 mihomo-exporter /usr/local/bin/mihomo-exporter
/usr/local/bin/mihomo-exporter --version
```

创建不允许登录的专用用户：

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin mihomo-exporter
```

部分发行版的 `nologin` 位于 `/sbin/nologin`，请按系统实际路径调整。用户已经存在时无需重复创建。

## 2. 创建环境文件

```bash
sudo install -d -m 0750 -o root -g mihomo-exporter /etc/mihomo-exporter
sudo install -m 0640 -o root -g mihomo-exporter /dev/null /etc/mihomo-exporter/mihomo-exporter.env
sudoedit /etc/mihomo-exporter/mihomo-exporter.env
```

写入需要的配置：

```ini
MIHOMO_URL=http://127.0.0.1:9090
MIHOMO_SECRET=your-secret

OUTPUT_PROMETHEUS=true
PROMETHEUS_LISTEN=:9091

OUTPUT_TELEGRAF=false
CONNECTION_INTERVAL=1s
LOG_LEVEL=info
```

环境文件中直接写 `KEY=value`，不要添加 `export`。值包含空格或 `#` 等特殊字符时，应使用双引号包裹。

如果 Prometheus 也在本机运行，可以把监听地址限制为 `127.0.0.1:9091`。如果 Mihomo 在另一台机器上，请确保其 External Controller 配置了 Secret，并通过可信网络访问。

## 3. 创建服务单元

创建 `/etc/systemd/system/mihomo-exporter.service`：

```ini
[Unit]
Description=Mihomo Prometheus and Telegraf Exporter
Documentation=https://github.com/tiancheng91/mihomo-exporter
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=mihomo-exporter
Group=mihomo-exporter
EnvironmentFile=/etc/mihomo-exporter/mihomo-exporter.env
ExecStart=/usr/local/bin/mihomo-exporter
Restart=on-failure
RestartSec=5s

# Service hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
CapabilityBoundingSet=
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
UMask=0077

[Install]
WantedBy=multi-user.target
```

如果 Mihomo 本身也是 systemd 服务，可以在 `[Unit]` 中加入：

```ini
Wants=mihomo.service
After=mihomo.service
```

Exporter 自带断线重连，因此 Mihomo 晚于 exporter 就绪或运行中重启时无需重启 exporter。

## 4. 启动和检查

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mihomo-exporter
sudo systemctl status mihomo-exporter
curl --fail http://127.0.0.1:9091/healthz
```

查看实时日志：

```bash
sudo journalctl -u mihomo-exporter -f
```

修改环境文件后重启服务：

```bash
sudo systemctl restart mihomo-exporter
```

## 升级

下载并解压新版本后，替换程序并重启服务：

```bash
sudo systemctl stop mihomo-exporter
sudo install -m 0755 mihomo-exporter /usr/local/bin/mihomo-exporter
sudo systemctl start mihomo-exporter
```

使用以下命令确认实际运行版本：

```bash
/usr/local/bin/mihomo-exporter --version
```
