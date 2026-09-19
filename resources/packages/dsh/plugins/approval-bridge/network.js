// Only the bundled DeepSeek provider is delegated to the host. Local approval
// IPC and arbitrary tools/robot traffic are not intercepted.
export function installNetworkTransport(bridgeURL, token) {
  if (!bridgeURL || !token || globalThis[Symbol.for('alemonx.network')]) return
  const nativeFetch = globalThis.fetch.bind(globalThis)
  globalThis[Symbol.for('alemonx.network')] = true
  globalThis.fetch = async (input, init) => {
    const request = new Request(input, init)
    const target = new URL(request.url)
    if (target.hostname !== 'api.deepseek.com') return nativeFetch(request)
    if (target.protocol !== 'https:' || target.port || target.username || target.password) throw new Error('Invalid provider endpoint')
    const headers = new Headers(request.headers)
    headers.set('x-alx-dsh-bridge', token)
    headers.set('x-alx-network-url', target.href)
    return nativeFetch(new URL('/network', bridgeURL), {
      method: request.method,
      headers,
      body: request.body,
      duplex: 'half',
      signal: request.signal,
      redirect: 'error'
    })
  }
}

installNetworkTransport(process.env.ALX_DSH_BRIDGE_URL, process.env.ALX_DSH_BRIDGE_TOKEN)
