import type { SQLiteColumn } from '../../lib/sqliteEditing'
export type SQLiteRowEditorProps = {
  names: string[]
  row: unknown[]
  metadata: SQLiteColumn[]
  onClose: () => void
  onGenerate: (changes: Record<string, unknown>) => void
}
