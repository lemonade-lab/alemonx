import { useEffect, useState } from 'react'
import { Check, Loader2, LogOut, RotateCcw, Save } from 'lucide-react'
import { Button } from './Button'
import { GithubMark } from './GithubMark'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

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
    <SettingsPage
      title="GitHub"
      description="配置 GitHub OAuth 客户端与登录状态。"
    >
      <SettingsCard
        icon={<GithubMark className="size-4" />}
        title="GitHub OAuth App（Client ID）"
        description="用于“使用 GitHub 登录”。项目内置默认值，这里可自定义覆盖，留空恢复默认。"
        actions={
          <span className="settings-pill">当前来源：{sourceLabels[source]}</span>
        }
      >
        <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
          Client ID
          <input
            className="settings-input"
            value={clientId}
            onChange={event => {
              setClientId(event.target.value)
              setMessage('')
            }}
            placeholder="如 Iv1.xxxxxxxxxxxx"
          />
        </label>
        <div className="settings-card-actions">
          <Button
            variant="primary"
            className="gap-1.5"
            disabled={busy}
            onClick={() => void saveClientID()}
          >
            {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            保存
          </Button>
          <Button
            variant="secondary"
            className="gap-1.5"
            disabled={busy || !clientId}
            onClick={() => void resetClientID()}
          >
            <RotateCcw className="size-3.5" />
            恢复默认
          </Button>
        </div>
      </SettingsCard>

      <SettingsCard
        icon={<GithubMark className="size-4" />}
        title="GitHub 登录状态"
        description="授权后 GitHub API 配额提升至 5000 次/小时；点击窗口顶部的 GitHub 图标可发起授权登录。"
      >
        {status ? (
          status.loggedIn ? (
            <div className="settings-card-actions">
              <span className="settings-pill is-success">
                <Check className="size-3.5" />
                {status.login ? `已登录：${status.login}` : '已登录 GitHub'}
              </span>
              <Button
                variant="secondary"
                className="gap-1.5"
                disabled={busy}
                onClick={() => void logout()}
              >
                <LogOut className="size-3.5" />
                退出登录
              </Button>
            </div>
          ) : (
            <p className="settings-message is-info m-0 inline-flex items-center gap-1.5">
              <GithubMark className="size-4" />
              未登录；点击窗口顶部的 GitHub 图标发起授权登录。
            </p>
          )
        ) : (
          <p className="m-0 text-xs text-(--theme-text-muted)">
            正在读取登录状态…
          </p>
        )}
      </SettingsCard>

      {message && (
        <SettingsMessage tone="success">{message}</SettingsMessage>
      )}
      {error && (
        <SettingsMessage tone="error">{error}</SettingsMessage>
      )}
    </SettingsPage>
  )
}
