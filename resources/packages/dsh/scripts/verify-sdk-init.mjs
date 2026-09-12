import { mkdtemp, mkdir, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawn } from 'node:child_process'

const home = await mkdtemp(join(tmpdir(), 'alemonx-dsh-sdk-'))
try {
  const bridge = join(process.cwd(), 'node_modules', '@alemonx', 'dsh-approval-bridge')
  const target = join(home, 'node_modules', '@alemonx', 'dsh-approval-bridge')
  await mkdir(join(home, 'node_modules', '@alemonx'), { recursive: true })
  await symlink(bridge, target, 'dir')
  const patch = join(home, 'alemonx.patch.yml')
  await writeFile(patch, `- insert:\n    - id: alemonx-approval-bridge\n      name: '@alemonx/dsh-approval-bridge'\n      config:\n        url: !!js process.env.ALX_DSH_BRIDGE_URL\n        token: !!js process.env.ALX_DSH_BRIDGE_TOKEN\n`, { mode: 0o600 })
  const child = spawn(process.execPath, [join(process.cwd(), 'node_modules', '@deepseek-ai', 'dsh', 'lib', 'bin.js'), '--profile', 'alemonx', '--patch', patch, '--from-default-profile', 'sdk'], {
    env: { ...process.env, DSH_HOME: home, ALX_DSH_BRIDGE_URL: 'http://127.0.0.1:1/approval', ALX_DSH_BRIDGE_TOKEN: 'check-token', DEEPSEEK_API_KEY: 'check-key' },
    stdio: ['pipe', 'pipe', 'pipe']
  })
  let stdout = ''
  let stderr = ''
  child.stdout.on('data', (chunk) => { stdout += chunk })
  child.stderr.on('data', (chunk) => { stderr += chunk })
  child.stdin.end(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { cwd: process.cwd(), provider: 'deepseek-official', model: 'deepseek-flash' } }) + '\n')
  const exitCode = await new Promise((resolve, reject) => {
    child.once('error', reject)
    child.once('close', resolve)
  })
  const response = stdout.trim().split('\n').map((line) => {
    try { return JSON.parse(line) } catch { return null }
  }).find(Boolean)
  if (exitCode !== 0 || response?.result?.serverInfo?.name !== 'deepseek-harness-sdk-runtime') {
    throw new Error(`DSH SDK initialization contract failed: ${stderr || stdout}`)
  }
} finally {
  await rm(home, { recursive: true, force: true })
}
