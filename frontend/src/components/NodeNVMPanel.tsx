import { useState } from 'react'
import {
  useManageNVMNodeMutation,
  useNvmNodeStatusQuery
} from '../store/workspaceApi'

export function NodeNVMPanel({ onChanged }: { onChanged: () => void }) {
  const [version, setVersion] = useState('')
  const [message, setMessage] = useState('')
  const { data: status, isLoading, error, refetch } = useNvmNodeStatusQuery(
    undefined,
    { refetchOnMountOrArgChange: true }
  )
  const [manage, { isLoading: isManaging }] = useManageNVMNodeMutation()
  const currentVersion = status?.activeVersion ?? ''

  const run = async (action: 'install' | 'use', target: string) => {
    setMessage('')
    try {
      const result = await manage({ action, version: target }).unwrap()
      setMessage(result.output || 'NodeJS 版本已更新。')
      onChanged()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : 'NodeJS 操作未完成。')
    }
  }
  const errorMessage =
    error && 'data' in error && typeof error.data === 'object' && error.data
      ? (error.data as { error?: string }).error
      : '无法读取 NodeJS 版本。'

  return (
    <section className="grid gap-3 py-3">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">NodeJS 版本管理</strong>
      </header>

      <div className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/80 p-3.5 dark:border-slate-800 dark:bg-slate-900/45">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-slate-500">当前使用</span>
          {currentVersion ? <code className="rounded bg-emerald-100 px-2 py-1 text-xs font-semibold text-emerald-800 dark:bg-emerald-950/70 dark:text-emerald-200">{currentVersion}</code> : <span className="text-xs text-amber-700 dark:text-amber-300">尚未设置</span>}
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-slate-500">推荐使用</span>
          <div className="flex items-center gap-1.5">
            <code className="text-xs font-semibold text-slate-700 dark:text-slate-200">{status?.recommendedVersion ?? 'v22.22.3'}</code>
            <span className={status?.recommendedInstalled ? 'rounded-full bg-emerald-100 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-950 dark:text-amber-300'}>
              {status?.recommendedInstalled ? '已下载' : '未下载'}
            </span>
          </div>
        </div>
        <div className="h-px bg-slate-200 dark:bg-slate-800" />
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-slate-500">最新版本（LTS）</span>
          {status?.latestVersion ? (
            <div className="flex items-center gap-1.5">
              <code className="text-xs font-semibold text-slate-700 dark:text-slate-200">{status.latestVersion}</code>
              <span className={status.latestInstalled ? 'rounded-full bg-emerald-100 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-950 dark:text-amber-300'}>
                {status.latestInstalled ? '已下载' : '未下载'}
              </span>
            </div>
          ) : <span className="text-xs text-slate-400">读取失败</span>}
        </div>
      </div>

      <div className="grid gap-2">
        <label className="text-xs font-medium text-slate-700 dark:text-slate-200" htmlFor="nvm-node-version">安装指定版本</label>
        <div className="flex gap-2">
          <input
            id="nvm-node-version"
            className="min-w-0 flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-xs text-slate-900 outline-none focus:border-brand-400 focus:ring-2 focus:ring-brand-100 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100 dark:focus:ring-brand-900/40"
            placeholder="例如 22.22.3"
            value={version}
            onChange={event => setVersion(event.target.value)}
          />
          <button className="primary-button shrink-0" disabled={!version.trim() || isManaging} onClick={() => void run('install', version)}>
            {isManaging ? '安装中…' : '安装'}
          </button>
        </div>
      </div>

      <div className="grid gap-2 border-t border-slate-200 pt-3 dark:border-slate-800">
        <div className="flex items-center justify-between gap-2">
          <strong className="text-xs font-semibold text-slate-700 dark:text-slate-200">已下载版本</strong>
          <span className="text-[11px] text-slate-500">{status?.versions.length ?? 0} 个</span>
        </div>
        {isLoading ? <p className="m-0 text-xs text-slate-500">正在读取已下载版本…</p> : error ? (
          <div className="flex items-center gap-3 text-xs text-amber-700 dark:text-amber-300">
            <span>{errorMessage}</span>
            <button className="font-semibold hover:underline" onClick={() => void refetch()}>重试</button>
          </div>
        ) : status?.versions.length ? (
          <div className="grid gap-1.5">
            {status.versions.map(item => {
              const active = item === currentVersion
              return (
                <div className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-900/35" key={item}>
                  <code className="min-w-0 flex-1 text-xs font-medium text-slate-700 dark:text-slate-200">{item}</code>
                  {active && <span className="rounded-full bg-emerald-100 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">当前</span>}
                  <button className="rounded-md px-2 py-1 text-xs font-semibold text-brand-600 hover:bg-brand-50 disabled:opacity-50 dark:text-brand-300 dark:hover:bg-slate-800" disabled={active || isManaging} onClick={() => void run('use', item)}>
                    {active ? '正在使用' : '切换'}
                  </button>
                </div>
              )
            })}
          </div>
        ) : <p className="m-0 text-xs leading-5 text-slate-500">暂无已下载版本。</p>}
      </div>
      {message && <p className="m-0 rounded-lg bg-slate-100 px-3 py-2 text-xs leading-5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">{message}</p>}
    </section>
  )
}
