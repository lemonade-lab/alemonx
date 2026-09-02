# 应用私有 Redis 运行时

ALemonX 的 Redis 默认是应用私有、持久化且自动启动的服务。它不安装或接管操作系统 Redis 服务。

## 启动状态机

1. 读取 `alx-redis.json` 和实例记录 `alx-redis-owner.json`。
2. 若记录的私有运行时仍在运行且 PID、启动令牌、二进制路径均匹配，复用它。
3. 若已安装私有运行时但未运行，使用应用私有配置和数据目录启动它。
4. 若端口存在其他 Redis，报告 `external` 并只复用连接；绝不发送关闭、清空或迁移命令。
5. 没有可用 Redis 时立即启动 miniredis；同时异步准备私有运行时。下载或准备失败不影响 miniredis。
6. 私有运行时通过临时端口自检成功后，冻结由 ALemonX 代理的写请求、保存最终快照、导入并校验数据，然后将代理后端切换到私有服务。

首次激活私有运行时时，`privateInitialized` 会在快照成功导入后持久化。后续启动只恢复 Redis 自己的 AOF/RDB，绝不反复覆盖为过时的 miniredis 回退快照。

服务地址必须由 ALemonX 的本地代理持有。客户端若直接连接后端端口，切换时长连接不能保证无感。

## 文件布局

所有文件位于 ALemonX 用户配置目录的 `redis/` 下：

```text
redis/
  runtime/<version>/<os>-<arch>/redis-server[.exe]
  data/                 # appendonly.aof、dump.rdb
  config/redis.conf
  alx-redis-owner.json  # PID、实例 ID、二进制哈希、启动令牌
  alx-redis-data.json   # miniredis 回退快照
```

应用退出、`alx redis stop` 与 `alx redis restart` 共用同一个受控关闭流程：仅当实例记录匹配时，发送 `SHUTDOWN SAVE`、等待退出，最后才终止该 PID。外部 Redis 永远不在控制范围内。

## 运行时发行协议

默认来源是本仓库的正式 Release。每次发行需提供：

```text
redis-runtime-index.json
redis-runtime-<version>-linux-amd64.tar.gz
redis-runtime-<version>-linux-arm64.tar.gz
redis-runtime-<version>-darwin-amd64.tar.gz
redis-runtime-<version>-darwin-arm64.tar.gz
redis-runtime-<version>-windows-amd64.zip
```

索引必须列出每个资产的 URL、大小、SHA-256、签名、Redis/兼容实现版本、平台和架构。客户端只接受经内置公钥验证的索引，下载时限制大小、校验 SHA-256，并使用防路径穿越的解压逻辑写入 staging 目录；验证可执行文件后以原子 rename 激活。

Linux 需分别构建 glibc 与 musl 变体或在索引中标识 ABI；macOS 运行时需要对应架构的已签名、公证二进制；Windows 发行物必须由 ALemonX 构建/审核并作为前台子进程运行，不能安装为系统服务。

## 命令行

```text
alx redis status
alx redis start
alx redis stop
alx redis restart
```

`status` 显示 `private`、`fallback`、`external` 或 `stopped`，并返回运行时版本、PID、数据目录和最近迁移结果。`start` 默认启动持久化私有运行时；未准备完成时启动 miniredis 并触发后台准备。
