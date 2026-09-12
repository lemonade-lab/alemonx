import { mkdtemp, mkdir, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'

// Exercise a real SDK process against a local model: initialization alone
// cannot detect request-extension failures or initial idle notifications.
const home = await mkdtemp(join(tmpdir(), 'alx-dsh-prompt-'))
let calls = 0
let restoredHistory = false
const server = createServer((req, res) => {
  let body = ''
  req.on('data', chunk => { body += chunk })
  req.on('end', () => {
    calls++
    if (calls > 1) {
      const request = JSON.parse(body)
      restoredHistory ||= request.messages?.some(message => message.role === 'assistant' && JSON.stringify(message.content).includes('ALX prompt verified'))
    }
    res.writeHead(200, { 'content-type': 'text/event-stream' })
    res.end('data: ' + JSON.stringify({
      id: 'mock', object: 'chat.completion.chunk',
      choices: [{ index: 0, delta: { role: 'assistant', content: 'ALX prompt verified' }, finish_reason: null }]
    }) + '\n\ndata: ' + JSON.stringify({
      id: 'mock', object: 'chat.completion.chunk',
      choices: [{ index: 0, delta: {}, finish_reason: 'stop' }]
    }) + '\n\ndata: [DONE]\n\n')
  })
})
server.listen(0, '127.0.0.1')
await once(server, 'listening')
let child
try {
  await mkdir(join(home, 'node_modules', '@alemonx'), { recursive: true })
  await symlink(join(process.cwd(), 'plugins', 'approval-bridge'), join(home, 'node_modules', '@alemonx', 'dsh-approval-bridge'), 'dir')
  const patch = join(home, 'alemonx.patch.yml')
  await writeFile(patch, `- id: plugin-package-inventory-deepseek
  disabled: true
- id: session-log-deepseek
  disabled: true
- insert:
    - id: alemonx-approval-bridge
      name: '@alemonx/dsh-approval-bridge'
      config:
        url: http://127.0.0.1:1/approval
        token: mock-token
`)
  for (let boot = 0; boot < 2; boot++) {
  child = spawn(process.execPath, [join(process.cwd(), 'node_modules/@deepseek-ai/dsh/lib/bin.js'), '--profile', 'alemonx', '--patch', patch, ...(boot === 0 ? ['--from-default-profile', 'sdk'] : [])], {
    env: { PATH: process.env.PATH, HOME: home, DSH_HOME: home, DEEPSEEK_API_KEY: 'mock-key', DEEPSEEK_BASE_URL: 'http://127.0.0.1:' + server.address().port },
    stdio: ['pipe', 'pipe', 'pipe']
  })
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('SDK prompt timed out')), 45000)
    let buffer = '', answer = false
    const done = error => { clearTimeout(timer); error ? reject(error) : resolve() }
    child.once('error', done)
    child.once('exit', code => { if (!answer) done(new Error('SDK exited: ' + code)) })
    child.stderr.resume()
    child.stdout.on('data', chunk => {
      buffer += chunk
      const lines = buffer.split('\n'); buffer = lines.pop()
      for (const line of lines) {
        let frame
        try { frame = JSON.parse(line) } catch { continue }
        if (frame.error) { done(new Error('SDK RPC rejected: ' + frame.error.code)); continue }
        const prompt = () => child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'session/prompt', params: { sessionId: 'alx-mock-prompt', contentBlocks: [{ type: 'text', text: 'Reply briefly without tools.' }] } }) + '\n')
        if (frame.id === 1) {
          if (boot === 0) prompt()
          else child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 4, method: 'alemonx/session/history', params: { sessionId: 'alx-mock-prompt' } }) + '\n')
        }
        if (frame.id === 4) {
          const messages = frame.result.messages
          if (!messages.some(m => m.role === 'user') || !messages.some(m => m.role === 'assistant' && m.content.includes('ALX prompt verified'))) {
            done(new Error('History endpoint did not restore both sides of conversation'))
          } else prompt()
        }
        const event = frame.params?.event
        if (event?.type === 'assistant/message') answer ||= event.data.message.content.some(b => b.type === 'text' && b.text.includes('ALX prompt verified'))
        if (event?.type === 'turn/end') done(answer && calls > 0 && event.data.reason.kind === 'completed' ? undefined : new Error('SDK turn failed: ' + event.data.reason.kind))
      }
    })
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { cwd: home, provider: 'deepseek-official', model: 'deepseek-chat' } }) + '\n')
  })
  const exited = once(child, 'exit')
  child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 3, method: 'shutdown', params: {} }) + '\n')
  await exited
  }
  if (!restoredHistory) throw new Error('Restart lost the previous assistant history')
  console.log('Real SDK first prompt and restart → resume same session with history → reply passed.')
} finally {
  if (child && child.exitCode === null) { const exited = once(child, 'exit'); child.kill(); await exited }
  server.closeAllConnections()
  server.close()
  await rm(home, { recursive: true, force: true })
}
