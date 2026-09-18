import { useEffect } from 'react'
import { dataError, useDefaultSQLiteMutation } from '../../store/dataApi'
import { Button } from '../Button'
import { SQLiteBrowser } from './SQLiteBrowser'

export function BuiltinSQLite() {
  const [prepare, state] = useDefaultSQLiteMutation()
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) void prepare()
    })
    return () => {
      active = false
    }
  }, [prepare])
  if (state.isLoading || state.isUninitialized)
    return <p role="status">正在打开默认 SQLite…</p>
  if (state.isError || !state.data)
    return (
      <div role="alert">
        {dataError(state.error)}
        <Button onClick={() => void prepare()}>重试打开默认 SQLite</Button>
      </div>
    )
  return (
    <SQLiteBrowser
      key={state.data.path}
      defaults={[]}
      fixedPath={state.data.path}
    />
  )
}
