export function directoryName(path: string) {
  const cleaned = path.replace(/[\\/]+$/, '')
  const leaf = cleaned.split(/[\\/]/).filter(Boolean).pop()
  return leaf || cleaned || path
}
