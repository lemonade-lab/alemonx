// Usage: node scripts/audit-api-tests.mjs <Go coverage profile>
// This reports registered route boundaries and handler statement coverage,
// not end-to-end success coverage for every action under a prefix route.
import { readFileSync, readdirSync, writeFileSync, mkdirSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const profile = process.argv[2]
if (!profile) throw new Error('Pass the coverage profile produced by go test.')
const source = readFileSync(resolve(root, 'internal/web/server.go'), 'utf8')
const allSource = readdirSync(resolve(root, 'internal/web')).filter(name => name.endsWith('.go') && !name.endsWith('_test.go')).map(name => readFileSync(resolve(root, 'internal/web', name), 'utf8')).join('\n')
const broker = allSource.match(/\bpluginDownloadBrokerPath\s*=\s*"([^"]+)"/)?.[1]
if (!broker) throw new Error('Could not resolve the download-broker route.')
const functions = new Map()
for (const line of execFileSync('go', ['tool', 'cover', `-func=${resolve(profile)}`], { cwd: root, encoding: 'utf8' }).split('\n')) {
  const parts = line.trim().split(/\s+/)
  if (parts.length === 3 && parts[0].includes('/internal/web/')) functions.set(parts[1], parts[2])
}
const routes = []
for (const match of source.matchAll(/mux\.HandleFunc\(("[^"]+"|pluginDownloadBrokerPath),\s*(s\.(\w+)|func\()/g)) {
  const path = match[1].startsWith('"') ? JSON.parse(match[1]) : broker
  if (!path?.startsWith('/api/v1/')) continue
  const handler = match[3] ?? '(inline legacy endpoint)'
  routes.push({ path, handler, handlerStatementCoverage: functions.get(handler) ?? null, prefix: path.endsWith('/') })
}
routes.sort((a, b) => a.path.localeCompare(b.path))
const protectedCount = routes.filter(route => !['/api/v1/auth/status', '/api/v1/auth/setup', '/api/v1/auth/login', '/api/v1/auth/logout', broker].includes(route.path)).length
const date = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(new Date())
const summary = `# API 测试审计\n\n生成日期：${date}。运行命令：\`make test-api\`。\n\n- 当前注册 API 路由：${routes.length} 条（含动态前缀路由，不含静态页面）。\n- 认证边界：${protectedCount} 条受保护路由 × GET/POST/PUT/PATCH/DELETE/HEAD，共 ${protectedCount * 6} 个请求；公开认证与下载代理由专项测试验证。\n- 非法方法边界：全部 ${routes.length} 条路由发送 TRACE，验证拒绝并返回 JSON。\n- 成功路径：运行 internal/web 全量测试，并补充背包列表、版本与 Git 提交接口夹具。\n- 下表是处理函数的语句覆盖率，不是业务成功率；动态路由下的所有子动作、第三方服务、真实安装/发布和操作系统权限流程不能据此宣称完成端到端验证。\n- /healthz 由现有健康检查测试验证；未知 API 路径必须返回 JSON 404。\n\n| 路由 | 处理函数 | 语句覆盖率 |\n| --- | --- | --- |\n` + routes.map(route => `| \`${route.path}\` | \`${route.handler}\` | ${route.handlerStatementCoverage ?? '内联处理'} |`).join('\n') + '\n'
mkdirSync(resolve(root, 'docs'), { recursive: true })
writeFileSync(resolve(root, 'docs/API_TEST_AUDIT.md'), summary)
writeFileSync(resolve(root, 'docs/api-test-audit.json'), JSON.stringify({ date, registeredRoutes: routes.length, authenticationRequests: protectedCount * 6, unsupportedMethodRequests: routes.length, routes }, null, 2) + '\n')
console.log(`Audited ${routes.length} registered routes; reports written to docs/API_TEST_AUDIT.md and docs/api-test-audit.json.`)
