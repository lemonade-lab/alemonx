import type { SQLConnection, SQLEngine } from '../../lib/connectModels'
import { newConnection } from '../../lib/connectModels'
import { dataInputClass } from './DataResults'

export function SQLConnectionForm({
  value,
  onChange,
  disabled,
  suggestions
}: {
  value: SQLConnection
  onChange: (value: SQLConnection) => void
  disabled: boolean
  suggestions: string[]
}) {
  const field = (
    key: 'host' | 'username' | 'password' | 'database' | 'path',
    label: string
  ) => (
    <label className="min-w-0 text-xs" key={key}>
      {label}
      <input
        className={dataInputClass}
        value={value[key]}
        disabled={disabled}
        type={key === 'password' ? 'password' : 'text'}
        autoComplete={key === 'password' ? 'new-password' : 'off'}
        onChange={e => onChange({ ...value, [key]: e.target.value })}
        list={key === 'path' ? 'connect-sqlite-paths' : undefined}
      />
    </label>
  )
  return (
    <div className="grid min-w-0 gap-3 sm:grid-cols-2">
      <label className="text-xs">
        数据库类型
        <select
          className={dataInputClass}
          disabled={disabled}
          value={value.engine}
          aria-label="数据库类型"
          onChange={e => onChange(newConnection(e.target.value as SQLEngine))}
        >
          <option value="mysql">MySQL</option>
          <option value="mariadb">MariaDB</option>
          <option value="postgres">PostgreSQL</option>
          <option value="sqlite">SQLite</option>
        </select>
      </label>
      {value.engine === 'sqlite' ? (
        <>
          {field('path', 'SQLite 文件（服务器绝对路径）')}
          <datalist id="connect-sqlite-paths">
            {suggestions.map(path => (
              <option key={path} value={path} />
            ))}
          </datalist>
        </>
      ) : (
        <>
          {field('host', '主机')}
          <label className="text-xs">
            端口
            <input
              className={dataInputClass}
              disabled={disabled}
              type="number"
              min={1}
              max={65535}
              value={value.port}
              onChange={e =>
                onChange({ ...value, port: Number(e.target.value) })
              }
            />
          </label>
          {field('username', '用户名')}
          {field('password', '密码（仅本次窗口）')}
          {field('database', '初始数据库（可选）')}
          <label className="text-xs">
            传输安全
            <select
              className={dataInputClass}
              disabled={disabled}
              value={value.tls}
              aria-label="传输安全"
              onChange={e =>
                onChange({
                  ...value,
                  tls: e.target.value as SQLConnection['tls']
                })
              }
            >
              <option value="verify-full">TLS · 验证证书和主机名</option>
              <option value="disable">关闭 TLS（仅可信网络）</option>
            </select>
          </label>
        </>
      )}
    </div>
  )
}
