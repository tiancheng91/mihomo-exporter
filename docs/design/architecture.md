# 架构设计

本文描述 `mihomo-exporter` 当前实现的运行模型和关键设计决策。它不是待实现需求清单；用户使用说明位于根目录 [README](../../README.md)。

## 目标与边界

Exporter 持续消费 Mihomo External Controller 的两个 HTTP JSON stream：

- `/traffic`：提供全局实时速率和累计流量。
- `/connections?interval=<milliseconds>`：提供当前全部活跃连接的周期快照。

数据被聚合为 Global、Client（`metadata.sourceIP`）、Proxy Node，以及默认关闭的 Client × Proxy 四类维度，然后分别提供给 Prometheus 和 Telegraf。

Exporter 不持久化数据。所有 Counter 都以进程生命周期为边界，长期存储和查询由 Prometheus、InfluxDB 等外部系统负责。

## 数据流

```text
                      ┌──────────────────────────┐
Mihomo /traffic ─────>│ traffic baseline/delta   │──> Global counters/rates
                      └─────────────┬────────────┘
                                    │
                                    ├──> Prometheus cumulative snapshot
                                    └──> Telegraf pending delta

                      ┌──────────────────────────┐
Mihomo /connections ─>│ connection state/delta   │──> active connection gauges
                      └─────────────┬────────────┘
                                    │
                                    ├──> Client/Proxy cumulative counters
                                    ├──> Telegraf pending delta
                                    └──> tracking accuracy metrics
```

采集、聚合、Prometheus scrape 和 Telegraf push 并发运行。共享状态由 `sync.RWMutex` 保护；网络请求不会在持锁状态下执行。

## Traffic 增量

`/traffic` 每条消息包含：

```json
{
  "up": 35833,
  "down": 183,
  "upTotal": 3406438896,
  "downTotal": 174505116558
}
```

`up` 和 `down` 直接作为当前速率 Gauge。`upTotal` 和 `downTotal` 使用相邻消息计算增量，并累加到 exporter 生命周期 Counter。

第一次收到消息时只建立 baseline。当前 total 小于上一次 total 时，视为 Mihomo 重启或 counter reset：本次对应方向不产生 delta，当前值成为新 baseline。因此不会产生负数或无符号整数下溢。

## Connection 增量

每个活跃连接保存以下状态：

```text
connection ID -> source IP, proxy node, upload, download, last seen
```

对连续两个 snapshot 中的同一连接分别计算：

```text
delta upload   = current upload   - previous upload
delta download = current download - previous download
```

处理规则：

- 第一次看到连接时只建立 baseline。
- 当前计数大于或等于上次计数时，累加正向 delta。
- 计数回退时忽略对应方向的 delta，并使用当前值重建 baseline。
- 连接未出现在下一份完整 snapshot 时，立即删除其状态。
- 同一连接重新出现时按新连接处理，不计入消失期间的历史流量。

连接 delta 同时更新 Client、Proxy 和可选的 Client × Proxy 聚合。连接 ID、目标地址、端口、规则和进程等高基数字段不会成为 Prometheus label。

## Proxy Node 解析

Proxy Node 解析集中在独立的 `ResolveProxyNode` 函数中。当前策略使用 `chains[0]`；`chains` 为空或首项为空时返回 `DIRECT`。

集中解析可以在 Mihomo 数据结构或节点选择语义变化时替换策略，而不影响聚合器其他部分。

## Active Connections

活跃连接是 Gauge，而不是 Counter。每次收到完整 `/connections` snapshot 后都会重新计算：

- Global：有效连接 ID 数量。
- Client：同一来源 IP 的连接数。
- Proxy：同一解析节点的连接数。
- Client × Proxy：同一组合的连接数，仅在启用时计算。

历史维度仍保留累计 Counter，但其 Active Gauge 会回到零。

## 双输出状态模型

每个流量 delta 同时写入两类状态：

- `cumulative`：进程生命周期累计值，供 Prometheus scrape 使用。
- `pending`：上次成功推送后新增的值，供 Telegraf 使用。

Prometheus scrape 只读取一致性快照，不修改 cumulative 或 pending 状态。

### Telegraf 提交协议

每次 flush 遵循以下过程：

1. 在读锁下复制 pending 和最新 Gauge。
2. 释放锁并执行 HTTP POST。
3. 失败时不修改 pending。
4. 成功时从当前 pending 中逐项扣除步骤 1 的快照。

使用“扣除已发送快照”而不是“成功后清空”，保证 POST 期间新到达的 delta 不会被误删。减法使用饱和语义作为额外保护。

Telegraf 的 `upload_bytes` 和 `download_bytes` 表示当前 flush 周期新增流量；Prometheus 的 `*_bytes_total` 表示进程生命周期累计流量。

## 误差检测

Connection snapshot 无法保证看到两个 snapshot 之间创建并关闭的短连接。Exporter 使用 `/traffic` 的全局 delta 作为参考，并统计同一 traffic 周期内通过 connection delta 捕获到的字节。

由此产生：

- `mihomo_exporter_connection_tracking_ratio`
- `mihomo_exporter_tracked_upload_bytes_total`
- `mihomo_exporter_tracked_download_bytes_total`
- `mihomo_exporter_missed_upload_bytes_total`
- `mihomo_exporter_missed_download_bytes_total`

分母为零且 tracked 同样为零时，tracking ratio 记为 1。两个 stream 独立到达，因此单个周期可能存在时间边界偏移；该指标更适合观察一段时间内的趋势，而不是账务级精确对账。

## Prometheus 指标

全局指标：

```text
mihomo_upload_bytes_total
mihomo_download_bytes_total
mihomo_upload_bytes_per_second
mihomo_download_bytes_per_second
mihomo_active_connections
```

Client 指标使用唯一动态 label `client`：

```text
mihomo_client_upload_bytes_total
mihomo_client_download_bytes_total
mihomo_client_active_connections
```

Proxy 指标使用唯一动态 label `proxy`：

```text
mihomo_proxy_upload_bytes_total
mihomo_proxy_download_bytes_total
mihomo_proxy_active_connections
```

`ENABLE_CLIENT_PROXY=true` 时额外暴露：

```text
mihomo_client_proxy_upload_bytes_total
mihomo_client_proxy_download_bytes_total
mihomo_client_proxy_active_connections
```

Exporter 自监控指标还包括 stream 连接状态、Telegraf 推送失败次数、最近成功时间和包含 `version`、`commit` 的构建信息。

## Telegraf Line Protocol

每次 flush 发送以下 measurement：

```text
mihomo_traffic
mihomo_client,client=...
mihomo_proxy,proxy=...
mihomo_client_proxy,client=...,proxy=...
```

最后一项默认关闭。动态 tag 中的反斜杠、空格、逗号和等号会按 Influx Line Protocol 规则转义。字段使用整数格式。

## Stream 和生命周期

命令入口将解析后的分组配置传入 `internal/app.Run`。应用先绑定 Prometheus 监听端口，再启动采集任务，绑定失败会直接返回错误。HTTP 服务错误也会触发取消；正常终止会取消请求并等待所有后台任务结束。HTTP server 在优雅关闭超时后强制关闭连接。

两个 stream 使用独立 goroutine。EOF、连接重置、Mihomo 重启、非 2xx HTTP 响应或临时网络错误都会使连接状态变为断开，并在 `RECONNECT_INTERVAL` 后重试，避免 tight loop。

HTTP stream 使用 `json.Decoder` 直接解析连续 JSON 对象，不受 `bufio.Scanner` 默认 token 大小限制。请求通过父级 `context.Context` 管理。

收到 SIGINT 或 SIGTERM 后：

1. 取消根 context。
2. 中断 stream 和正在进行的 Telegraf 请求。
3. 优雅关闭 Prometheus HTTP server。
4. 在超时后退出。

## Cardinality 策略

默认仅允许 `client` 和 `proxy` 成为动态 Prometheus label。Client × Proxy 会形成乘积级时间序列，因此必须显式启用。

以下字段不会输出为 label：

```text
connection_id, host, destinationIP, destinationPort,
sourcePort, rule, process
```

新增 label 前应先估算长期运行时的唯一值数量和 Prometheus 存储成本。

## 已知取舍

- Client 和 Proxy 统计可能遗漏极短生命周期连接。
- 进程不持久化 Counter，重启后从零开始。
- 已出现过的 Client 和 Proxy 会在当前进程的累计 map 中保留，以维持 Prometheus Counter 单调性和输出零值 Active Gauge。
- tracking ratio 受两个独立 stream 的到达顺序影响，只用于可观测性评估。
- Telegraf pending 只保存在内存中；进程在推送成功前异常退出时，尚未发送的数据无法恢复。
