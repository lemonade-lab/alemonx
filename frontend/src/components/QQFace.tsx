import { useState } from 'react'
import { qqFaceData, qqFaceLabel } from './qqMessageMedia'

export function QQFaceSegment({ value }: { value: unknown }) {
  const face = qqFaceData(value)
  const label = qqFaceLabel(face)
  const source =
    face.faceType === '1' && /^\d+$/.test(face.faceId)
      ? `/qq-face/${face.faceId}.png`
      : ''
  const [failedSource, setFailedSource] = useState('')

  if (source && failedSource !== source)
    return (
      <span
        className="mx-0.5 inline-flex align-middle"
        title={label}
        aria-label={label}
      >
        <img
          className="size-6 object-contain"
          src={source}
          alt={label}
          loading="lazy"
          decoding="async"
          onError={() => setFailedSource(source)}
        />
      </span>
    )

  return (
    <span
      className="mx-0.5 inline-flex items-center rounded-[5px] border border-(--theme-border-subtle) bg-(--theme-surface-hover) px-1 align-baseline text-(--theme-text-secondary)"
      title={label}
    >
      {label}
    </span>
  )
}
