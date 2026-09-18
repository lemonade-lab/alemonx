# Connect 通用 SQL 客户端升级

## 当前交付

- 通用连接模型、独立 Connect 工作区和 `/api/v1/data/connect` 接口，不依赖 `SQLiteBrowser`。
- MySQL/MariaDB、PostgreSQL、SQLite 引擎适配：连接测试、数据库/Schema、表与视图、结构、索引、分页浏览。
- 命名连接新增/编辑/保存/忘记/断开，旧版 SQLite 连接迁移，默认 TLS 验证。
- 多查询页签、执行选中内容、单条 SQL 查询与确认写入、取消请求、查询上下文切换确认。
- 原内置 SQLite 和 Redis 保持独立；新接口仍受超级管理员及系统 SQLite 强制只读保护。

## 原计划阶段状态

| 阶段 | 当前状态 |
| --- | --- |
| 1 通用架构 | 通用契约和引擎适配已落地；后续按新增功能补充能力描述，而非假装所有驱动能力一致 |
| 2 多数据库连接 | 四种引擎接入、连接表单及密码非持久化完成；证书配置/SSH 隧道未实现 |
| 3 对象工作区 | 数据库/Schema/对象导航、搜索、结构及索引读取完成；当前为分组选择器加列表，不是完整可展开树 |
| 4 数据操作 | 浏览和分页、SQL 写入完成；通用表格直接编辑、筛选排序、待提交变更集未实现 |
| 5 SQL 查询 | 多页签、单条/选中执行和取消完成；查询文件保存、会话事务未实现 |
| 6 结构与交换 | 结构/索引查看及手写 DDL 完成；可视化结构编辑、CSV/SQL 导入导出未实现 |
| 7 集成验收 | 当前增量测试通过；完整替代产品验收须待后续能力实现 |

## 依赖与部署边界

- MySQL 驱动 `github.com/go-sql-driver/mysql v1.9.3`：MPL-2.0；PostgreSQL 驱动 `github.com/lib/pq v1.12.3`：MIT。均为纯 Go。MySQL 选用与项目 Go 1.23 基线兼容的版本，不直接追随需要更高 Go 版本的主分支。
- 新增间接依赖 `filippo.io/edwards25519 v1.1.0` 用于驱动认证；未引入前端编辑器或 UI 大型依赖。数据窗口仍为懒加载，构建时数据模块约 15 KiB gzip（后续构建可能变化）。
- 保留第三方许可证声明；没有复制 Navicat 的代码或素材。
- 远程连接从 AlemonX 服务器发起，Docker 中 localhost 指向容器。不得将无认证的工作台暴露公网，数据库账户使用最小权限。
- 本地 Go 1.25.6 的 `govulncheck ./internal/datamanager` 报告 10 项可达标准库漏洞，并非安全扫描通过。发布前需选择已修复的 Go 工具链重新扫描；本轮未修改全局 Go 安装或项目工具链基线。

## 验证与复现

单元/接口：`CGO_ENABLED=0 go test ./internal/datamanager ./internal/web -run 'TestConnect|TestData|TestDefault'`。
前端：`yarn lint`、`yarn build`、`yarn test:sse --workers=2`。
跨平台：`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c ./internal/web`。

远程集成测试只在显式设置 `ALX_CONNECT_TEST_PG_PORT`、`ALX_CONNECT_TEST_MARIA_PORT`、`ALX_CONNECT_TEST_MYSQL_PORT` 时运行，对应本机回环测试容器。测试会创建并删除固定名测试表及 PostgreSQL 测试 Schema，**不可指向业务实例**。本轮使用 PostgreSQL 17、MariaDB 11.4、MySQL 8.4 临时容器，覆盖读写、元数据、精度、只读事务、错误脱敏与拒绝不可信 TLS。
