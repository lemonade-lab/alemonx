import { apply, inject } from '../plugins/approval-bridge/index.js'

if (!Array.isArray(inject) || !inject.includes('tools')) {
  throw new Error('DSH bridge must declare the tools injection')
}

const handlers = {}
const tools = []
apply({
  on(name, handler) { handlers[name] = handler },
  tools: { register(tool) { tools.push(tool) } }
}, { url: 'http://127.0.0.1:1/approval', token: 'verification-token' })

const allow = await handlers['tools/pre-execute']({ name: 'pm2_status' }, async () => ({ kind: 'allow' }))
const ask = await handlers['tools/pre-execute']({ name: 'write' }, async () => ({ kind: 'allow' }))
const deny = await handlers['tools/pre-execute']({ name: 'bash' }, async () => ({ kind: 'allow' }))
if (allow.kind !== 'allow' || ask.kind !== 'ask' || deny.kind !== 'deny' || tools.length !== 4) {
  throw new Error('DSH bridge capability contract failed')
}
