# Redis 安装与迁移实测（2026-09-18）

结论：当前不能判定跨平台安装到使用全链路通行。线上索引只发布 Windows amd64；含空格的数据目录会导致真实 Redis 启动失败。

## 下载 URL

- 默认索引 `https://github.com/lemonade-lab/alemonx/releases/latest/download/redis-runtime-index.json`：GET 成功，重定向至 v0.2.56 的索引。
- 索引唯一资产 `https://github.com/lemonade-lab/alemonx/releases/download/v0.2.56/redis-runtime-v5.0.14.1-windows-amd64.zip`：完整下载成功。
- 实际大小 12,617,669 字节，与索引一致。
- 实际 SHA256 `018ea18a35876383cbb5f4cd0258adfc87747cf9d619bce1cf73a2e36f720ccf`，与索引一致。
- ZIP 包含 redis-server.exe。当前 macOS 主机不能验证该 Windows 可执行文件的启动及运行兼容性。
- 索引没有 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64；这些平台会在资产选择阶段失败。CI 发布步骤也只生成 Windows 资产。

URL 可用性是本次网络实测结果，不保证后续发布或其他网络环境始终可用。

## 真 Redis 集成测试

测试主机 Darwin arm64。在独立临时目录下载官方 Redis 7.2.7 源码并编译 redis-server / redis-cli，仅作为测试进程，未安装系统服务。下载来源：`https://download.redis.io/releases/redis-7.2.7.tar.gz`。

新增 `internal/redis/runtime_integration_test.go`，通过 `ALX_TEST_REDIS_BIN` 显式启用。测试将真实可执行文件打包，使用本地 HTTP 索引调用项目原有下载器，覆盖大小/SHA256 验证、解压、安装、启动、代理、迁移及持久化。

- 无客户端连接的自动安装：调用 Start 后，后台下载、校验、解压、启动和自动切换成功，对外代理可以 PING。
- 普通目录：MiniRedis 的 string、hash、list、set、zset、stream、多数据库内容迁移后读取正确，TTL 仍存在；对外端口保持不变；迁移后写入在重启真实 Redis 后保留。
- 存在长连接时：拒绝迁移，断开连接后显式调用激活可以成功。代码没有在断开后自动重试的调度，不能把 `waiting-for-idle` 理解为一定会自行完成切换。
- 含空格目录：失败。Redis 输出 `FATAL CONFIG FILE ERROR`、`dir …/Application Support/redis-data`、`wrong number of arguments`。原因是 private_runtime.go 写入 dir/logfile 配置时未引用路径。
- 常规 internal/redis 竞态测试通过；禁用 CGO 的 Redis 管理层及 Web Redis 相关回归通过；Redis 包 Windows amd64 测试程序交叉编译通过。交叉编译不等于 Windows 运行测试。

复现命令：

```sh
ALX_TEST_REDIS_BIN=/absolute/path/to/redis-server go test -race ./internal/redis -run TestRealRuntimeInstallAndMigration -count=1 -v
```

redis-cli 需要与 redis-server 位于同一目录。未设置环境变量时，该实机测试跳过；设置后含空格用例当前应失败，保留作缺陷复现。

## 后续修复顺序

1. 为 Redis 配置文件路径正确转义和加引号，修复含空格目录启动。
2. 补齐目标平台运行时发布资产，并对最终发布索引逐包验证 URL、大小、SHA256 与启动兼容性。
3. 补齐等待迁移的自动重试生命周期；验证活跃连接判定与切换期间新连接的一致性。

本轮新增测试和测试报告，未修改生产迁移代码或发布线上资产。
