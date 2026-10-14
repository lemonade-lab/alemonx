export function gitErrorMessage(reason: unknown, fallback: string): string {
  if (reason instanceof Error) return reason.message
  if (reason && typeof reason === 'object' && 'data' in reason) {
    const data = reason.data
    if (
      data &&
      typeof data === 'object' &&
      'error' in data &&
      typeof data.error === 'string'
    )
      return data.error
  }
  if (
    reason &&
    typeof reason === 'object' &&
    'error' in reason &&
    typeof reason.error === 'string'
  )
    return reason.error
  return fallback
}
