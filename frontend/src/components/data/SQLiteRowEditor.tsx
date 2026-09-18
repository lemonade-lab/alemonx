import { useStoreState } from '../../store/guideStore'
import { DataDialog } from './DataDialog'
import { Button } from '../Button'
import { dataInputClass } from './DataResults'
import type { SQLiteRowEditorProps } from './SQLiteRowEditor.d'

export function SQLiteRowEditor({
  names,
  row,
  metadata,
  onClose,
  onGenerate
}: SQLiteRowEditorProps) {
  const writable = metadata.filter(c => c.hidden === 0 && c.pk === 0)
  const [draft, setDraft] = useStoreState<Record<string, string>>(() =>
    Object.fromEntries(
      writable.map(c => [
        c.name,
        JSON.stringify(row[names.indexOf(c.name)]) ?? 'null'
      ])
    )
  )
  const [error, setError] = useStoreState('')
  const generate = () => {
    try {
      const changes: Record<string, unknown> = Object.fromEntries(
        writable.map(c => [c.name, JSON.parse(draft[c.name]) as unknown])
      )
      onGenerate(changes)
    } catch (e) {
      setError(e instanceof Error ? e.message : '字段值无效，请输入 JSON 值。')
    }
  }
  return (
    <DataDialog open title="编辑 SQLite 行" onClose={onClose}>
      <p className="text-xs">
        填写 JSON 值：文本使用双引号，数字直接填写，空值为
        null。仅生成变更字段的 SQL，主键和生成列不更新。
      </p>
      {writable.map(c => (
        <label key={c.name} className="text-xs">
          {c.name}
          <textarea
            className={`${dataInputClass} mt-1 font-mono`}
            value={draft[c.name]}
            onChange={e => setDraft({ ...draft, [c.name]: e.target.value })}
          />
        </label>
      ))}
      {!writable.length && <p>此表没有可自动编辑的字段。</p>}
      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}
      <footer className="flex justify-end gap-2">
        <Button onClick={onClose}>取消</Button>
        <Button disabled={!writable.length} onClick={generate}>
          生成变更 SQL
        </Button>
      </footer>
    </DataDialog>
  )
}
