import { bridge } from './nativeWindow'

type PasswordCredential = Credential & { password: string }
const PasswordCredential = (
  window as Window & {
    PasswordCredential?: new (data: {
      id: string
      password: string
    }) => PasswordCredential
  }
).PasswordCredential

const lastAccountKey = 'alx:auth:last-account'
const rememberPasswordKey = 'alx:auth:remember-password'

function readPreference(key: string) {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export function lastAccount() {
  return readPreference(lastAccountKey)
}

export function rememberAccount(account: string) {
  try {
    localStorage.setItem(lastAccountKey, account)
  } catch {
    // The credential form remains usable with browser storage disabled.
  }
}

export const canRememberPassword = Boolean(
  bridge || (PasswordCredential && navigator.credentials)
)

export function remembersPassword(account: string) {
  return (
    canRememberPassword &&
    !!account &&
    readPreference(rememberPasswordKey) === account
  )
}

function nativeCredential(
  action: 'get' | 'set' | 'delete',
  account: string,
  password?: string
) {
  return new Promise<string>((resolve, reject) => {
    const id = Array.from(crypto.getRandomValues(new Uint8Array(16)), value =>
      value.toString(16).padStart(2, '0')
    ).join('')
    const finish = () => {
      window.clearTimeout(timer)
      window.removeEventListener('alx:workbench-credential', receive)
    }
    const receive = (event: Event) => {
      const result = (
        event as CustomEvent<{ id: string; password?: string; error?: string }>
      ).detail
      if (result?.id !== id) return
      finish()
      if (result.error) reject(new Error(result.error))
      else resolve(result.password ?? '')
    }
    const timer = window.setTimeout(() => {
      finish()
      reject(new Error('系统密码管理器响应超时。'))
    }, 30000)
    window.addEventListener('alx:workbench-credential', receive)
    bridge?.postMessage(
      `alx:workbench-credential:${JSON.stringify({ id, action, account, password })}`
    )
  })
}

export async function readRememberedPassword(
  account: string,
  signal: AbortSignal
) {
  if (!remembersPassword(account)) return ''
  if (bridge) return nativeCredential('get', account)
  const options: CredentialRequestOptions & { password: boolean } = {
    password: true,
    mediation: 'silent',
    signal
  }
  const credential = (await navigator.credentials.get(
    options
  )) as PasswordCredential | null
  return credential?.id === account ? credential.password : ''
}

export async function setRememberedPassword(
  account: string,
  password: string,
  remember: boolean
) {
  const previousAccount = readPreference(rememberPasswordKey)
  if (bridge && previousAccount && (!remember || previousAccount !== account)) {
    await nativeCredential('delete', previousAccount)
  }
  if (remember && bridge) await nativeCredential('set', account, password)
  else if (remember && PasswordCredential && navigator.credentials) {
    await navigator.credentials.store(
      new PasswordCredential({ id: account, password })
    )
  }
  if (remember) localStorage.setItem(rememberPasswordKey, account)
  else if (previousAccount) localStorage.removeItem(rememberPasswordKey)
}
