import { workspaceApi } from './workspaceApi'
import type { SQLConnection } from '../lib/connectModels'
export type ConnectOperation = {
  connection: SQLConnection
  action:
    | 'test'
    | 'databases'
    | 'schemas'
    | 'tables'
    | 'browse'
    | 'structure'
    | 'indexes'
    | 'query'
  schema?: string
  table?: string
  offset?: number
  sql?: string
  write?: boolean
  confirmed?: boolean
}
export type SQLResult = {
  columns: string[]
  rows: unknown[][]
  affected: number
  truncated: boolean
  systemDatabase?: boolean
}
export type RedisConnection = {
  address: string
  username: string
  password: string
  db: number
}
export const dataApi = workspaceApi.injectEndpoints({
  endpoints: build => ({
    connectSQL: build.mutation<SQLResult, ConnectOperation>({
      query: body => ({ url: 'data/connect', method: 'POST', body })
    }),
    defaultSQLite: build.mutation<{ path: string }, void>({
      query: () => ({ url: 'data/sqlite/default', method: 'POST', body: {} })
    }),
    dataCatalog: build.query<{ sqlite: string[]; redisAddress: string }, void>({
      query: () => 'data/catalog',
      providesTags: ['SystemRedis'],
      keepUnusedDataFor: 0
    }),
    dataSQL: build.mutation<
      SQLResult,
      { path: string; sql: string; write?: boolean; confirmed?: boolean }
    >({
      query: body => ({ url: 'data/sql', method: 'POST', body })
    }),
    dataRedis: build.mutation<
      { value: unknown },
      RedisConnection & {
        args: string[]
        confirmed?: boolean
        expected?: string
      }
    >({
      query: body => ({ url: 'data/redis', method: 'POST', body })
    })
  })
})
export const {
  useConnectSQLMutation,
  useDataCatalogQuery,
  useDataSQLMutation,
  useDataRedisMutation,
  useDefaultSQLiteMutation
} = dataApi
export function dataError(error: unknown) {
  if (error && typeof error === 'object' && 'data' in error) {
    const data = error.data as { error?: string } | undefined
    if (typeof data?.error === 'string') return data.error
  }
  return '请求未完成，请检查连接后重试。'
}
