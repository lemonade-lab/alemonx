import type { SQLResult } from '../../store/dataApi'
import { Button } from '../Button'
export const dataInputClass =
  'w-full min-w-0 rounded-md border border-(--theme-border-default) bg-(--theme-surface-panel) p-2 text-sm text-(--theme-text-primary)'
export function DataResults({
  result,
  onEdit,
  onDelete
}: {
  result: SQLResult
  onEdit?: (row: unknown[]) => void
  onDelete?: (row: unknown[]) => void
}) {
  return (
    <div className="grid min-w-0 gap-2">
      <p role="status" className="text-xs text-(--theme-text-secondary)">
        {result.columns.length
          ? `${result.rows.length} 行${result.truncated ? '（结果已截断：上限 200 行 / 1 MiB，请添加 LIMIT 或分页）' : ''}`
          : `执行完成，影响 ${result.affected} 行`}
      </p>
      {result.columns.length > 0 && (
        <div className="max-h-96 overflow-auto rounded-md border border-(--theme-border-default)">
          <table className="w-full border-collapse text-left text-xs">
            <thead className="sticky top-0 bg-(--theme-surface-panel)">
              <tr>
                {onEdit && <th className="p-2">操作</th>}
                {result.columns.map((c, i) => (
                  <th
                    className="whitespace-nowrap border-b border-(--theme-border-default) p-2"
                    key={i}
                  >
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.rows.map((row, i) => (
                <tr key={i}>
                  {onEdit && (
                    <td className="whitespace-nowrap p-2">
                      <Button onClick={() => onEdit(row)}>编辑行</Button>{' '}
                      <Button onClick={() => onDelete?.(row)}>删除行</Button>
                    </td>
                  )}
                  {row.map((v, j) => (
                    <td
                      className="max-w-96 whitespace-pre-wrap break-words border-b border-(--theme-border-default) p-2 align-top"
                      key={j}
                    >
                      {v === null ? (
                        <em className="text-(--theme-text-secondary)">NULL</em>
                      ) : typeof v === 'object' ? (
                        JSON.stringify(v)
                      ) : (
                        String(v)
                      )}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {!result.rows.length && <p className="p-3">没有记录</p>}
        </div>
      )}
    </div>
  )
}
