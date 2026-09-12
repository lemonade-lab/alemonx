import { defineTool } from '@deepseek-ai/dsh-tools'
import { installSessionResume } from './session-resume.js'

installSessionResume()

// Cordis deliberately hides services that a plugin did not declare. The
// bridge owns tool registration, so this is required before apply() may read
// ctx.tools during SDK initialization.
export const inject = ['tools']

// ALemonX's DSH Host answerer. It deliberately carries no operation
// arguments: the loopback host renders only a safe tool-category summary and
// applies the decision to exactly this request.
export function apply(ctx, config = {}) {
  const url = typeof config.url === 'string' ? config.url : ''
  const token = typeof config.token === 'string' ? config.token : ''
  if (!url || !token) return
  // The bundled SDK profile contains many convenience tools. ALemonX narrows
  // their *execution* surface to project inspection plus explicitly-approved
  // file mutation and commands. Keeping the guard here means even a future
  // profile patch cannot silently make a newly mounted tool executable.
  const readOnlyTools = new Set(['read', 'read_image', 'glob', 'grep', 'pm2_status', 'pm2_logs'])
  const approvalTools = new Set(['write', 'edit'])
  ctx.on('tools/pre-execute', async (execution, next) => {
    if (readOnlyTools.has(execution.name)) return next()
    if (approvalTools.has(execution.name)) return { kind: 'ask', reason: `ALemonX requires one-time approval for ${execution.name}` }
    return { kind: 'deny', reason: `tool "${execution.name}" is not enabled by the ALemonX bridge` }
  })
  ctx.on('approval/request', async (request, next) => {
    if (request.signal?.aborted) return 'cancelled'
    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          'x-alx-dsh-bridge': token
        },
        body: JSON.stringify({
          sessionId: String(request.agent.session.id),
          toolName: request.toolName,
          // DSH renders this for its own audit log, but the host intentionally
          // discards it rather than exposing provider/tool context to browsers.
          reason: request.reason ?? ''
        }),
        signal: request.signal
      })
      if (!response.ok) return 'unavailable'
      const body = await response.json()
      return ['allowed-once', 'rejected', 'cancelled', 'unavailable'].includes(body?.outcome) ? body.outcome : 'unavailable'
    } catch (error) {
      return request.signal?.aborted ? 'cancelled' : 'unavailable'
    }
  })
  for (const [name, action, description] of [
    ['pm2_status', 'status', 'Read the current PM2 status for this project.'],
    ['pm2_logs', 'logs', 'Read a redacted recent PM2 log summary for this project.'],
    ['pm2_restart', 'restart', 'Restart this project through the guarded PM2 controller.'],
    ['pm2_reload', 'reload', 'Reload this project through the guarded PM2 controller.']
  ]) {
    ctx.tools.register(defineTool({
      name,
      description,
      parameters: {},
      output: {
        schema: { type: 'object', additionalProperties: false, properties: { summary: { type: 'string', required: true } } },
        render: (_args, value) => [{ type: 'text', text: value.summary }]
      },
      async execute(_args, exec) {
        const sessionId = exec.agent?.session?.id
        if (!sessionId) throw new Error('PM2 bridge requires an active DSH session')
        const response = await fetch(new URL('/pm2', url), {
          method: 'POST',
          headers: { 'content-type': 'application/json', 'x-alx-dsh-bridge': token },
          body: JSON.stringify({ sessionId: String(sessionId), action }),
          signal: exec.signal
        })
        if (!response.ok) throw new Error('PM2 bridge request was denied')
        const body = await response.json()
        if (typeof body?.summary !== 'string') throw new Error('PM2 bridge returned an invalid result')
        return { summary: body.summary.slice(0, 12000) }
      }
    }))
  }
}
