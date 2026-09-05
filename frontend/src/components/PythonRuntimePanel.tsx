import { useState } from 'react'
import { useManagePythonRuntimeMutation, usePythonRuntimeStatusQuery } from '../store/workspaceApi'

export function PythonRuntimePanel({ onChanged }: { onChanged: () => void }) {
  const [version, setVersion] = useState('')
  const [message, setMessage] = useState('')
  const { data: status, isLoading, error, refetch } = usePythonRuntimeStatusQuery(undefined, { refetchOnMountOrArgChange: true })
  const [manage, { isLoading: working }] = useManagePythonRuntimeMutation()
  const current = status?.activeVersion ?? ''
  const run = async (action: 'install' | 'use', target: string) => {
    setMessage('')
    try {
      const result = await manage({ action, version: target }).unwrap()
      setMessage(result.output)
      onChanged()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : 'Python 操作未完成。')
    }
  }
  return <section className="grid gap-3 py-3">
    <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">Python 版本管理</strong>
    <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50/80 p-3.5 dark:border-slate-800 dark:bg-slate-900/45">
      <span className="text-xs font-medium text-slate-500">当前使用</span>
      {current ? <code className="rounded bg-emerald-100 px-2 py-1 text-xs font-semibold text-emerald-800 dark:bg-emerald-950/70 dark:text-emerald-200">Python {current}</code> : <span className="text-xs text-amber-700 dark:text-amber-300">未检测到 Python</span>}
    </div>
    <div className="grid gap-2">
      <label className="text-xs font-medium text-slate-700 dark:text-slate-200" htmlFor="python-version">安装指定版本</label>
      <div className="flex gap-2"><input id="python-version" className="min-w-0 flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-xs text-slate-900 outline-none focus:border-brand-400 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" placeholder="例如 3.12.10" value={version} onChange={event => setVersion(event.target.value)} /><button className="primary-button shrink-0" disabled={!version.trim() || working} onClick={() => void run('install', version)}>{working ? '安装中…' : '安装'}</button></div>
    </div>
    <div className="grid gap-2 border-t border-slate-200 pt-3 dark:border-slate-800">
      <div className="flex items-center justify-between"><strong className="text-xs font-semibold text-slate-700 dark:text-slate-200">已下载版本</strong><span className="text-[11px] text-slate-500">{status?.versions.length ?? 0} 个</span></div>
      {isLoading ? <p className="m-0 text-xs text-slate-500">正在读取已下载版本…</p> : error ? <button className="text-left text-xs font-semibold text-brand-600" onClick={() => void refetch()}>无法读取，点击重试</button> : status?.versions.length ? <div className="grid gap-1.5">{status.versions.map(item => { const active = item === current; return <div className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-900/35" key={item}><code className="min-w-0 flex-1 text-xs font-medium text-slate-700 dark:text-slate-200">{item}</code>{active && <span className="rounded-full bg-emerald-100 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">当前</span>}<button className="rounded-md px-2 py-1 text-xs font-semibold text-brand-600 hover:bg-brand-50 disabled:opacity-50 dark:text-brand-300" disabled={active || working} onClick={() => void run('use', item)}>{active ? '正在使用' : '切换'}</button></div> })}</div> : <p className="m-0 text-xs text-slate-500">暂无已下载版本。</p>}
    </div>
    {message && <p className="m-0 rounded-lg bg-slate-100 px-3 py-2 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-300">{message}</p>}
  </section>
}
