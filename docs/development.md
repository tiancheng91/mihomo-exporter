# 开发、测试与发布

本文面向项目维护者。普通用户请查看根目录的 [README](../README.md)。

## 环境要求

- Go 1.23 或更高版本
- 可选：Docker 和 Docker Compose

## 代码结构

```text
.
├── cmd/mihomo-exporter/       信号、CLI、退出码和构建版本入口
├── internal/app/              生命周期与后台任务协调
├── internal/config/           默认值、CLI/环境变量解析和校验
├── internal/mihomo/           Mihomo HTTP stream 客户端
├── internal/collector/        增量计算、聚合和并发状态
├── internal/prometheus/       Prometheus collector 与 HTTP 服务
├── internal/telegraf/         Line Protocol 编码与可靠推送
├── docs/design/               架构设计文档
├── .github/workflows/         CI 与 Release 工作流
├── Dockerfile
└── docker-compose.yml
```

## 本地开发

安装依赖并运行：

```bash
go mod download
go run ./cmd/mihomo-exporter
```

执行完整检查：

```bash
make check
```

等价命令为：

```bash
go test ./...
go vet ./...
go build ./cmd/mihomo-exporter
```

修改并发聚合、stream 或推送逻辑时，还应执行竞态检测：

```bash
go test -race ./...
```

## 测试重点

单元测试应持续覆盖以下边界：

- 新连接只建立 baseline，不产生 delta
- 正常 delta 和 counter reset
- 连接从 snapshot 消失后的状态清理
- Client、Proxy 和 Client × Proxy 聚合
- traffic total reset
- Telegraf 推送失败不丢 pending 数据
- 推送期间新增 delta 不被成功回调清除
- Influx tag escaping
- Prometheus scrape 不修改聚合状态

Mihomo stream 协议相关变更应使用 `httptest.Server` 增加集成测试。

## 构建

本地构建会注入版本和 Git commit：

```bash
make build VERSION=v0.1.0
```

构建 Docker 镜像：

```bash
make docker-build
```

`Dockerfile` 使用 multi-stage build 和 distroless 运行镜像，可通过 BuildKit 构建 amd64/arm64 镜像。

## Changelog

所有面向用户的重要变化都应记录在根目录的 [CHANGELOG.md](../CHANGELOG.md)。开发期间写入 `Unreleased`；发布前创建带日期的版本章节，例如：

```markdown
## [0.2.0] - 2026-10-01

### Added

- 新增功能说明。
```

版本号遵循 Semantic Versioning，内容分类采用 Keep a Changelog 的 `Added`、`Changed`、`Fixed`、`Deprecated`、`Removed` 和 `Security`。

## 发布

推送 `v*` tag 会触发 `.github/workflows/release.yml`。工作流会：

1. 执行竞态测试和静态检查。
2. 交叉编译 Linux、macOS、Windows 的 amd64/arm64 二进制。
3. 打包二进制、README、LICENSE 和 CHANGELOG。
4. 生成 `checksums.txt`。
5. 从 Changelog 中提取对应版本内容作为 Release Notes。
6. 创建 GitHub Release 并上传所有构建产物。

发布前确保版本章节已提交，然后执行：

```bash
git tag v0.1.0
git push origin v0.1.0
```

如果 Changelog 缺少对应版本，Release 工作流会失败。工作流需要仓库允许 GitHub Actions 使用 `contents: write` 权限。

## 设计变更

配置使用按组件分组的强类型结构：`Mihomo`、`Prometheus`、`Telegraf`、`HTTP`。使用标准库 `flag`，不引入额外配置框架。新增选项时同步注册 CLI flag、更新校验、README 和测试；环境变量名由 flag 名自动映射，优先级保持环境变量 > CLI > 默认值。

`config.Defaults` 不读取进程环境；`config.Parse` 接收参数、环境查询函数和输出 Writer；`Config.Validate` 可独立调用。`app.Run` 接收 context 和 logger，不修改全局 logger，返回前等待后台任务退出。`main` 只负责组装这些依赖以及退出码：0 为正常退出或帮助/版本，1 为运行失败，2 为配置或参数错误。

修改聚合语义、指标含义、并发模型、输出可靠性或 cardinality 策略时，应同步更新[架构设计](design/architecture.md)、README 和相关测试。新增动态 Prometheus label 前必须评估时间序列数量。
