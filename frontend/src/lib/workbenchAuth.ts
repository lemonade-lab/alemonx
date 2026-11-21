export type AuthStatus = {
  enabled: boolean
  authenticated: boolean
  account?: string
  githubLogin?: string
}

export async function authRequest(path: string, init?: RequestInit) {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 8000)
  try {
    const response = await fetch(`/api/v1/auth/${path}`, {
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
      signal: controller.signal,
      ...init
    })
    const data = (await response.json()) as AuthStatus & { error?: string }
    if (!response.ok) throw new Error(data.error || '身份认证操作未完成。')
    return data
  } finally {
    window.clearTimeout(timer)
  }
}

export function readAuthStatus() {
  return authRequest('status')
}
