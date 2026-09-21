# mihomo-exporter

`mihomo-exporter` 用于采集 [Mihomo](https://github.com/MetaCubeX/mihomo) 的实时流量和连接数据，并输出为 Prometheus 指标或推送给 Telegraf。

## 快速上手

如果 Mihomo 使用默认地址 `http://127.0.0.1:9090` 且没有设置 Secret，下载并解压程序后直接运行：

```bash
./mihomo-exporter
```

设置了 Secret 时也只需要一行命令：

```bash
MIHOMO_SECRET=your-secret ./mihomo-exporter
```

启动后访问：

- Metrics：`http://127.0.0.1:9091/metrics`
- 健康检查：`http://127.0.0.1:9091/healthz`

Mihomo 地址不同，可以直接通过参数指定：

```bash
./mihomo-exporter --mihomo-url=http://192.168.1.1:9090
```

它可以回答这些常见问题：

- Mihomo 当前的上传、下载速率是多少？
- 每个局域网客户端使用了多少流量？
- 每个代理节点承载了多少流量和连接？
- `/connections` 快照遗漏了多少短连接流量？

## 功能

- 全局上传、下载速率与累计流量
- 按 Client（来源 IP）统计流量和活跃连接
- 按 Proxy Node 统计流量和活跃连接
- 可选的 Client × Proxy 组合统计
- Prometheus `/metrics` 和 `/healthz`
- Telegraf Influx Line Protocol 推送
- 断线自动重连和安全的推送失败重试
- Linux、macOS、Windows 的 amd64/arm64 Release 包

## 使用前准备

请先在 Mihomo 配置中启用 External Controller：

```yaml
external-controller: 0.0.0.0:9090
secret: your-secret
```

如果 exporter 与 Mihomo 不在同一台机器，请确认防火墙和容器网络允许 exporter 访问该端口。不要把未设置密码的 External Controller 暴露到公网。

## 使用预编译程序

从 [GitHub Releases](https://github.com/tiancheng91/mihomo-exporter/releases) 下载对应系统和架构的压缩包，解压后运行：

```bash
MIHOMO_URL=http://127.0.0.1:9090 \
MIHOMO_SECRET=your-secret \
./mihomo-exporter
```

Release 页面同时提供 `checksums.txt`，可用于校验下载文件的 SHA-256。

Linux 用户如需开机启动和异常自动重启，可参考 [systemd 服务配置](docs/deployment/systemd.md)。

## Docker Compose

克隆仓库后，可以直接使用仓库内的 `docker-compose.yml`：

```bash
git clone https://github.com/tiancheng91/mihomo-exporter.git
cd mihomo-exporter
docker compose up -d --build
```

按实际环境修改 `docker-compose.yml` 中的连接信息：

```yaml
services:
  mihomo-exporter:
    build: .
    environment:
      MIHOMO_URL: http://host.docker.internal:9090
      MIHOMO_SECRET: your-secret
      OUTPUT_PROMETHEUS: "true"
      OUTPUT_TELEGRAF: "false"
    ports:
      - "9091:9091"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    restart: unless-stopped
```

该示例假设 Mihomo 运行在宿主机。如果 Mihomo 也运行在同一个 Compose 网络中，请把 `MIHOMO_URL` 改为对应的服务名，例如 `http://mihomo:9090`。

## Prometheus

Prometheus 输出默认启用，默认监听 `:9091`。在 Prometheus 中增加：

```yaml
scrape_configs:
  - job_name: mihomo
    static_configs:
      - targets: ["mihomo-exporter:9091"]
```

主要指标：

| 指标 | 说明 |
| --- | --- |
| `mihomo_upload_bytes_total` | exporter 启动后的全局上传字节 |
| `mihomo_download_bytes_total` | exporter 启动后的全局下载字节 |
| `mihomo_upload_bytes_per_second` | 当前上传速率 |
| `mihomo_download_bytes_per_second` | 当前下载速率 |
| `mihomo_active_connections` | 当前活跃连接数 |
| `mihomo_client_*` | 按来源 IP 聚合的指标 |
| `mihomo_proxy_*` | 按代理节点聚合的指标 |
| `mihomo_client_proxy_*` | 可选的 Client × Proxy 指标 |
| `mihomo_exporter_connection_tracking_ratio` | 连接快照流量与全局流量的比值 |
| `mihomo_exporter_stream_connected` | 两个数据流的连接状态 |
| `mihomo_exporter_telegraf_push_errors_total` | Telegraf 推送失败次数 |

完整指标可直接查看 `/metrics`，也可参考[架构文档](docs/design/architecture.md#prometheus-指标)。

## Telegraf

设置以下环境变量启用 Telegraf 推送：

```bash
OUTPUT_TELEGRAF=true \
TELEGRAF_URL=http://127.0.0.1:8186/mihomo \
FLUSH_INTERVAL=10s \
./mihomo-exporter
```

Telegraf 可以使用 `inputs.http_listener_v2` 接收数据，并将 `data_format` 设置为 `influx`。Exporter 会发送以下 measurement：

- `mihomo_traffic`
- `mihomo_client`
- `mihomo_proxy`
- `mihomo_client_proxy`，仅在 `ENABLE_CLIENT_PROXY=true` 时发送

每次发送的 `upload_bytes` 和 `download_bytes` 是当前 flush 周期的新增流量，不是进程启动后的累计值。推送失败时数据会保留到下次重试。

## 配置

所有配置均支持环境变量和 CLI 参数，优先级为 **环境变量 > CLI 参数 > 默认值**。环境变量名转为小写并把下划线换成短横线，即为对应参数，例如 `MIHOMO_URL` 对应 `--mihomo-url`。

```bash
./mihomo-exporter --mihomo-url=http://127.0.0.1:9090 --prometheus-listen=:9091
./mihomo-exporter --output-prometheus=false --output-telegraf --telegraf-url=http://127.0.0.1:8186/mihomo
./mihomo-exporter --help
./mihomo-exporter --version
```

布尔参数关闭时使用 `--参数名=false`。Secret 建议通过 `MIHOMO_SECRET` 传入，避免写入命令历史。当前没有 YAML/TOML 配置文件支持。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MIHOMO_URL` | `http://127.0.0.1:9090` | Mihomo External Controller 地址 |
| `MIHOMO_SECRET` | 空 | Mihomo Secret；为空时不发送认证信息 |
| `CONNECTION_INTERVAL` | `1s` | 连接快照间隔，必须为正整数毫秒 |
| `OUTPUT_PROMETHEUS` | `true` | 是否启用 Prometheus 输出 |
| `PROMETHEUS_LISTEN` | `:9091` | Metrics 和健康检查监听地址 |
| `OUTPUT_TELEGRAF` | `false` | 是否启用 Telegraf 推送 |
| `TELEGRAF_URL` | `http://127.0.0.1:8186/mihomo` | Telegraf HTTP 接收地址 |
| `FLUSH_INTERVAL` | `10s` | Telegraf 推送周期 |
| `ENABLE_CLIENT_PROXY` | `false` | 是否启用 Client × Proxy 维度 |
| `HTTP_TIMEOUT` | `5s` | 建连、TLS、响应头与 Telegraf 请求超时；不会限时关闭持续读取的 stream |
| `RECONNECT_INTERVAL` | `2s` | Mihomo 数据流断开后的重连间隔 |
| `LOG_LEVEL` | `info` | `debug`、`info`、`warn` 或 `error` |

至少需要启用 Prometheus 或 Telegraf 中的一种输出。

## 数据差异说明

全局流量来自 `/traffic`，Client 和 Proxy 流量来自 `/connections` 的定期快照。生命周期短于两个快照间隔的连接可能无法被捕获，因此各 Client、Proxy 的流量之和可能略小于全局流量。

Exporter 第一次看到一个连接时只会建立基线，不会把该连接之前已经产生的流量计入本次运行。Exporter 重启后，Prometheus Counter 也会从零开始。

## 常见问题

### Metrics 中没有 Client 或 Proxy 数据

新连接第一次出现时只建立 baseline。等待连接继续产生流量，并确认 `/connections` 数据流处于连接状态：

```promql
mihomo_exporter_stream_connected{stream="connections"}
```

### 无法连接 Mihomo

检查 `MIHOMO_URL`、`MIHOMO_SECRET`、Mihomo 的 `external-controller` 配置，以及容器间使用的主机名和网络。

### Client 流量与全局流量不一致

这是定期连接快照的已知限制。可以降低 `CONNECTION_INTERVAL`，但更频繁的快照会增加 Mihomo 和 exporter 的处理开销。

## 更多文档

- [架构设计](docs/design/architecture.md)
- [systemd 服务配置](docs/deployment/systemd.md)
- [开发、测试与发布](docs/development.md)
- [版本变更](CHANGELOG.md)
- [MIT License](LICENSE)
