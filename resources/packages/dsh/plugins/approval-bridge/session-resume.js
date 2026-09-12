import { HarnessSdkJsonRpcServer } from '@deepseek-ai/dsh-sdk-jsonrpc-server'

// The pinned SDK only creates identities absent from its in-process map.
// Rehydrate an existing durable identity via the agent registry, retaining the
// SDK's handle ownership, serialization and shutdown behavior.
const installed = Symbol.for('alemonx.dsh.sdk.session-resume')
export function installSessionResume() {
  const prototype = HarnessSdkJsonRpcServer.prototype
  if (prototype[installed]) return
  const create = prototype.createSession
  const handleRequest = prototype.handleRequest
  prototype.handleRequest = async function (method, params) {
    if (method !== 'alemonx/session/history') return handleRequest.call(this, method, params)
    const record = await this.getOrCreateSession(params.sessionId)
    const messages = []
    let final = ''
    const flush = () => {
      if (final) messages.push({ role: 'assistant', content: final })
      final = ''
    }
    const text = blocks => Array.isArray(blocks) ? blocks.filter(b => b.type === 'text').map(b => b.text).join('') : ''
    for (const event of record.handle.agent.session.snapshotEvents()) {
      if (event.type === 'user/message') {
        flush()
        const content = text(event.data.content)
        if (content) messages.push({ role: 'user', content })
      } else if (event.type === 'assistant/message') {
        const content = text(event.data.message.content)
        if (content) final = content
      } else if (event.type === 'turn/end') flush()
    }
    flush()
    return { messages }
  }
  if (typeof create !== 'function') throw new Error('Unsupported DSH session lifecycle')
  prototype.createSession = async function (sessionId) {
    try {
      return await create.call(this, sessionId)
    } catch (error) {
      // Never turn arbitrary initialization, permission or storage failures
      // into a resume or silently replace a conversation with a fresh id.
      if (!(error instanceof Error) || error.message !== `session "${sessionId}" already exists`) throw error
      const handle = await this.ctx.agents.resume({
        resumeSessionId: sessionId,
        agentOptions: {
          provider: this.provider,
          model: this.model,
          ...(this.reasoningEffort === undefined ? {} : { reasoningEffort: this.reasoningEffort }),
          ...(this.maxTokens === undefined ? {} : { maxTokens: this.maxTokens })
        }
      })
      const record = { handle }
      this.sessions.set(sessionId, record)
      return record
    }
  }
  prototype[installed] = true
}
