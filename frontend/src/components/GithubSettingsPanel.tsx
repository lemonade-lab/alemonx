import { useEffect, useState } from 'react'
import { Check, Loader2, LogOut, RotateCcw, Save } from 'lucide-react'
import { GithubMark } from './GithubMark'

type ClientIDInfo = {
  clientId: string
  source: '' | 'env' | 'file' | 'builtin'
}

type AuthStatus = {
  loggedIn: boolean
  login?: string
}

const sourceLabels: Record<ClientIDInfo['source'], string> = {
  '': '未配置',
  env: '环境变量',
  file: '配置文件',
  builtin: '内置默认'
}

export function GithubSettingsPanel() {
  const [clientId, setClientId] = useState('')
  const [source, setSource] = useState<ClientIDInfo['source']>('')
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const load = async () => {
    setError('')
    try {
      const [idResponse, statusResponse] = await Promise.all([
        fetch('/api/v1/github/auth/client-id'),
        fetch('/api/v1/github/auth/status')
      ])
      const idData = (await idResponse.json()) as ClientIDInfo & {
        error?: string
      }
      if (!idResponse.ok) throw new Error(idData.error ?? '读取失败')
      setClientId(idData.clientId ?? '')
      setSource(idData.source ?? '')
      const statusData = (await statusResponse.json()) as AuthStatus & {
        error?: string
      }
      if (statusResponse.ok) setStatus(statusData)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '读取失败')
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const saveClientID = async () => {
    setBusy(true)
    setMessage('')
    setError('')
    try {
      const response = await fetch('/api/v1/github/auth/client-id', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ clientId: clientId.trim() })
      })
      const data = (await response.json()) as ClientIDInfo & { error?: string }
      if (!response.ok) throw new Error(data.error ?? '保存失败')
      setClientId(data.clientId ?? '')
      setSource(data.source ?? '')
      setMessage('已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const resetClientID = async () => {
    setClientId('')
    await saveClientID()
  }

  const logout = async () => {
    setBusy(true)
    setError('')
    try {
      const response = await fetch('/api/v1/github/auth/logout', {
        method: 'POST'
      })
      if (!response.ok) {
        const data = (await response.json()) as { error?: string }
        throw new Error(data.error ?? '退出登录失败')
      }
      setStatus(current => (current ? { ...current, loggedIn: false } : current))
      setMessage('已退出 GitHub 登录')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '退出登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-4">
      <section className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
        <header className="grid gap-0.5">
          <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            GitHub OAuth App（Client ID）
          </strong>
          <small className="text-xs text-slate-500 dark:text-slate-400">
            用于“使用 GitHub 登录”。项目内置默认值，这里可自定义覆盖，留空恢复默认。
          </small>
        </header>
        <label className="grid gap-1 text-xs font-semibold text-slate-600 dark:text-slate-300">
          Client ID
          <input
            className="min-h-9 rounded-md border border-slate-300 bg-white px-2.5 text-sm text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100"
            value={clientId}
            onChange={event => {
              setClientId(event.target.value)
              setMessage('')
            }}
            placeholder="如 Iv1.xxxxxxxxxxxx"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded-md bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-500 dark:bg-slate-800 dark:text-slate-400">
            当前来源：{sourceLabels[source]}
          </span>
          <button
            className="inline-flex min-h-8 items-center gap-1.5 rounded-md bg-brand-600 px-3 text-xs font-semibold text-white hover:bg-brand-700 disabled:opacity-60"
            disabled={busy}
            onClick={() => void saveClientID()}
          >
            {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            保存
          </button>
          <button
            className="inline-flex min-h-8 items-center gap-1.5 rounded-md border border-slate-200 px-2.5 text-xs font-semibold text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
            disabled={busy || !clientId}
            onClick={() => void resetClientID()}
          >
            <RotateCcw className="size-3.5" />
            恢复默认
          </button>
        </div>
      </section>

      <section className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
        <header className="grid gap-0.5">
          <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            GitHub 登录状态
          </strong>
          <small className="text-xs text-slate-500 dark:text-slate-400">
            授权后 GitHub API 配额提升至 5000 次/小时；点击窗口顶部的 GitHub
            图标可发起授权登录。
          </small>
        </header>
        {status ? (
          status.loggedIn ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1.5 rounded-md border border-emerald-200 bg-emerald-50 px-2.5 py-1.5 text-xs font-semibold text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300">
                <Check className="size-3.5" />
                {status.login ? `已登录：${status.login}` : '已登录 GitHub'}
              </span>
              <button
                className="inline-flex min-h-8 items-center gap-1.5 rounded-md border border-slate-200 px-2.5 text-xs font-semibold text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                disabled={busy}
                onClick={() => void logout()}
              >
                <LogOut className="size-3.5" />
                退出登录
              </button>
            </div>
          ) : (
            <p className="m-0 inline-flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
              <GithubMark className="size-4" />
              未登录；点击窗口顶部的 GitHub 图标发起授权登录。
            </p>
          )
        ) : (
          <p className="m-0 text-xs text-slate-500">正在读取登录状态…</p>
        )}
      </section>

      {message && (
        <p className="m-0 rounded-md border border-emerald-200 bg-emerald-50 px-2.5 py-2 text-[11px] text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300">
          {message}
        </p>
      )}
      {error && (
        <p className="m-0 rounded-md border border-red-200 bg-red-50 px-2.5 py-2 text-[11px] text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300">
          {error}
        </p>
      )}
    </div>
  )
}
