export type SQLiteColumn = { name: string; pk: number; hidden: number }
export const sqlIdentifier = (name: string) =>
  '"' + name.replace(/"/g, '""') + '"'
export const sqlLiteral = (value: unknown): string => {
  if (value === null) return 'NULL'
  if (
    typeof value === 'number' &&
    Number.isInteger(value) &&
    !Number.isSafeInteger(value)
  )
    throw new Error('大整数请使用双引号作为字符串输入，避免精度丢失。')
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value === 'string') return "'" + value.replace(/'/g, "''") + "'"
  throw new Error('此行包含二进制或不支持的数据类型，请手动编写 SQL。')
}
export function tableSelect(
  name: string,
  metadata: SQLiteColumn[],
  offset: number
) {
  const primary = metadata.filter(c => c.pk > 0).sort((a, b) => a.pk - b.pk)
  const order = primary.length ? primary : metadata.filter(c => c.hidden !== 1)
  if (!order.length) throw new Error('无法确定表的排序字段')
  return `SELECT * FROM ${sqlIdentifier(name)} ORDER BY ${order.map(c => sqlIdentifier(c.name)).join(', ')} LIMIT 100 OFFSET ${offset}`
}
export function rowMutation(
  table: string,
  names: string[],
  original: unknown[],
  metadata: SQLiteColumn[],
  changes: Record<string, unknown> | null
) {
  const primary = metadata.filter(c => c.pk > 0)
  if (
    !primary.length ||
    primary.some(c => original[names.indexOf(c.name)] == null)
  )
    throw new Error('主键缺失，无法安全生成单行操作。')
  const where = names
    .map((name, i) => `${sqlIdentifier(name)} IS ${sqlLiteral(original[i])}`)
    .join(' AND ')
  if (changes === null)
    return `DELETE FROM ${sqlIdentifier(table)} WHERE ${where};`
  const assignments = metadata
    .filter(
      c =>
        c.hidden === 0 &&
        c.pk === 0 &&
        Object.prototype.hasOwnProperty.call(changes, c.name) &&
        JSON.stringify(changes[c.name]) !==
          JSON.stringify(original[names.indexOf(c.name)])
    )
    .map(c => `${sqlIdentifier(c.name)} = ${sqlLiteral(changes[c.name])}`)
  if (!assignments.length)
    throw new Error('没有修改可写字段。主键和生成列不参与自动更新。')
  return `UPDATE ${sqlIdentifier(table)} SET\n  ${assignments.join(',\n  ')}\nWHERE ${where};`
}
