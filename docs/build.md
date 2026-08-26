# 机器人构建规范

本文说明 ALemonX 在机器人项目中如何选择构建入口。构建规范用于 Git 发布、NPM 发布以及其它需要生成发布产物的流程。

## 构建入口优先级

ALemonX 按以下顺序选择构建入口：

```text
package.json 的 alemonjs.build
        ↓
package.json 的 scripts.bundle
        ↓
lvy build
```

一旦找到更高优先级的入口，就不会继续尝试后面的入口。

## 推荐写法

推荐在 `package.json` 中声明一个 `bundle` 脚本，并让 `alemonjs.build` 指向它：

```json
{
  "scripts": {
    "bundle": "yarn --cwd frontend build && lvy build"
  },
  "alemonjs": {
    "build": "bundle"
  }
}
```

`alemonjs.build` 的值是当前 `package.json` 中已有的脚本名。ALemonX 会使用项目自己的包管理器执行该脚本：

```text
<package-manager> run bundle
```

其中包管理器根据 `packageManager` 字段或 lockfile 选择，可以是 npm、Yarn 或 pnpm。

## 仅使用 bundle

如果不需要额外声明 `alemonjs.build`，可以只提供：

```json
{
  "scripts": {
    "bundle": "lvy build"
  }
}
```

ALemonX 会执行：

```bash
<package-manager> run bundle
```

## 默认回退

没有 `alemonjs.build`，也没有 `scripts.bundle` 时，ALemonX 执行：

```bash
lvy build
```

这适用于标准 AlemonJS 项目。项目应确保 `lvy` 已经可以被当前运行环境找到，或者使用前面的 `bundle` 入口通过包管理器脚本调用它。

## 嵌套前端项目

如果 `bundle` 脚本构建独立的前端目录，例如：

```json
{
  "scripts": {
    "bundle": "yarn --cwd frontend build && lvy build"
  }
}
```

ALemonX 会识别 `--cwd frontend` 这类嵌套项目，并在执行根项目构建前安装该目录的依赖。这样 `frontend/package.json` 中的 React、Vite 等依赖不会因为根目录不是 Yarn workspace 而遗漏。

嵌套目录必须是项目根目录下的相对路径，并且自身包含 `package.json`。不要使用绝对路径或越过项目根目录的路径。

## 从旧项目迁移

旧项目可能只有：

```json
{
  "scripts": {
    "build": "yarn --cwd frontend build && lvy build"
  }
}
```

建议改为：

```json
{
  "scripts": {
    "bundle": "yarn --cwd frontend build && lvy build"
  },
  "alemonjs": {
    "build": "bundle"
  }
}
```

不要把 `bundle` 写成任意宿主机 Shell 命令；它应当是一个普通的 `package.json` 脚本名。构建流程会在隔离的 Git worktree 中执行，不会把未提交的工作区修改带入发布包。

## 内置 Yarn 的职责

ALemonX 内置 Yarn bundle 的作用是提供稳定的 Yarn 执行环境，避免系统 PATH 或 Yarn 版本差异影响构建。它不负责决定项目的构建步骤，也不会取代项目声明的 npm、Yarn 或 pnpm。

构建步骤由上述构建入口决定，包管理器只负责安装依赖和执行脚本。

## 构建失败排查

在「机器人面板 → 发布 → Git 发布」中，构建失败时应重点查看：

1. 当前选中的源码提交是否包含最新的 `package.json`；
2. 是否声明了 `alemonjs.build` 指向的脚本；
3. `scripts.bundle` 是否存在；
4. 嵌套前端目录是否包含自己的 `package.json` 和 lockfile；
5. 构建日志最后部分的实际错误，而不是 Git worktree 的 `HEAD is now at ...` 输出。

`Preparing worktree` 和 `HEAD is now at ...` 是创建隔离构建目录的正常信息，不代表构建失败。
