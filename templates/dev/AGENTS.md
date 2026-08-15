# AGENTS.md

## 项目说明

这是一个 AlemonJS 机器人项目模板，基于 Node.js 与 TypeScript 开发。模板内置最基础的命令响应（`hello`/`help`）、jsxp 图片组件与数据存取示例（`store`），新项目从这里起步。

## 工作原则

- 在开始修改前，先查看 `package.json`、`alemon.config.yaml`、`lvy.config.ts` 与 `src/` 下的已有实现。
- 优先做最小改动，只修改完成当前任务所必需的文件。
- 优先复用现有模块、工具函数和数据结构，不要随意新增依赖。
- 不要无故重构、重命名或移动文件；不要修改与当前任务无关的代码。
- 修改代码时，保持现有项目结构、编码风格和运行方式一致。

## 使用的技术

- alemonjs（跨平台聊天机器人框架）、jsxp（图片渲染）、lvyjs（构建）、React 19、TypeScript；可按需接入 Redis / MySQL。

## 开发与验证

改完代码后根据当前环境运行验证命令，确认没有破坏现有功能：

- 类型检查：`npm install -g @typescript/native-preview` 后运行 `tsgo`
- 代码检查：`npm install -g eslint` 后运行 `eslint src --ext .ts,.tsx,.js --max-warnings=0`
- 本地开发：`npm run dev`；图片预览：`npm run view`；生产构建：`npm run build`

如果由于环境、依赖、权限或上下文限制无法执行，请明确说明原因，不要假装已经完成验证。

## 项目目录说明

- `src/index.ts`：路由入口，用 `Router.create().group().use()` 注册命令与 HTTP 路由
- `src/response/`：指令响应处理器（一个文件一个能力）
- `src/image/component/`：jsxp 图片组件
- `src/assets/`：静态资源（CSS、图片、字体）
- `src/store.ts`：数据存取示例
- `src/expose.ts`：对外暴露的能力
- `app.ts` / `index.js`：lvyjs 开发/生产入口
- `lvy.config.ts` / `jsxp.config.tsx`：构建与图片渲染配置
- `alemon.config.yaml`：用户配置
- `pm2.config.cjs`：PM2 运行配置

## 开发约束（遵循 alemonjs-dev-skill）

- 命令注册用 `group.use('path', importer)`，参数校验写进 `schema`；不要在 handler 里手写命令匹配。
- 消息统一用 `Format.create()` 链式构建；路由上下文用 `useRoute()`，当前事件用 `useEvent()`。
- 图片组件统一用 `Html` 外壳包裹；增删改组件时同步调整 `jsxp.config.tsx` 的路由。
- 用户配置统一写入 `alemon.config.yaml` 并挂在应用自己的命名空间下；不要用环境变量或 Redis 保存一般用户配置。
- 时间逻辑统一使用 `dayjs`，先定义时区与边界，再写业务判断。
- 静态资源用 `@src` 别名 direct import，不要在业务层用 `path.join()` 等方式拼路径读取。
- 严禁使用 `require`；`import()` 仅限路由懒加载场景。
- 不要臆造不存在的命令、文件、配置或脚本；不要假设未经代码、配置或现有逻辑验证的业务规则。
- 涉及气泡/图片展示修改时，完成后把气泡效果发出来进行二次确认。

## 技能读取

遇到框架相关内容设计时，可读取仓库 https://github.com/lemonade-lab/alemonjs-dev-skill 获得开发技能。
