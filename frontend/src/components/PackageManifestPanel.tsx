import { useStoreState } from '../store/guideStore'
import { useEffect, useState } from 'react'
import { useAutoSave } from '../hooks/useAutoSave'
import {
  usePackageManifestQuery,
  useWritePackageManifestMutation,
  type PackageConfigField
} from '../store/workspaceApi'
import { AlemonjsConfigEditor } from './AlemonjsConfigEditor'
import { RobotPanel } from './RobotPanel'

type Manifest = {
  name: string
  version: string
  description: string
  homepage: string
  repository: string
  license: string
  private: boolean
  access: string
  packageManager: string
  moduleType: string
  workspacesEnabled: boolean
  workspaces: string[]
  alemonjsConfig: PackageConfigField[]
  alemonjsConfigSourceReadme: string
  alemonjsConfigSourceOfficial: string
  alemonjsConfigSourcePlatform: string
  alemonjsDesktopLogo: string
  alemonjsWebRoot: string
  alemonjsWebServerPort: boolean
}

const blank: Manifest = {
  name: '',
  version: '',
  description: '',
  homepage: '',
  repository: '',
  license: '',
  private: false,
  access: 'public',
  packageManager: '',
  moduleType: '',
  workspacesEnabled: false,
  workspaces: [],
  alemonjsConfig: [],
  alemonjsConfigSourceReadme: '',
  alemonjsConfigSourceOfficial: '',
  alemonjsConfigSourcePlatform: '',
  alemonjsDesktopLogo: '',
  alemonjsWebRoot: '',
  alemonjsWebServerPort: false
}

function sameManifest(left: Manifest, right: Manifest) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function configFieldsReady(fields: PackageConfigField[]): boolean {
  return fields.every(
    field =>
      Boolean(field.name.trim()) &&
      (field.type !== 'object' || configFieldsReady(field.config ?? []))
  )
}

function saveErrorMessage(reason: unknown) {
  if (reason instanceof Error && reason.message) return reason.message
  if (reason && typeof reason === 'object') {
    const value = reason as {
      data?: { error?: unknown; message?: unknown }
      error?: unknown
      message?: unknown
    }
    if (typeof value.data?.error === 'string') return value.data.error
    if (typeof value.data?.message === 'string') return value.data.message
    if (typeof value.error === 'string') return value.error
    if (typeof value.message === 'string') return value.message
  }
  return '发布信息未保存，请检查目录权限。'
}

const sectionClass =
  'grid gap-3 rounded-xl border border-slate-200 bg-white p-4 shadow-[0_7px_18px_rgb(28_26_23/0.035)]'
const fieldClass =
  'min-h-9 w-full rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none transition focus:border-brand-600 focus:ring-2 focus:ring-brand-100 disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-400'
const labelClass = 'grid gap-1 text-xs font-semibold text-slate-600'

function SectionTitle({ title, description }: { title: string; description: string }) {
  return (
    <div className="grid gap-0.5">
      <h3 className="text-sm font-semibold text-slate-800">{title}</h3>
      <p className="text-xs leading-5 text-slate-500">{description}</p>
    </div>
  )
}

export function PackageManifestPanel({
  root,
  onSaveError
}: {
  root: string
  onSaveError: (message: string) => void
}) {
  const {
    data,
    isLoading: isInitialLoading,
    error
  } = usePackageManifestQuery(root, { skip: !root })
  const [save, { isLoading }] = useWritePackageManifestMutation()
  const [values, setValues] = useStoreState<Manifest>(blank)
  const [configText, setConfigText] = useState('[]')
  const [configError, setConfigError] = useState('')
  useEffect(() => {
    if (!data) return
    const next = {
      ...blank,
      ...data,
      access: data.access || 'public',
      workspaces: data.workspaces ?? [],
      alemonjsConfig: data.alemonjsConfig ?? []
    }
    setValues(current => (sameManifest(current, next) ? current : next))
    setConfigText(JSON.stringify(next.alemonjsConfig, null, 2))
    setConfigError('')
  }, [data, setValues])
  const saveManifest = async (next: Manifest) => {
    try {
      return await save({
        root,
        ...next,
        access: next.private ? '' : next.access
      }).unwrap()
    } catch (reason) {
      onSaveError(saveErrorMessage(reason))
    }
  }
  const scheduleSave = useAutoSave(saveManifest)
  const set = <K extends keyof Manifest>(key: K, value: Manifest[K]) => {
    const next = { ...values, [key]: value }
    setValues(next)
    scheduleSave(next)
  }
  const setWorkspaces = (text: string) => {
    set(
      'workspaces',
      text
        .split('\n')
        .map(item => item.trim())
        .filter(Boolean)
    )
  }
  const setWorkspacesEnabled = (enabled: boolean) => {
    const next = { ...values, workspacesEnabled: enabled }
    setValues(next)
    // Enabling shows an empty editor first; wait for the first pattern before
    // sending it so the user does not immediately receive a validation error.
    if (!enabled || next.workspaces.length > 0) scheduleSave(next)
  }
  const commitAlemonjsConfig = () => {
    const text = configText.trim() || '[]'
    try {
      const config: unknown = JSON.parse(text)
      if (!Array.isArray(config)) throw new Error('not-array')
      setConfigError('')
      updateAlemonjsConfig(config as PackageConfigField[])
    } catch {
      setConfigError('配置字段必须是有效的 JSON 数组；修正后离开输入框即可保存。')
    }
  }
  const updateAlemonjsConfig = (config: PackageConfigField[]) => {
    setConfigText(JSON.stringify(config, null, 2))
    const next = { ...values, alemonjsConfig: config }
    setValues(next)
    // A fresh visual field starts with an empty key. Keep it local until it
    // becomes a valid declaration instead of sending an avoidable error toast.
    if (configFieldsReady(config)) scheduleSave(next)
  }
  if (isInitialLoading)
    return (
      <section className="grid max-w-180 rounded-xl border border-slate-200 bg-white p-4 text-xs text-slate-500">
        <p>正在读取 package.json…</p>
      </section>
    )
  if (error)
    return (
      <section className="grid max-w-180 rounded-xl border border-slate-200 bg-white p-4 text-xs text-slate-500">
        <p>无法读取 package.json。</p>
      </section>
    )
  return (
    <RobotPanel
      className="package-manifest-panel max-w-180"
      title="包配置"
      description={`package.json · ${isLoading ? '正在自动保存…' : '修改后自动保存'}`}
    >
      <div className="grid gap-4">
        <section className={sectionClass}>
          <SectionTitle title="基础信息与发布" description="npm 发布、仓库链接与包可见性。" />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className={labelClass}>
              包名
              <input
                className={fieldClass}
                value={values.name}
                onChange={event => set('name', event.target.value)}
                placeholder="@scope/package-name"
              />
            </label>
            <label className={labelClass}>
              版本
              <input
                className={fieldClass}
                value={values.version}
                onChange={event => set('version', event.target.value)}
                placeholder="1.2.3"
              />
            </label>
            <label className={`col-span-full ${labelClass}`}>
              描述
              <input
                className={fieldClass}
                value={values.description}
                onChange={event => set('description', event.target.value)}
                placeholder="一句话说明这个包的用途"
              />
            </label>
            <label className={labelClass}>
              主页
              <input
                className={fieldClass}
                value={values.homepage}
                onChange={event => set('homepage', event.target.value)}
                placeholder="https://example.com"
              />
            </label>
            <label className={labelClass}>
              仓库
              <input
                className={fieldClass}
                value={values.repository}
                onChange={event => set('repository', event.target.value)}
                placeholder="https://github.com/owner/repo"
              />
            </label>
            <label className={labelClass}>
              许可证
              <input
                className={fieldClass}
                value={values.license}
                onChange={event => set('license', event.target.value)}
                placeholder="MIT"
              />
            </label>
            <label className={labelClass}>
              发布权限
              <select
                className={fieldClass}
                value={values.access}
                disabled={values.private}
                onChange={event => set('access', event.target.value)}
              >
                <option value="public">公开（public）</option>
                <option value="restricted">受限（restricted）</option>
              </select>
            </label>
            <label className="col-span-full flex items-start gap-2 rounded-lg bg-slate-50 px-3 py-2.5 text-xs text-slate-600">
              <input
                className="mt-0.5 size-4 accent-brand-600"
                type="checkbox"
                checked={values.private}
                onChange={event => set('private', event.target.checked)}
              />
              <span className="grid gap-0.5">
                <strong className="text-sm text-slate-800">私有包</strong>
                <span>写入 <code>private: true</code>，npm 会拒绝发布此包。</span>
              </span>
            </label>
          </div>
        </section>

        <section className={sectionClass}>
          <SectionTitle title="工作区与运行时" description="适用于 monorepo；保留现有工作区对象中的 nohoist 等附加配置。" />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className={labelClass}>
              模块类型
              <select
                className={fieldClass}
                value={values.moduleType}
                onChange={event => set('moduleType', event.target.value)}
              >
                <option value="">未指定</option>
                <option value="module">ESM（module）</option>
                <option value="commonjs">CommonJS（commonjs）</option>
              </select>
            </label>
            <label className={labelClass}>
              包管理器
              <input
                className={fieldClass}
                value={values.packageManager}
                onChange={event => set('packageManager', event.target.value)}
                placeholder="pnpm@9.15.0"
              />
            </label>
            <label className="col-span-full flex items-start gap-2 rounded-lg bg-slate-50 px-3 py-2.5 text-xs text-slate-600">
              <input
                className="mt-0.5 size-4 accent-brand-600"
                type="checkbox"
                checked={values.workspacesEnabled}
                onChange={event => setWorkspacesEnabled(event.target.checked)}
              />
              <span className="grid gap-0.5">
                <strong className="text-sm text-slate-800">启用工作空间</strong>
                <span>使用 <code>workspaces</code> 管理子包；每行一个目录匹配规则。</span>
              </span>
            </label>
            {values.workspacesEnabled && (
              <label className={`col-span-full ${labelClass}`}>
                工作空间目录
                <textarea
                  className={`${fieldClass} min-h-20 py-2 font-mono text-xs`}
                  value={values.workspaces.join('\n')}
                  onChange={event => setWorkspaces(event.target.value)}
                  placeholder={'packages/*\nplugins/*'}
                />
              </label>
            )}
          </div>
        </section>

        <section className={sectionClass}>
          <SectionTitle title="AlemonJS 声明" description="机器人工作台据此识别配置来源、桌面入口与 WebView。未填写的字段不会写入 package.json。" />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className={labelClass}>
              配置说明地址
              <input
                className={fieldClass}
                value={values.alemonjsConfigSourceReadme}
                onChange={event => set('alemonjsConfigSourceReadme', event.target.value)}
                placeholder="README 或配置文档地址"
              />
            </label>
            <label className={labelClass}>
              官方地址
              <input
                className={fieldClass}
                value={values.alemonjsConfigSourceOfficial}
                onChange={event => set('alemonjsConfigSourceOfficial', event.target.value)}
                placeholder="https://example.com"
              />
            </label>
            <label className={labelClass}>
              平台标识
              <input
                className={fieldClass}
                value={values.alemonjsConfigSourcePlatform}
                onChange={event => set('alemonjsConfigSourcePlatform', event.target.value)}
                placeholder="qq / onebot / custom"
              />
            </label>
            <label className={labelClass}>
              桌面图标
              <input
                className={fieldClass}
                value={values.alemonjsDesktopLogo}
                onChange={event => set('alemonjsDesktopLogo', event.target.value)}
                placeholder="图标 URL 或包内资源路径"
              />
            </label>
            <label className={labelClass}>
              Web 根目录
              <input
                className={fieldClass}
                value={values.alemonjsWebRoot}
                onChange={event => set('alemonjsWebRoot', event.target.value)}
                placeholder="dist"
              />
            </label>
            <label className="flex items-end pb-0.5 text-xs font-semibold text-slate-600">
              <span className="flex min-h-9 items-center gap-2 rounded-md bg-slate-50 px-3">
                <input
                  className="size-4 accent-brand-600"
                  type="checkbox"
                  checked={values.alemonjsWebServerPort}
                  onChange={event => set('alemonjsWebServerPort', event.target.checked)}
                />
                WebView 使用服务端端口
              </span>
            </label>
            <div className="col-span-full grid gap-2">
              <div className="grid gap-0.5">
                <strong className="text-xs font-semibold text-slate-700">配置字段（<code>alemonjs.config</code>）</strong>
                <span className="text-xs text-slate-500">按字段声明自动生成机器人配置表单，支持嵌套对象和正则校验。</span>
              </div>
              <AlemonjsConfigEditor fields={values.alemonjsConfig} onChange={updateAlemonjsConfig} />
              <details className="group rounded-lg border border-slate-200 bg-slate-50/60 p-3">
                <summary className="cursor-pointer list-none text-xs font-semibold text-slate-600 marker:content-none [&::-webkit-details-marker]:hidden">
                  高级 JSON 编辑
                  <span className="ml-1 font-normal text-slate-400">用于粘贴或微调完整声明</span>
                </summary>
                <div className="grid gap-1.5 pt-3">
                  <textarea
                    className={`${fieldClass} min-h-52 resize-y py-2 font-mono text-xs leading-5 ${configError ? 'border-red-400 focus:border-red-500 focus:ring-red-100' : ''}`}
                    value={configText}
                    onChange={event => setConfigText(event.target.value)}
                    onBlur={commitAlemonjsConfig}
                    spellCheck={false}
                    placeholder={'[\n  {\n    "name": "token",\n    "type": "string",\n    "required": true,\n    "description": "访问令牌"\n  }\n]'}
                  />
                  {configError && <span className="text-xs font-normal text-red-600">{configError}</span>}
                </div>
              </details>
            </div>
          </div>
        </section>
      </div>
    </RobotPanel>
  )
}
