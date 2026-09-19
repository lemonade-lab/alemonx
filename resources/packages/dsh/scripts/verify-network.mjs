import assert from 'node:assert/strict'
import { installNetworkTransport } from '../plugins/approval-bridge/network.js'

const original = globalThis.fetch
const calls = []
globalThis.fetch = async (input, init) => {
  const request = new Request(input, init)
  calls.push(request)
  return new Response('data: ok\n\n', { headers: { 'content-type': 'text/event-stream' } })
}
try {
  installNetworkTransport('http://127.0.0.1:17391/approval', 'bridge-secret')
  const response = await fetch('https://api.deepseek.com/chat/completions', {
    method: 'POST', headers: { authorization: 'Bearer provider-secret' }, body: '{"stream":true}'
  })
  assert.equal(await response.text(), 'data: ok\n\n')
  assert.equal(calls[0].url, 'http://127.0.0.1:17391/network')
  assert.equal(calls[0].headers.get('x-alx-network-url'), 'https://api.deepseek.com/chat/completions')
  assert.equal(calls[0].headers.get('authorization'), 'Bearer provider-secret')
  assert.equal(calls[0].headers.get('x-alx-dsh-bridge'), 'bridge-secret')
  assert.equal(await calls[0].text(), '{"stream":true}')
  await fetch('http://127.0.0.1:17391/approval', { method: 'POST', body: '{}' })
  assert.equal(calls[1].url, 'http://127.0.0.1:17391/approval')
  assert.equal(calls[1].headers.get('x-alx-network-url'), null)
  await assert.rejects(fetch('http://api.deepseek.com/chat/completions'))
} finally {
  globalThis.fetch = original
  delete globalThis[Symbol.for('alemonx.network')]
}
