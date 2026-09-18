export type SQLEngine = 'mysql' | 'mariadb' | 'postgres' | 'sqlite'
export type SQLConnection = {
  engine: SQLEngine
  host: string
  port: number
  username: string
  password: string
  database: string
  path: string
  tls: 'verify-full' | 'disable'
}
export type ConnectionProfile = Omit<SQLConnection, 'password'> & {
  id: string
  name: string
}
export const newConnection = (engine: SQLEngine = 'mysql'): SQLConnection => ({
  engine,
  host: 'localhost',
  port: engine === 'postgres' ? 5432 : 3306,
  username: engine === 'postgres' ? 'postgres' : 'root',
  password: '',
  database: engine === 'postgres' ? 'postgres' : '',
  path: '',
  tls: 'verify-full'
})
// Explicit allowlist: never persist arbitrary form properties or credentials.
export function connectionProfile(
  id: string,
  name: string,
  c: SQLConnection
): ConnectionProfile {
  return {
    id,
    name: name.slice(0, 80),
    engine: c.engine,
    host: c.host,
    port: c.port,
    username: c.username,
    database: c.database,
    path: c.path,
    tls: c.tls
  }
}
export function migrateConnections(
  value: unknown,
  paths: string[],
  names: Record<string, string>
): ConnectionProfile[] {
  if (Array.isArray(value))
    return value
      .filter(
        (v): v is ConnectionProfile =>
          v &&
          typeof v === 'object' &&
          ['mysql', 'mariadb', 'postgres', 'sqlite'].includes(v.engine) &&
          ['id', 'name', 'host', 'username', 'database', 'path'].every(
            k => typeof v[k] === 'string'
          ) &&
          Number.isInteger(v.port) &&
          ['verify-full', 'disable'].includes(v.tls)
      )
      .slice(0, 50)
      .map(v => connectionProfile(v.id, v.name, { ...v, password: '' }))
  return paths.map(path =>
    connectionProfile(`legacy:${path}`, names[path] || path, {
      ...newConnection('sqlite'),
      path
    })
  )
}
