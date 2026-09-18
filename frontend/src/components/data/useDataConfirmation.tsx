import { useEffect, useRef } from 'react'
import { useStoreState } from '../../store/guideStore'
import { ConfirmDialog } from '../ConfirmDialog'

export function useDataConfirmation(title: string) {
  const [message, setMessage] = useStoreState<string | null>(null)
  const resolve = useRef<((accepted: boolean) => void) | null>(null)
  useEffect(
    () => () => {
      resolve.current?.(false)
      resolve.current = null
    },
    []
  )
  const finish = (accepted: boolean) => {
    const pending = resolve.current
    resolve.current = null
    setMessage(null)
    pending?.(accepted)
  }
  const confirm = (text: string): Promise<boolean> => {
    if (resolve.current) return Promise.resolve(false)
    setMessage(text)
    return new Promise<boolean>(done => {
      resolve.current = done
    })
  }
  return {
    confirm,
    dialog: (
      <ConfirmDialog
        open={message !== null}
        title={title}
        message={message ?? ''}
        destructive
        onCancel={() => finish(false)}
        onConfirm={() => finish(true)}
      />
    )
  }
}
