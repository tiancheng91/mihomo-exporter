# Changelog

本项目的所有重要变更都会记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.1.1] - 2026-09-21

### Added

- 增加 Linux systemd 服务安装、配置和运维示例。

### Changed

- GitHub Actions 升级到 Node.js 24 运行时对应版本。
- README 按从简单到复杂的顺序调整，并增加单行快速启动示例。

### Fixed

- `/traffic` 和 `/connections` 统一使用 WebSocket；修复普通 HTTP GET 只返回一次 connections snapshot，导致 exporter 持续重连且 `CONNECTION_INTERVAL` 不生效的问题。

## [0.1.0] - 2026-09-21

### Added

- 支持持续采集 Mihomo `/traffic` 和 `/connections` HTTP stream。
- 支持 Global、Client、Proxy 及可选 Client × Proxy 维度聚合。
- 提供 Prometheus metrics、健康检查和 Telegraf Line Protocol 推送。
- 支持连接增量追踪、counter reset、流量误差指标与可靠 pending 重试。
- 提供环境变量配置、自动重连、优雅关闭与周期状态日志。
- 所有运行配置支持 CLI 参数，新增 `--help` 和 `--version`；环境变量优先于 CLI 参数。
- 提供 Docker、Docker Compose、GitHub Actions CI 和多平台 Release 构建。
- 使用 MIT License 发布。

### Changed

- 配置按组件分组，分离默认值、解析、校验和应用生命周期，日志由应用实例注入。
- README 调整为面向普通用户的安装、配置和排障指南，开发与架构文档迁移到 `docs/`。

### Fixed

- 监听端口失败返回非零退出码，关闭时等待后台任务退出。
- 拒绝小于 1 毫秒或非整毫秒的连接快照间隔，避免请求参数被截断。

[Unreleased]: https://github.com/tiancheng91/mihomo-exporter/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/tiancheng91/mihomo-exporter/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/tiancheng91/mihomo-exporter/releases/tag/v0.1.0
