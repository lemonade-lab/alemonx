import { useMemo } from 'react'

type DiffLine = {
  text: string
  kind: '' | 'hunk' | 'meta' | 'add' | 'del'
  oldLine?: number
  newLine?: number
}

function formatDiff(diff: string): DiffLine[] {
  let oldLine: number | undefined
  let newLine: number | undefined

  return diff.split('\n').map(text => {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(text)
    if (hunk) {
      oldLine = Number(hunk[1])
      newLine = Number(hunk[2])
      return { text, kind: 'hunk' }
    }
    if (text.startsWith('diff --git')) {
      oldLine = undefined
      newLine = undefined
    }
    if (oldLine !== undefined && newLine !== undefined) {
      if (text.startsWith('+')) return { text, kind: 'add', newLine: newLine++ }
      if (text.startsWith('-')) return { text, kind: 'del', oldLine: oldLine++ }
      if (text.startsWith(' ')) return { text, kind: '', oldLine: oldLine++, newLine: newLine++ }
    }
    const kind = text.startsWith('@@')
      ? 'hunk'
      : /^(?:diff --git|index |--- |\+\+\+ |new file|deleted file|similarity|rename )/.test(text)
        ? 'meta'
        : text.startsWith('+') ? 'add' : text.startsWith('-') ? 'del' : ''
    return { text, kind }
  })
}

export function DiffView({ diff, untracked }: { diff: string; untracked: boolean }) {
  const lines = useMemo(() => formatDiff(diff), [diff])
  return (
    <div className="git-diff-body" aria-label="差异内容">
      {lines.map(({ text, kind, oldLine, newLine }, index) => (
        <div className={`git-diff-line${kind ? ` git-diff-${kind}` : ''}`} key={index}>
          <span className="git-diff-number" aria-hidden="true">{oldLine}</span>
          <span className="git-diff-number" aria-hidden="true">{newLine}</span>
          <span className="min-w-0">{text || (untracked ? '\u00a0' : ' ')}</span>
        </div>
      ))}
    </div>
  )
}
