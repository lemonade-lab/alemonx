import { expect, test } from '@playwright/test'
import { rowMutation, tableSelect, sqlLiteral } from '../src/lib/sqliteEditing'

const metadata = [
  { name: 'id', pk: 1, hidden: 0 },
  { name: 'value', pk: 0, hidden: 0 },
  { name: 'computed', pk: 0, hidden: 3 }
]
test('row form rejects unsafe integer numbers instead of rounding them', () => {
  expect(() => sqlLiteral(9007199254740992)).toThrow('精度丢失')
  expect(sqlLiteral('9223372036854775807')).toBe("'9223372036854775807'")
})
test('row update excludes primary and generated columns and only changes edited values', () => {
  const sql = rowMutation(
    'demo',
    ['id', 'value', 'computed'],
    [1, 'before', 6],
    metadata,
    { id: 2, value: 'after', computed: 99 }
  )
  expect(sql).toBe(
    `UPDATE "demo" SET\n  "value" = 'after'\nWHERE "id" IS 1 AND "value" IS 'before' AND "computed" IS 6;`
  )
  expect(() =>
    rowMutation(
      'demo',
      ['id', 'value', 'computed'],
      [1, 'before', 6],
      metadata,
      { value: 'before' }
    )
  ).toThrow('没有修改')
  expect(() =>
    rowMutation(
      'demo',
      ['id', 'value', 'computed'],
      [null, 'before', 6],
      metadata,
      { value: 'after' }
    )
  ).toThrow('主键缺失')
})
test('table browsing has explicit composite-key ordering and quotes identifiers', () => {
  const sql = tableSelect(
    'a"b',
    [
      { name: 'second', pk: 2, hidden: 0 },
      { name: 'first', pk: 1, hidden: 0 }
    ],
    100
  )
  expect(sql).toBe(
    'SELECT * FROM "a""b" ORDER BY "first", "second" LIMIT 100 OFFSET 100'
  )
  expect(
    tableSelect(
      'view',
      [
        { name: 'x', pk: 0, hidden: 0 },
        { name: 'internal', pk: 0, hidden: 1 }
      ],
      0
    )
  ).toContain('ORDER BY "x" LIMIT')
})
