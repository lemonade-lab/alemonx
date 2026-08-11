import { useStoreState } from '../store/guideStore'
import { useEffect } from 'react'
import { Package } from 'lucide-react'
import { Tabs } from './Tabs'
import { RobotPanel } from './RobotPanel'

type Props = {
  content: string
  onChange: (content: string) => void
}

const officialRegistry = 'https://registry.npmjs.org/'
const mirrorRegistry = 'https://registry.npmmirror.com/'

function registryFrom(content: string) {
  return content.match(/^\s*registry\s*=\s*(.+?)\s*$/m)?.[1] ?? officialRegistry
}

function withRegistry(content: string, registry: string) {
  const lines = content
    .split(/\r?\n/)
    .filter(line => !/^\s*registry\s*=/.test(line) && line.trim())
  return [...lines, `registry=${registry}`].join('\n') + '\n'
}

export function NpmrcConfigForm({ content, onChange }: Props) {
  const [editor, setEditor] = useStoreState<'visual' | 'text'>('visual')
  const [preset, setPreset] = useStoreState(officialRegistry)
  const [customRegistry, setCustomRegistry] = useStoreState('')

  useEffect(() => {
    const registry = registryFrom(content)
    if (registry === officialRegistry || registry === mirrorRegistry) {
      setPreset(registry)
    } else {
      setPreset('custom')
      setCustomRegistry(registry)
    }
  }, [content, setCustomRegistry, setPreset])

  const updateRegistry = (candidate: string) => {
    const registry = candidate.trim()
    if (!registry) return
    onChange(withRegistry(content, registry))
  }

  const mode = (
    <Tabs
      ariaLabel="编辑模式"
      value={editor}
      onChange={setEditor}
      variant="segmented"
      items={[
        { id: 'visual', label: '表单' },
        { id: 'text', label: '文本' }
      ]}
    />
  )
  const fieldClass =
    'min-h-9 w-full rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none transition focus:border-brand-600 focus:ring-2 focus:ring-brand-100'
  return (
    <RobotPanel
      className="npmrc-config-form max-w-190"
      icon={<Package className="size-4" />}
      title="npm 源"
      description="配置当前机器人的包下载源"
      actions={mode}
    >
      {editor === 'visual' ? (
        <>
          <div className="npmrc-config-fields grid grid-cols-1 gap-3">
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              Registry
              <select
                className={fieldClass}
                value={preset}
                onChange={event => {
                  const next = event.target.value
                  setPreset(next)
                  if (next !== 'custom') updateRegistry(next)
                }}
              >
                <option value={officialRegistry}>npm 官方源</option>
                <option value={mirrorRegistry}>npmmirror 镜像</option>
                <option value="custom">自定义地址</option>
              </select>
            </label>
            {preset === 'custom' && (
              <label className="grid gap-1 text-xs font-semibold text-slate-600">
                自定义地址
                <input
                  className={fieldClass}
                  value={customRegistry}
                  onChange={event => {
                    setCustomRegistry(event.target.value)
                    updateRegistry(event.target.value)
                  }}
                  placeholder="https://registry.example.com/"
                />
              </label>
            )}
          </div>
        </>
      ) : (
        <textarea
          className="min-h-72 w-full resize-y rounded-xl border border-slate-200 bg-white p-3 font-mono text-sm text-slate-700 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          value={content}
          onChange={event => onChange(event.target.value)}
          placeholder="npm 配置"
        />
      )}
    </RobotPanel>
  )
}
