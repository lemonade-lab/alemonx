# ALemonX Agent 发布验收报告

## 当前能力

- TaskService 统一任务创建、启动、等待、取消和恢复入口。
- TaskPlan 按 `understand → implement → verify` 串行推进，验证失败保留在当前步骤。
- GoalRun 支持 queued/running/terminal 状态、重启补偿和目标级互斥。
- checkpoint、events、snapshot、report 使用本地原子持久化，旧聊天接口保持兼容。
- 运维中心支持 PM2 错误指纹去重、Incident、Todo、MaintenanceRun、项目策略和指标查询。
- 自动维护采用项目白名单：默认 observe，只有明确授权项目才可进入 auto；高风险决策始终转人工。
- 自动任务完成后进入观察窗口；错误复发会停止自动修复并尝试快照回滚，失败则进入 recovery_required/Todo。
- 运维中心提供实时事件、待办、维护记录、策略、暂停采集和紧急停止入口。
- ServerRuntime 提供幂等 Shutdown；主进程收到 SIGINT/SIGTERM 时会停止后台循环、暂停运行任务并刷新持久化状态。
- OpsPolicy 预算闸门覆盖 Token、PM2 动作和重试；预算超限会转待办，失败达到阈值会自动熔断为 strict。
- PM2 健康信号、进程退出和日志事件统一持久化；支持 Webhook 告警和 Prometheus 指标导出。

## 运维状态机与灰度规则

```text
PM2 日志 → fingerprint 去重 → Incident triaged → AI 决策
  → auto_fix → TaskPlan/Reviewer → observing → resolved
  → 失败、复发或高风险 → todo / recovery_required
```

- 项目策略保存在 `incidents/policy-*.json`，使用 `autoAllowed` 作为白名单闸门。
- `OpsMonitor` 将事件指纹持久化到 `seen-events.json`，重启后不重复唤醒 AI。
- `FailureCircuitBreak` 达到阈值后自动切换 strict 并撤销白名单。
- 启动时对账未完成 MaintenanceRun；缺失任务或快照冲突不会自动重放写操作。
- `GET /api/v1/ops/metrics/prometheus` 输出 incident、维护成功、回滚和 MTTR 指标。
- Webhook 通过 `ALX_OPS_WEBHOOK_URL` 配置，通知失败异步重试，不阻塞 Agent 任务。

## 线上启用与紧急停止

1. 生产建议设置 `ALX_DEPLOYMENT=production`、`ALX_OPS_STORAGE=sqlite` 并启用本地身份认证；两者未配置时服务都不会拒绝启动，只会打印提示（canary 准入报告会将其标记为未就绪）。
2. 先将项目保持 `observe`，确认 PM2 日志、事件聚合和待办链路正常。
3. 管理员在“机器人目录 → 运行 → AI 运维”填写理由，切换到 `canary`。初始只允许 restart/reload，禁止代码修改。
4. 稳定观察 24 小时的 MTTR、回滚率、误修率、告警送达率与租约异常后，管理员再次确认才可开启小范围代码修复；不会自动升级为 `auto`。
5. 发生异常时使用“紧急停止”，或调用 `POST /api/v1/ops/monitor/emergency-stop`；恢复使用 `POST /api/v1/ops/monitor/resume`。
- SSE 支持 `Last-Event-ID` 续传，写操作重新进入 ask 权限确认。

## 生产自动维护门禁

- 新任务 API 默认 `plan_pending`；旧聊天接口仅为兼容而隐式批准。自动维护只能在 canary/auto、低风险、未编辑计划和项目白名单同时满足时批准。
- `verificationCommand` 是观察阶段的硬门槛：实现完成后必须通过受控验证命令、`agent_verify` 与 Reviewer；任一失败会回滚快照、生成 Todo，并保留验证输出和报告。
- 所有 PM2 写操作（start/stop/restart/reload/delete）都经过 fencing 租约、紧急停止和预算检查；只读 status/logs 不会申请写租约。
- 文件批采集与流式回退分别保存 file/stream cursor。批采集失败才启用唯一流式消费者，探测恢复后自动切回批采集，避免两个来源重复消费。
- 告警只通过持久化队列投递；独立 AlertDeliveryWorker 持有投递租约，崩溃中的 sending 项会在一分钟后回收，重试采用指数退避。指标按 5 分钟时间桶保存 90 天，JSON API 与 Prometheus 使用同一快照。

## 入口兼容矩阵

| 入口 | 执行路径 | 兼容性 |
| --- | --- | --- |
| `/api/v1/agent/tasks` | TaskService → TaskManager | 新任务需批准计划 |
| `/api/v1/agent/chat` | TaskService → Wait | 保持 `{answer, sessionId}` |
| `/api/v1/agent/chat?stream=1` | TaskService → SSE | 保持旧事件格式 |
| Goal 手动运行 | GoalRun queued → TaskService | 强制 ask |
| Goal 定时运行 | Scheduler → GoalRun queued → TaskService | 强制 ask |

## 验收命令

```bash
make test-agent
make test-all
make lint
make build-frontend
git diff --check
```

测试环境可使用 `ALX_TEST_CACHE_DIR`、`GOCACHE` 和 `TEST_LISTEN_ADDR` 注入临时目录/监听地址，避免写入用户缓存或绑定 IPv6 回环地址。

## 已知限制

- 任务仅在当前 ALemonX 进程存活期间运行；重启后会暂停并需要用户显式恢复。
- workspace 是默认隔离方式，worktree 仍为可选模式。
- 定时任务不会自动获得高风险写权限。
- 真实供应商不参与自动化测试，集成测试应使用 fake provider。
- PM2 健康信号当前基于受控 `jlist/logs` 轮询，不是 PM2 daemon 原生事件流。
- 首期 Webhook 是通用 JSON 适配，不包含具体厂商的值班升级策略。
- PM2 批采集已通过 `robot.PM2LogBatchSource` 使用真实文件 device/inode/offset，并在批读取失败时才启动流式回退；无法取得 PM2 日志路径时仍会退回 PM2 流式接口。
- SQLite v2 将事件、待办、维护、策略、告警、审计、游标、预算和租约的核心检索字段持久化为列，同时保留 payload 以兼容旧 checkpoint/记录；更深层的嵌套字段仍按 payload 演进。
- AlertDeliveryWorker 是当前进程内单消费者；跨区域或独立扩缩容仍需后续将其拆为独立部署单元。
- 指标按 5 分钟桶聚合并保留 90 天；高基数 fingerprint 仍应通过项目策略和日志限流控制规模。
- 多实例租约具备事务竞争保护和 fencing token；旧实例在 token 变化、租约过期或紧急停止后不能继续执行 PM2 写操作。生产仍建议先采用单主实例灰度。

## 下一阶段生产可靠性增量

- OpsStore 现在通过 `OpsRepository` 暴露持久化边界；生产可使用基于 `modernc.org/sqlite` 的纯 Go SQLite 适配器，开发/测试可回退 JSON，迁移前保留原始备份；不依赖系统 `sqlite3` 命令或 CGO。
- 监控实例使用持久化租约，避免多实例同时消费日志和触发自动修复。
- 策略支持 `canary` 灰度模式，自动维护前可执行 dry-run 或单次人工批准。
- 运维写操作支持 viewer/operator/approver/admin 角色头，并将拒绝、批准、回滚、策略修改和紧急停止写入 `audit.jsonl`。
- 告警会持久化到 `alert-*.json`，支持确认、静默和恢复后的审计查询。
- 维护运行保存安全、验证、目标完成和无关 diff 评分，供后续评测使用。
- SQLite 适配器提供 schema 元数据、索引和显式关闭；任务支持 `Idempotency-Key`，避免请求重试创建重复任务；PM2 项目/进程游标持久化，监控可从最近采集位置继续。
- OpsMonitor、GoalScheduler 和自动写任务使用统一 LeaseManager；租约包含创建、续期和过期时间，续期失败会停止后续执行。告警重试次数和退避间隔可按 severity 策略配置。
- SQLite 核心实体已事务性双写到明确业务表与兼容 records 表；项目写任务、GoalScheduler、OpsMonitor 均纳入租约边界。
- 本轮将 SQLite 业务实体切换为主读取路径，并为 Incident、Todo、Maintenance、Policy、Alert、LogCursor 增加稳定倒序查询；兼容 records 表仅作为迁移/回退来源。
- SQLite 租约已改为 `leases` 表内的条件事务更新，支持过期接管、续期失败检测和 owner 校验，不再生成 sqlite lease sidecar 文件。
- 自动维护按 Incident 检查 queued/running/fixing/verifying/observing 记录，重复分析不会再次启动同一 Incident 的写任务。
- 告警投递失败会保存 `delivery_failed`、重试次数、下次尝试时间和错误原因；OpsMonitor 轮询时会恢复到达重试时间的记录。
- SQLite 和 JSON 均提供 `AlertQueue`，失败投递可领取、确认或重新排队；SQLite 队列使用 `alert_deliveries` 表事务更新。
- 租约现在暴露 `GetLease/ListLeases` 和 fencing token；项目写任务会在续期时校验 token 变化并主动暂停。
- 新增 `LogBatch/LogBatchSource` 兼容扩展，可由带文件元数据的日志源提供真实路径、device、inode、offset；旧 PM2 流式接口继续作为回退。
- `robot.PM2LogBatchSource` 已从 PM2 jlist 暴露 error log 路径，并处理文件轮转、截断、inode/device 和 offset；OpsMonitor 优先使用批读取，失败时回退旧流式源。
- 自动 restart/reload 已通过 `GuardedPM2Executor` 校验项目 fencing token、紧急停止状态和 PM2 预算。
- 新增 JSON/SQLite `MetricsRepository` 和 `ops_metrics` 表，事件指标通过原子累计并由 JSON/Prometheus 共用 Snapshot。
- 新增 `/api/v1/ops/metrics/query` 查询入口；GuardedPM2Executor 统一保护 restart/reload，并写入审计记录和 PM2 失败结果。

SQLite 启用方式：设置 `ALX_OPS_STORAGE=sqlite`，并可用 `ALX_OPS_SQLITE_PATH` 指定数据库文件。服务启动时会使用纯 Go 迁移器将 JSON 记录导入 SQLite；迁移失败保持 JSON 模式且不启动自动写任务。

Canary 准入报告：管理员可查询 `GET /api/v1/ops/canary-readiness?root=<机器人目录>`，页面位于“机器人目录 → 运行 → AI 运维”。报告要求生产认证、SQLite、白名单、受围栏 PM2 权限、验证契约、告警 Worker/接收端和未触发紧急停止全部就绪；它不会自动启用 canary。

生产启用时建议先使用 `observe`，再选择单项目 `canary`，确认告警送达、租约接管、dry-run 和人工接管流程后才切换 `auto`。

## 回滚说明

- 任务级 snapshot 保留修改前数据，回滚前会校验当前文件 hash。
- checkpoint、events、report 不因回滚删除，便于审计和再次恢复。
- worktree 合并前需要用户确认；冲突时拒绝静默覆盖。

## 本轮验收结果

- `make lint`：通过，Go vet 和前端 ESLint 均无错误/警告。
- `make test-all`：使用临时 `ALX_TEST_CACHE_DIR`、`GOCACHE` 和 `GOTMPDIR`；发布机必须预留至少 1 GiB 测试临时空间，空间不足会在 `make test-space` 明确失败。
- `make build-frontend`：通过。
- `git diff --check`：通过。
- 新增故障夹具覆盖预算超限、持久化去重、Webhook 失败隔离和 Runtime Shutdown 幂等性。
- 普通全仓测试、`go test -race ./internal/agent ./internal/web`、前端 lint/build 均通过。
